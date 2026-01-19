package strategy

import (
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
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
