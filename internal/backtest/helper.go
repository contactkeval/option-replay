package backtest

import (
	"math"
	"sort"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
)

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

func findBarDate(d time.Time, dates []time.Time, mode DateMatchType) time.Time {

	// Search useful info
	var (
		exact  time.Time
		lower  time.Time
		higher time.Time
	)

	// default to MatchNearest
	switch mode {
	case MatchExact, MatchHigher, MatchLower, MatchNearest:
		// ok
	default:
		mode = MatchNearest
	}

	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	for _, dt := range dates {
		if dt.Equal(d) {
			exact = dt
		}
		if dt.Before(d) {
			lower = dt // will keep last ≤ d
		}
		if dt.After(d) && higher.IsZero() {
			higher = dt
		}
	}

	switch mode {

	case MatchExact:
		return exact // may be zero → caller skips it

	case MatchLower:
		return lower // last date before d

	case MatchHigher:
		return higher // first date after d

	case MatchNearest:
		if !exact.IsZero() {
			return exact
		}
		// choose whichever is closer
		switch {
		case !lower.IsZero() && !higher.IsZero():
			if d.Sub(lower) <= higher.Sub(d) {
				return lower
			}
			return higher
		case !lower.IsZero():
			return lower
		case !higher.IsZero():
			return higher
		}
	}

	return time.Time{} // nothing found
}

func extractCloses(bars []data.Bar) []float64 {
	out := make([]float64, 0, len(bars))
	for _, b := range bars {
		out = append(out, b.Close)
	}
	return out
}

func intSliceContains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

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
