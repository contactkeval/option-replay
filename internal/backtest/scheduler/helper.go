package scheduler

import (
	"sort"
	"time"
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

func intSliceContains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
