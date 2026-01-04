package strategy

import (
	"testing"
	"time"

	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	underlying  = "SPY"
	asOfPrice   = 581.39
	openDate    = time.Date(2025, time.January, 14, 0, 0, 0, 0, time.UTC)
	expiryDate  = time.Date(2025, time.January, 17, 0, 0, 0, 0, time.UTC)
	provFile    = tests.GetLocalFileDataProvider()
	provMassive = tests.GetMassiveDataProvider()
)

func TestResolveStrike(t *testing.T) {
	tests := []struct {
		expr     string
		expected float64
	}{
		{"ATM", 581.0},
		{"ATM:+10", 591.0},
		{"ATM:-20", 561.0},
		{"ATM:+10%", 640.0},
		{"ATM:-20%", 465.0},
		{"ABS:600", 600.0},
		// {"DELTA:30", 610.0},
		// {"DELTA:50", 580.0},
		// {"{LEG1.STRIKE}+{LEG1.PREMIUM}", 581.0},
	}

	for _, test := range tests {
		actual, err := ResolveStrike(test.expr, underlying, asOfPrice, openDate, expiryDate, nil, provMassive)
		if err != nil {
			t.Fatalf("Failed to resolve strike: %v", err)
		}
		if actual != test.expected {
			t.Fatalf("For strike expression {%s}, expected %f, got %f", test.expr, test.expected, actual)
		}
	}
}

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
