// Package strategy contains logic for converting a high-level option strategy
// definition into fully-resolved trade legs.
//
// Responsibilities:
//   - Resolve expiration dates using market calendars
//   - Resolve strikes using rules such as ATM, DELTA, or leg expressions
//   - Fetch option prices and implied volatility via data providers
//   - Produce deterministic, replayable trade legs
//
// Design notes:
//   - This package is deterministic given inputs and provider behavior
//   - Logging is informational only and does not affect execution
//   - Errors are typed where useful and wrapped for caller inspection
package strategy

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Knetic/govaluate"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pricing"
)

//
// ==========================
// Error taxonomy
// ==========================
//

// Typed errors allow callers and tests to detect failure categories
// without string matching.
var (
	ErrInvalidStrikeExpression = errors.New("invalid strike expression")
	ErrLegIndexOutOfRange      = errors.New("leg index out of range")
)

//
// ==========================
// Domain Types
// ==========================
//

// TradeLeg represents a fully resolved option leg.
//
// It is the output of strategy planning and contains concrete market
// values derived from a LegSpec.
type TradeLeg struct {
	Spec         LegSpec   // Original leg specification
	Strike       float64   // Resolved option strike
	Expiration   time.Time // Resolved option expiration date
	OpenPremium  float64   // Premium at trade open
	ClosePremium float64   // Premium at trade close (filled later)
}

// LegSpec defines a single option leg as provided by the user or strategy JSON.
//
// This struct represents *intent*, not resolved market values.
type LegSpec struct {
	Side       string `json:"side,omitempty"`        // buy or sell (default: buy)
	OptionType string `json:"option_type,omitempty"` // call or put (default: call)
	StrikeRule string `json:"strike_rule"`           // ATM, ATM:+10, DELTA:0.3, {LEG1.STRIKE}, etc.
	Qty        int    `json:"qty,omitempty"`         // Quantity for ratio spreads
	Expiration int    `json:"expiration,omitempty"`  // DTE override for this leg
}

// StrategySpec defines a multi-leg option strategy.
//
// Shared defaults apply unless overridden at the leg level.
type StrategySpec struct {
	DaysToExpiry  int                `json:"dte,omitempty"`             // Default DTE
	DateMatchType data.DateMatchType `json:"date_match_type,omitempty"` // Expiry matching rule
	Legs          []LegSpec          `json:"strategy"`                  // Strategy legs
}

//
// ==========================
// Strategy Planning
// ==========================
//

// PlanStrategy resolves a strategy specification into concrete trade legs.
//
// It determines expiration dates, resolves strikes, fetches option premiums,
// and returns a slice of fully-specified TradeLegs ready for execution or replay.
//
// Parameters:
//   - strategy: Strategy definition including defaults and legs
//   - openDateTime: Timestamp when the strategy is opened
//   - underlying: Underlying symbol (e.g. NIFTY, SPY)
//   - openPrice: Spot price of the underlying at open
//   - expiryList: Available option expiration dates
//   - prov: Market data provider
//
// Returns:
//   - []TradeLeg: Fully resolved trade legs in order
//   - error: Non-nil if any leg cannot be resolved
func PlanStrategy(
	strategy StrategySpec,
	openDateTime time.Time,
	underlying string,
	openPrice float64,
	expiryList []time.Time,
	prov data.Provider,
) ([]TradeLeg, error) {

	logger.Infof(
		"event=plan_strategy underlying=%s open_time=%s price=%.2f",
		underlying,
		openDateTime.Format(time.RFC3339),
		openPrice,
	)

	legs := []TradeLeg{}

	for i, legSpec := range strategy.Legs {
		logger.Debugf("event=resolve_leg index=%d spec=%+v", i+1, legSpec)

		// Determine expiration offset
		offset := strategy.DaysToExpiry
		if legSpec.Expiration != 0 {
			offset = legSpec.Expiration
		}

		// Resolve expiration date
		expiryDate := ResolveExpiration(openDateTime, offset, expiryList, strategy.DateMatchType)
		logger.Tracef("event=expiry_resolved leg=%d expiry=%s", i+1, expiryDate.Format("2006-01-02"))

		strike, err := ResolveStrike(
			legSpec.StrikeRule,
			underlying,
			openPrice,
			openDateTime,
			expiryDate,
			legs,
			prov,
		)
		if err != nil {
			logger.Errorf("event=strike_resolution_failed leg=%d err=%v", i+1, err)
			return nil, err
		}

		// Fetch option premium
		openPremium, err := prov.GetOptionPrice(
			underlying,
			strike,
			expiryDate,
			legSpec.OptionType,
			openDateTime,
		)
		if err != nil {
			logger.Errorf("event=premium_fetch_failed leg=%d err=%v", i+1, err)
			return nil, err
		}

		logger.Infof(
			"event=leg_resolved leg=%d side=%s type=%s strike=%.2f premium=%.2f",
			i+1,
			legSpec.Side,
			legSpec.OptionType,
			strike,
			openPremium,
		)

		// Append resolved leg
		legs = append(legs, TradeLeg{
			Spec:        legSpec,
			Strike:      strike,
			Expiration:  expiryDate,
			OpenPremium: openPremium,
		})
	}

	return legs, nil
}

