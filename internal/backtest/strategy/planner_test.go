package strategy

import (
	"testing"
	"time"

	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	underlying = "SPY"
	spotPrice  = 450.0
	openDate   = time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC)
	expiryDate = time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC)
	prov       = tests.GetLocalFileDataProvider()
)

func TestATMStrikeStrategy(t *testing.T) {
	strikeExpr := "ATM"
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestATMPlus10StrikeStrategy(t *testing.T) {
	strikeExpr := "ATM:+10"
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestDelta30StrikeStrategy(t *testing.T) {
	strikeExpr := "DELTA:30"
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestDelta50StrikeStrategy(t *testing.T) {
	strikeExpr := "DELTA:50"
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestAbsoluteStrikeStrategy(t *testing.T) {
	strikeExpr := "ABS:460"
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestUnsupportedStrikeStrategy(t *testing.T) {
	strikeExpr := "RANDOM:100"
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, nil, prov)
	if err == nil {
		t.Fatalf("Expected error for unsupported strike rule")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}

func TestLegBasedStrikeStrategy(t *testing.T) {
	strikeExpr := "{LEG1.STRIKE}+{LEG1.PREMIUM}"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	strike, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err != nil {
		t.Fatalf("Failed to resolve strike: %v", err)
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr, strike)
}

func TestLegBasedStrikeStrategyMissingLeg(t *testing.T) {
	strikeExpr := "{LEG2.STRIKE}+{LEG1.PREMIUM}"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err == nil {
		t.Fatalf("Expected error for missing leg")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}

func TestLegBasedStrikeStrategyInvalidPlaceholder(t *testing.T) {
	strikeExpr := "{LEG1.UNKNOWN}+{LEG1.PREMIUM}"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err == nil {
		t.Fatalf("Expected error for invalid placeholder")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}

func TestLegBasedStrikeStrategyInvalidFormat(t *testing.T) {
	strikeExpr := "LEG1.STRIKE+{LEG1.PREMIUM}"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err == nil {
		t.Fatalf("Expected error for invalid format")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}

func TestLegBasedStrikeStrategyNonNumeric(t *testing.T) {
	strikeExpr := "{LEG1.STRIKE}+abc"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err == nil {
		t.Fatalf("Expected error for non-numeric addition")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}

func TestLegBasedStrikeStrategyDivisionByZero(t *testing.T) {
	strikeExpr := "{LEG1.STRIKE}/{LEG1.PREMIUM_MINUS_PREMIUM}"
	legs := []TradeLeg{
		{Strike: 450.0, OpenPremium: 5.0},
	}
	_, err := ResolveStrike(strikeExpr, underlying, spotPrice, openDate, expiryDate, legs, prov)
	if err == nil {
		t.Fatalf("Expected error for division by zero")
	}
	tests.CompareWithGolden(t, "strategy_"+strikeExpr+"_error", err.Error())
}
