package strategy

import (
	"testing"
	"time"

	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	underlying = "SPY"
	spotPrice  = 581.39
	openDate   = time.Date(2025, time.January, 14, 0, 0, 0, 0, time.UTC)
	expiryDate = time.Date(2025, time.January, 17, 0, 0, 0, 0, time.UTC)
	provFile   = tests.GetLocalFileDataProvider()
	prov       = tests.GetMassiveDataProvider()
)

func TestResolveStrike(t *testing.T) {
	// strikeExpr := []string{"ATM", "ATM:+10", "ATM:-20", "ATM:+10%", "ATM:-20%", "ABS:600", "DELTA:30", "DELTA:50", "{LEG1.STRIKE}+{LEG1.PREMIUM}"}
	strikeExpr := []string{"ATM"}
	expectedStrikes := []float64{581.0}

	for i, expr := range strikeExpr {
		actual, err := ResolveStrike(expr, underlying, spotPrice, openDate, expiryDate, nil, prov)
		if err != nil {
			t.Fatalf("Failed to resolve strike: %v", err)
		}
		expected := expectedStrikes[i]
		if actual != expected {
			t.Fatalf("For strike expression {%s} [%d], expected %f, got %f", expr, i, expected, actual)
		}
	}
}
