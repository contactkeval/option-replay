package strategy

import (
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	underlying  = "SPY"
	asOfPrice   = 581.39
	openDate    = time.Date(2025, time.January, 14, 0, 0, 0, 0, time.UTC)
	expiryDate  = time.Date(2025, time.January, 17, 0, 0, 0, 0, time.UTC)
	provFile    = data.GetLocalFileDataProvider()
	provMassive = data.GetMassiveDataProvider()
)

func TestResolveATMOffset(t *testing.T) {
	tests := []struct {
		expr     string
		expected float64
	}{
		{"+10", 591.39},
		{"-20", 561.39},
		{"+10%", 639.53},
		{"-20%", 465.11},
	}

	for _, test := range tests {
		actual, err := resolveATMOffset(test.expr, asOfPrice)
		if err != nil {
			t.Fatalf("Failed to resolve ATM offset: %v", err)
		}
		if actual != test.expected {
			t.Fatalf("For offset {%s}, expected %f, got %f", test.expr, test.expected, actual)
		}
	}
}

func TestEvalLegExp(t *testing.T) {
	// Sample legs with necessary data for testing
	legs := []TradeLeg{
		{Strike: 580.0, OpenPremium: 2.5},
		{Strike: 590.0, OpenPremium: 3.0},
	}
	tests := []struct {
		expr     string
		expected float64
	}{
		{"{LEG1.STRIKE}+{LEG1.PREMIUM}", 582.5},
		{"{LEG2.STRIKE}-{LEG2.PREMIUM}", 587.0},
		{"{LEG1.PREMIUM}+{LEG2.PREMIUM}", 5.5},
		{"({LEG1.PREMIUM}+{LEG2.PREMIUM})/2.0", 2.75},
		{"{LEG1.STRIKE}+({LEG1.PREMIUM}+{LEG2.PREMIUM})/2", 582.75},
	}

	for _, test := range tests {
		actual, err := evaluateLegExpression(test.expr, legs)
		if err != nil {
			t.Fatalf("Failed to evaluate leg expression: %v", err)
		}
		if actual != test.expected {
			t.Fatalf("For expression \"%s\", expected %f, got %f", test.expr, test.expected, actual)
		}
	}
}

func TestResolveStrike(t *testing.T) {
	// Sample legs with necessary data for testing
	legs := []TradeLeg{
		{Strike: 580.0, OpenPremium: 2.5},
		{Strike: 590.0, OpenPremium: 3.0},
	}
	tests := []struct {
		expr     string
		expected float64
	}{
		{"ATM", 581.0},
		{"ATM:+10", 591.0},
		{"ATM:-20", 561.0},
		{"ATM:+10%", 640.0},
		{"ATM:-20%", 465.0},
		// {"ABS:600", 600.0},
		// {"DELTA:30", 610.0},
		// {"DELTA:50", 580.0},
		{"{LEG1.STRIKE}+{LEG1.PREMIUM}", 583},
		{"{LEG2.STRIKE}-{LEG2.PREMIUM}", 587.0},
		{"{LEG1.STRIKE}+{LEG1.PREMIUM}+{LEG2.PREMIUM}", 586},
		{"{LEG1.STRIKE}+({LEG1.PREMIUM}+{LEG2.PREMIUM})/2", 583},
	}

	for _, test := range tests {
		actual, err := ResolveStrike(test.expr, underlying, asOfPrice, openDate, expiryDate, legs, provMassive)
		if err != nil {
			t.Fatalf("Failed to resolve strike for expression {%s}: %v", test.expr, err)
		}
		if actual != test.expected {
			t.Fatalf("For strike expression {%s}, expected %f, got %f", test.expr, test.expected, actual)
		}
	}
}

func TestPlanStrategyStrangle(t *testing.T) {
	strategy := StrategySpec{
		Legs: []LegSpec{
			{Side: "sell", OptionType: "call", StrikeRule: "ATM:+10", Qty: 1},
			{Side: "sell", OptionType: "put", StrikeRule: "ATM:-10", Qty: 1},
		},
		DateMatchType: data.MatchNearest,
	}
	legs, err := PlanStrategy(strategy, openDate, underlying, asOfPrice, []time.Time{expiryDate}, provMassive)
	if err != nil {
		t.Fatalf("Failed to plan strategy: %v", err)
	}
	if len(legs) != 2 {
		t.Fatalf("Expected 2 legs, got %d", len(legs))
	}

	tests.CompareWithGolden(t, "strategy_strangle", legs)
}

func TestPlanStrategyCustom(t *testing.T) {
	//"2025-01-02T10:00:00-05:00",
	expiryList := []time.Time{
		openDate, openDate.AddDate(0, 0, 1), openDate.AddDate(0, 0, 2), openDate.AddDate(0, 0, 3),
	}
	strategy := StrategySpec{
		Legs: []LegSpec{
			{Side: "sell", OptionType: "call", StrikeRule: "ATM", Qty: 1, Expiration: 0},
			{Side: "sell", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 0},
			{Side: "buy", OptionType: "call", StrikeRule: "ATM", Qty: 1, Expiration: 1},
			{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 1},
		},
		DateMatchType: data.MatchHigher,
	}
	legs, err := PlanStrategy(strategy, openDate, underlying, asOfPrice, expiryList, provMassive)
	if err != nil {
		t.Fatalf("Failed to plan strategy: %v", err)
	}
	if len(legs) != 4 {
		t.Fatalf("Expected 4 legs, got %d", len(legs))
	}

	tests.CompareWithGolden(t, "strategy_custom", legs)
}

func TestPlanStrategyCustom3(t *testing.T) {
	//"2025-01-02T10:00:00-05:00",
	expiryList := []time.Time{
		time.Date(2025, time.January, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2025, time.January, 24, 0, 0, 0, 0, time.UTC),
		time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC),
	}
	strategy := StrategySpec{
		Legs: []LegSpec{
			{Side: "sell", OptionType: "call", StrikeRule: "ATM", Qty: 1, Expiration: 2},
			{Side: "sell", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 2},
			{Side: "buy", OptionType: "call", StrikeRule: "ATM", Qty: 1, Expiration: 9},
			{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 9},
		},
		DateMatchType: data.MatchHigher,
	}
	legs, err := PlanStrategy(strategy, openDate, underlying, asOfPrice, expiryList, provMassive)
	if err != nil {
		t.Fatalf("Failed to plan strategy: %v", err)
	}
	if len(legs) != 4 {
		t.Fatalf("Expected 4 legs, got %d", len(legs))
	}

	tests.CompareWithGolden(t, "strategy_custom3", legs)
}
