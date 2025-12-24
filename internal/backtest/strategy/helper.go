package strategy

import (
	"math"
	"time"
)

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

func fetchATMOptionPrices(spot float64, underlying string, expiry time.Time) (call float64, put float64, err error) {
	// TODO: call your option chain API
	return 5.20, 4.85, nil
}

func estimateIVFromATM(call, put, spot float64) float64 {
	// TODO: real IV estimator
	return 0.20
}

func computeStrikeFromDelta(delta, spot, iv float64, expiry time.Time) float64 {
	// TODO: real delta → strike model
	return spot * (1 - (delta/100.0)*0.5)
}

func roundToNearestStrike(v float64) float64 {
	strikeInterval := 50.0 // Example for NIFTY, change as needed
	return math.Round(v/strikeInterval) * strikeInterval
}
