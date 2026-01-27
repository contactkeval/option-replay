package strategy

import (
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