//
// ==========================
// Expiration Resolution
// ==========================
//

// ResolveExpiration determines the expiration date for an option leg.
//
// Parameters:
//   - openDate: Strategy open timestamp
//   - offset: Days-to-expiry offset (calendar days)
//   - expiries: Available expiration dates
//   - dateMatchType: Matching rule (nearest, prior, next, etc.)
//
// Returns:
//   - time.Time: Selected expiration date (may be zero if no match)
func ResolveExpiration(
	openDate time.Time,
	offset int,
	expiries []time.Time,
	dateMatchType data.DateMatchType,
) time.Time {
	candidate := openDate.AddDate(0, 0, offset)
	return data.MatchBarDate(candidate, expiries, dateMatchType)
}

//
// ==========================
// Strike Resolution
// ==========================
//

// ResolveStrike converts a strike expression into a concrete strike price.
//
// Supported formats:
//   - ATM
//   - ATM:+10, ATM:-5%
//   - DELTA:0.3
//   - {LEG1.STRIKE}+{LEG1.PREMIUM}
//
// Parameters:
//   - strikeExpr: Strike expression
//   - underlying: Underlying symbol
//   - asOfPrice: Spot price at evaluation time
//   - openDate: Strategy open timestamp
//   - expiryDate: Option expiration date
//   - legs: Previously resolved legs
//   - prov: Market data provider
//
// Returns:
//   - float64: Resolved strike price
//   - error: If expression cannot be evaluated
func ResolveStrike(
	strikeExpr string,
	underlying string,
	asOfPrice float64,
	openDate time.Time,
	expiryDate time.Time,
	legs []TradeLeg,
	prov data.Provider,
) (float64, error) {

	strikeExpr = strings.TrimSpace(strings.ToUpper(strikeExpr))
	logger.Debugf("event=resolve_strike expr=%s", strikeExpr)

	if strikeExpr == "ATM" {
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, asOfPrice), nil
	}

	if strings.HasPrefix(strikeExpr, "ATM:") {
		target, err := resolveATMOffset(strikeExpr[len("ATM:"):], asOfPrice)
		if err != nil {
			return 0, err
		}
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	if strings.HasPrefix(strikeExpr, "DELTA:") {
		deltaStr := strings.TrimPrefix(strikeExpr, "DELTA:")
		logger.Debugf("delta-based strike with target delta=%s", deltaStr)
		targetDelta, err := strconv.ParseFloat(deltaStr, 64)
		if err != nil {
			logger.Errorf("parse float failed for DELTA expression:%s, %v", deltaStr, err)
			return 0, fmt.Errorf("invalid DELTA value: %w", err)
		}
		target, err := resolveDeltaStrike(
			underlying,
			expiryDate,
			openDate,
			asOfPrice,
			targetDelta,
			prov,
		)
		if err != nil {
			logger.Errorf("resolve strike failed for DELTA expression:%s, %v", deltaStr, err)
			return 0, err
		}

		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	// Expression using previous legs
	if strings.Contains(strikeExpr, "{LEG") {
		target, err := evaluateLegExpression(strikeExpr, legs)
		if err != nil {
			return 0, err
		}
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	return 0, fmt.Errorf("%w: %s", ErrInvalidStrikeExpression, strikeExpr)
}

//
// ==========================
// Helpers
// ==========================
//

// resolveDeltaStrike computes a strike corresponding to a target delta.
//
// Parameters:
//   - underlying: Underlying symbol
//   - expiryDate: Option expiration date
//   - openDate: Strategy open timestamp
//   - asOfPrice: Spot price
//   - targetDelta: Desired option delta
//   - dataProv: Market data provider
//
// Returns:
//   - float64: Estimated strike price
//   - error: If IV or pricing fails
func resolveDeltaStrike(
	underlying string,
	expiryDate time.Time,
	openDate time.Time,
	asOfPrice float64,
	targetDelta float64,
	dataProv data.Provider,
) (float64, error) {

	// Fetch ATM option prices
	strike, callPrice, putPrice, err := dataProv.GetATMOptionPrices(
		underlying,
		expiryDate,
		openDate,
		asOfPrice,
	)
	if err != nil {
		return 0, err
	}

	// Estimate implied volatility
	daysToExpiry := expiryDate.Sub(openDate).Hours() / 24 / 365.25
	iv, err := pricing.ImpliedVolATM(asOfPrice, strike, daysToExpiry, 0.02, callPrice, putPrice)
	if err != nil {
		return 0, err
	}

	logger.Tracef("event=iv_estimated iv=%.4f dte=%.3f", iv, daysToExpiry)

	return pricing.StrikeFromDelta(asOfPrice, targetDelta, 0.02, 0.0, iv, daysToExpiry, true), nil
}

// resolveATMOffset applies an absolute or percentage offset to a price.
//
// Parameters:
//   - offset: Offset string (+10, -5%, etc.)
//   - asOfPrice: Spot price
//
// Returns:
//   - float64: Adjusted price
//   - error: If offset cannot be parsed
func resolveATMOffset(offset string, asOfPrice float64) (float64, error) {

	if strings.HasSuffix(offset, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(offset, "%"), 64)
		if err != nil {
			return 0, err
		}
		return math.Round((asOfPrice+asOfPrice*pct/100)*100) / 100, nil
	}

	abs, err := strconv.ParseFloat(offset, 64)
	if err != nil {
		return 0, err
	}

	return math.Round((asOfPrice+abs)*100) / 100, nil
}

// evaluateLegExpression evaluates expressions referencing prior legs.
//
// Parameters:
//   - expr: Expression string
//   - legs: Previously resolved legs
//
// Returns:
//   - float64: Evaluated numeric result
//   - error: If expression is invalid or cannot be evaluated
func evaluateLegExpression(expr string, legs []TradeLeg) (float64, error) {

	re := regexp.MustCompile(`\{LEG(\d)\.(STRIKE|PREMIUM)\}`)
	matches := re.FindAllStringSubmatch(expr, -1)
	if matches == nil {
		return 0, ErrInvalidStrikeExpression
	}

	evalStr := expr

	for _, match := range matches {
		idx, _ := strconv.Atoi(match[1])
		idx-- // LEG1 → index 0

		if idx < 0 || idx >= len(legs) {
			return 0, ErrLegIndexOutOfRange
		}

		var value float64
		if match[2] == "STRIKE" {
			value = legs[idx].Strike
		} else {
			// "PREMIUM"
			value = legs[idx].OpenPremium
		}

		evalStr = strings.Replace(evalStr, match[0], fmt.Sprintf("%f", value), 1)
	}

	evalExpr, err := govaluate.NewEvaluableExpression(evalStr)
	if err != nil {
		return 0, err
	}

	result, err := evalExpr.Evaluate(nil)
	if err != nil {
		return 0, err
	}

	f, ok := result.(float64)
	if !ok {
		return 0, ErrInvalidStrikeExpression
	}

	return f, nil
}
