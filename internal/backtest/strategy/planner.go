package strategy

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
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
	spotPrice float64,
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
		return prov.RoundToNearestStrike(underlying, spotPrice, openDate, expiryDate), nil
	}

	// ---------------------------------------------------------
	// 2. ATM modifiers: ATM:+10, ATM:-10%, etc.
	// ---------------------------------------------------------
	if strings.HasPrefix(strikeExpr, "ATM:") {
		offset := strikeExpr[len("ATM:"):] // "+10", "-10%", etc.
		return resolveATMOffset(offset, spotPrice)
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
		return resolveDeltaStrike(targetDelta, spotPrice, underlying, expiryDate)
	}

	// ---------------------------------------------------------
	// 4. Expression evaluator using legs:
	//    "{LEG1.STRIKE}+{LEG1.PREMIUM}"
	// ---------------------------------------------------------
	if strings.Contains(strikeExpr, "{LEG") {
		val, err := evaluateLegExpression(strikeExpr, legs)
		if err != nil {
			return 0, err
		}
		return roundToNearestStrike(val), nil
	}

	return 0, fmt.Errorf("unrecognized strike expression: %s", strikeExpr)
}

// offset = "+10", "-20", "+10%", "-5%" etc.
func resolveATMOffset(offset string, spot float64) (float64, error) {

	// Percentage offset?
	if strings.HasSuffix(offset, "%") {
		pctStr := offset[:len(offset)-1]
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid percent offset: %w", err)
		}
		target := spot + (spot * pct / 100.0)
		return roundToNearestStrike(target), nil
	}

	// Absolute offset
	absVal, err := strconv.ParseFloat(offset, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid absolute offset: %w", err)
	}
	return roundToNearestStrike(spot + absVal), nil
}

func resolveDeltaStrike(
	targetDelta float64,
	spot float64,
	underlying string,
	expiry time.Time,
) (float64, error) {

	// 1. Fetch ATM option chain
	callPrice, putPrice, err := fetchATMOptionPrices(spot, underlying, expiry)
	if err != nil {
		return 0, err
	}

	// 2. Estimate implied volatility (stub)
	iv := estimateIVFromATM(callPrice, putPrice, spot)

	// 3. Compute strike for desired delta (Black–Scholes stub)
	strike := computeStrikeFromDelta(targetDelta, spot, iv, expiry)

	return roundToNearestStrike(strike), nil
}

func evaluateLegExpression(expr string, legs []TradeLeg) (float64, error) {

	// Regex to find patterns like {LEG1.STRIKE}
	re := regexp.MustCompile(`\{LEG(\d+)\.(STRIKE|PREMIUM)\}`)

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

	// Evaluate the final numeric expression
	return evalSimpleMath(evalStr)
}

func evalSimpleMath(s string) (float64, error) {
	// VERY simple expression evaluator — replace with govaluate if you want power
	s = strings.ReplaceAll(s, " ", "")

	// For now just handle + and -
	tokens := regexp.MustCompile(`([+\-])`).Split(s, -1)
	// ops := regexp.MustCompile(`[0-9.]+`).FindAllString(s, -1)

	if len(tokens) == 1 {
		return strconv.ParseFloat(tokens[0], 64)
	}

	// Full evaluator recommended for future, but this works for simple cases
	result, _ := strconv.ParseFloat(tokens[0], 64)
	pos := len(tokens[0])

	for pos < len(s) {
		op := string(s[pos])
		pos++

		// next number
		nextNumStr := ""
		for pos < len(s) && (s[pos] == '.' || (s[pos] >= '0' && s[pos] <= '9')) {
			nextNumStr += string(s[pos])
			pos++
		}
		nextNum, _ := strconv.ParseFloat(nextNumStr, 64)

		if op == "+" {
			result += nextNum
		} else {
			result -= nextNum
		}
	}

	return result, nil
}
