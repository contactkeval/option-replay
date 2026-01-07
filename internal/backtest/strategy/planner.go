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
	"github.com/contactkeval/option-replay/internal/pricing"
)

// Trade/TradeLeg/Bar types reused from original but simplified for internal use
type TradeLeg struct {
	Spec         LegSpec
	Strike       float64
	OptType      string
	Qty          int
	Expiration   time.Time
	OpenPremium  float64
	ClosePremium float64
}

// Individual leg specification
type LegSpec struct {
	Side       string `json:"side,omitempty"`        // "buy" or "sell", defaults to "buy"
	OptionType string `json:"option_type,omitempty"` // "call" or "put", defaults to "call"
	StrikeRule string `json:"strike_rule"`           // "ATM", "ABS:100", "DELTA:0.3", etc.
	Qty        int    `json:"qty,omitempty"`         // used for ratio spreads, defaults to one
	Expiration string `json:"expiration,omitempty"`  // used for calendar spreads, defaults DTE from config
	LegName    string `json:"leg_name,omitempty"`    // used for dependent wings
}

// ResolveStrike resolves a strike expression like:
// "ATM", "ATM:+10", "ATM:-10%", "DELTA:30",
// "{LEG1.STRIKE}+{LEG1.PREMIUM}" etc.
func ResolveStrike(
	strikeExpr string, // strike expression e.g. "ATM", "ATM:+10", "DELTA:30", "{LEG1.STRIKE}+{LEG1.PREMIUM}"
	underlying string,
	asOfPrice float64,
	openDate time.Time,
	expiryDate time.Time,
	legs []TradeLeg,
	prov data.Provider,
) (float64, error) {

	// Trim spaces for safety
	strikeExpr = strings.TrimSpace(strings.ToUpper(strikeExpr))

	// ---------------------------------------------------------
	// 1. Simple ATM case
	// ---------------------------------------------------------
	if strikeExpr == "ATM" {
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, asOfPrice), nil
	}

	// ---------------------------------------------------------
	// 2. ATM modifiers: ATM:+10, ATM:-10%, etc.
	// ---------------------------------------------------------
	if strings.HasPrefix(strikeExpr, "ATM:") {
		offset := strikeExpr[len("ATM:"):] // "+10", "-10%", etc.
		target, err := resolveATMOffset(offset, asOfPrice)
		if err != nil {
			return 0, err
		}
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	// ---------------------------------------------------------
	// 3. DELTA:X rule
	// ---------------------------------------------------------
	if strings.HasPrefix(strikeExpr, "DELTA:") {
		deltaStr := strings.TrimPrefix(strikeExpr, "DELTA:")
		targetDelta, err := strconv.ParseFloat(deltaStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid DELTA value: %w", err)
		}
		target, err := resolveDeltaStrike(underlying, expiryDate, openDate, asOfPrice, targetDelta, prov)
		if err != nil {
			return 0, err
		}
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	// ---------------------------------------------------------
	// 4. Expression evaluator using legs:
	//    "{LEG1.STRIKE}+{LEG1.PREMIUM}"
	// ---------------------------------------------------------
	if strings.Contains(strikeExpr, "{LEG") {
		target, err := evaluateLegExpression(strikeExpr, legs)
		if err != nil {
			return 0, err
		}
		return prov.RoundToNearestStrike(underlying, expiryDate, openDate, target), nil
	}

	return 0, fmt.Errorf("unrecognized strike expression: %s", strikeExpr)
}

// offset = "+10", "-20", "+10%", "-5%" etc.
func resolveATMOffset(offset string, asOfPrice float64) (float64, error) {

	// Percentage offset?
	if strings.HasSuffix(offset, "%") {
		pctStr := offset[:len(offset)-1]
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid percent offset: %w", err)
		}
		target := asOfPrice + (asOfPrice * pct / 100.0)
		target = math.Round(target*100) / 100 // round to 2 decimals
		return target, nil
	}

	// Absolute offset
	absVal, err := strconv.ParseFloat(offset, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid absolute offset: %w", err)
	}
	return math.Round((asOfPrice+absVal)*100) / 100, nil
}

func resolveDeltaStrike(
	underlying string,
	expiryDate time.Time,
	openDate time.Time,
	asOfPrice float64,
	targetDelta float64,
	prov data.Provider,
) (float64, error) {

	// 1. Fetch ATM option chain
	strike, callPrice, putPrice, err := prov.GetATMOptionPrices(underlying, expiryDate, asOfPrice)
	if err != nil {
		return 0, err
	}

	// 2. Estimate implied volatility (stub)
	iv, err := pricing.ImpliedVolATM(asOfPrice, strike, expiryDate.Sub(openDate).Hours()/24/365.25, 0.02, callPrice, putPrice)
	if err != nil {
		return 0, err
	}

	// 3. Compute strike for desired delta (Black–Scholes stub)
	return computeStrikeFromDelta(targetDelta, asOfPrice, iv, expiryDate), nil

	//4. TODO: refine with real market data (after estimating strike, find closest strike from option chain by calculating deltas using market prices)
}

func evaluateLegExpression(expr string, legs []TradeLeg) (float64, error) {

	// Regex to find patterns like {LEG1.STRIKE}
	re := regexp.MustCompile(`\{LEG(\d)\.(STRIKE|PREMIUM)\}`)

	m := re.FindAllStringSubmatch(expr, -1)
	if m == nil {
		return 0, errors.New("invalid leg expression")
	}

	// Replace tokens with numeric values
	evalStr := expr

	for _, match := range m {
		legIndexStr := match[1]
		field := match[2]

		idx, _ := strconv.Atoi(legIndexStr)
		idx = idx - 1 // LEG1 → index 0

		if idx < 0 || idx >= len(legs) {
			return 0, fmt.Errorf("LEG%d out of range", idx+1)
		}

		var value float64
		switch field {
		case "STRIKE":
			value = legs[idx].Strike
		case "PREMIUM":
			value = legs[idx].OpenPremium
		}

		evalStr = strings.Replace(evalStr, match[0], fmt.Sprintf("%f", value), 1)
	}

	evalExpr, err := govaluate.NewEvaluableExpression(evalStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse leg expression: %w", err)
	}

	calcVal, err := evalExpr.Evaluate(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to evaluate leg expression: %w", err)
	}

	if calcValResult, ok := calcVal.(float64); ok {
		return calcValResult, nil
	} else {
		return 0, fmt.Errorf("leg expression {%s} could not be evaluated to a number: %v", expr, calcValResult)
	}
}
