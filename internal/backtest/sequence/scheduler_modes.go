package sequence

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
)

// Constants for Mode strings to avoid typos
const (
	ModeEarningsOffset = "earnings_offset"
	ModeExpiryOffset   = "expiry_offset"
	ModeNthMonthDay    = "nth_month_day"
	ModeNthWeekday     = "nth_weekday"
	ModeDailyTime      = "daily_time"
)

// ScheduleDates computes a list of trading dates for a backtest entry rule
// using the provided market bars (barMap). The function interprets the EntryRule
// to produce candidate dates between entry.Start and entry.End (inclusive),
// matches those candidates to available bars with findBarDate using
// entry.DateMatchType, and returns a sorted, deduplicated slice of time.Time.
//
// Behavior and defaults:
//   - If entry.Start is zero, it defaults to today UTC minus one year.
//   - If entry.End is zero, it defaults to today UTC.
//   - If entry.Start is after entry.End, an error is returned.
//
// Supported Mode values (case-insensitive):
// -"earnings_offset":
//   - Requires entry.Underlying to be non-empty.
//   - Uses fetchEarningsDates(entry.Underlying) to obtain earnings dates.
//   - Uses the first element of entry.NthList as a day offset (e.g., -5
//     means 5 days before earnings).
//   - For each earnings date within the [Start, End] range, applies the
//     offset, matches to a bar via findBarDate and includes it if found.
//   - Returns an error if earnings lookup fails.
//
// -"expiry_offset":
//   - Assumes entry.Underlying is provided and obtains expiries via
//     getRelevantExpiries using a MassiveDataProvider initialized with the
//     POLYGON_API_KEY environment variable.
//   - Uses the first element of entry.NthList as a day offset relative to
//     each expiry date.
//   - Candidate dates outside [Start, End] are skipped. Each candidate is
//     matched to a bar via findBarDate.
//   - Returns an error if expiry lookup fails.
//
// -"nth_month_day":
//   - Requires entry.NthList to be non-empty.
//   - For every month overlapping the [Start, End] span, selects the day
//     numbers specified in entry.NthList (ignores invalid day numbers for
//     that month, e.g., Feb 30) and matches each valid candidate to a bar.
//
// -"nth_weekday":
//   - Requires entry.NthList to be non-empty.
//   - Iterates every calendar date in [Start, End]. For each date it
//     determines the weekday's occurrence index within the ISO week
//     (Monday = start of week, occurrences counted from 1). If that index
//     appears in entry.NthList the date is matched to a bar.
//     Example: to get the 2nd Tuesday of each ISO week, supply the index 2
//     for the Tuesday weekday.
//
// -default (any other mode):
//   - Daily schedule: every calendar date in [Start, End] is matched to a
//     bar and included if a bar exists.
//
// Matching and return details:
//   - Candidate dates are matched to bars using findBarDate(candidate, barMap,
//     entry.DateMatchType). Only non-zero matches are included.
//   - Candidates outside the provided [Start, End] range are ignored.
//   - The function sorts the resulting times ascending and removes duplicates
//     based on the calendar date (YYYY-MM-DD).
//   - Returned times correspond to the matched bar timestamps (as produced by
//     findBarDate).
//
// Errors:
//   - Returned for invalid input (e.g., Start after End), missing required
//     parameters for a mode (e.g., missing NthList), and for failures when
//     fetching external data (earnings or expiries). Mode-specific errors
//     wrap the underlying error to aid diagnosis.
//
// Parameters:
//   - when: scheduling rule describing mode and parameters (Mode, Underlying,
//     Nth, ExpiryCycle, Weekday, EveryNCalendar, etc.).
//   - start: inclusive lower bound of the scheduling window.
//   - end: inclusive upper bound of the scheduling window.
//   - barMap: a map of available market bars (indexed by date) used to snap
//     candidates to the nearest available trading date.
//
// Returns:
//   - []time.Time: sorted, unique list of scheduled trading dates (as time.Time).
func ScheduleDates(
	entryRule EntryRule,
	barMap []data.Bar,
	expiryList []time.Time,
) ([]time.Time, error) {
	logger.Debugf("Scheduling dates | Mode: %s", entryRule.Mode)

	// 1. Prepare Data & Validate
	barDates := extractBarDates(barMap)
	mode := strings.ToLower(strings.TrimSpace(entryRule.Mode))

	if entryRule.StartDate.After(entryRule.EndDate) {
		entryRule.StartDate, entryRule.EndDate = entryRule.EndDate, entryRule.StartDate
	}

	if err := validateEntryRule(mode, entryRule); err != nil {
		return nil, err
	}

	// 2. Dispatch to specialized logic
	var out []time.Time
	var err error

	switch mode {
	case ModeEarningsOffset:
		out, err = scheduleEarningsOffset(entryRule, barDates)
	case ModeExpiryOffset:
		out, err = scheduleExpiryOffset(entryRule, expiryList, barDates)
	case ModeNthMonthDay:
		out, err = scheduleNthMonthDay(entryRule, barDates)
	case ModeNthWeekday:
		out, err = scheduleNthWeekday(entryRule, barDates)
	default:
		out, err = scheduleDaily(entryRule, barDates)
	}

	if err != nil {
		return nil, err
	}

	return finalizeDates(out), nil
}

// --- Sub-Schedulers ---

func scheduleEarningsOffset(entryRule EntryRule, barDates []time.Time) ([]time.Time, error) {
	if entryRule.Underlying == "" {
		return nil, fmt.Errorf("underlying symbol required for earnings mode")
	}
	earnings, err := GetEarningsDates(entryRule.Underlying)
	if err != nil {
		return nil, err
	}

	var out []time.Time
	offset := entryRule.NthList[0]
	for _, e := range earnings {
		candidate := e.AddDate(0, 0, offset)
		if isWithinRange(candidate, entryRule.StartDate, entryRule.EndDate) {
			if day := data.MatchBarDate(candidate, barDates, entryRule.DateMatchType); !day.IsZero() {
				out = append(out, day)
			}
		}
	}
	return out, nil
}

func scheduleExpiryOffset(entryRule EntryRule, expiries []time.Time, barDates []time.Time) ([]time.Time, error) {
	var out []time.Time
	offset := entryRule.NthList[0]
	for _, expiry := range expiries {
		candidate := expiry.AddDate(0, 0, offset)
		if isWithinRange(candidate, entryRule.StartDate, entryRule.EndDate) {
			if day := data.MatchBarDate(candidate, barDates, entryRule.DateMatchType); !day.IsZero() {
				out = append(out, day)
			}
		}
	}
	return out, nil
}

func scheduleNthMonthDay(entryRule EntryRule, barDates []time.Time) ([]time.Time, error) {
	var out []time.Time
	for year := entryRule.StartDate.Year(); year <= entryRule.EndDate.Year(); year++ {
		for month := time.January; month <= time.December; month++ {
			for _, day := range entryRule.NthList {
				candidate := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
				// Guard: Check if date is valid (e.g. not Feb 30) and within range
				if candidate.Month() == month && isWithinRange(candidate, entryRule.StartDate, entryRule.EndDate) {
					if day := data.MatchBarDate(candidate, barDates, entryRule.DateMatchType); !day.IsZero() {
						out = append(out, day)
					}
				}
			}
		}
	}
	return out, nil
}

func scheduleNthWeekday(entryRule EntryRule, barDates []time.Time) ([]time.Time, error) {
	var out []time.Time
	for candidate := entryRule.StartDate; !candidate.After(entryRule.EndDate); candidate = candidate.AddDate(0, 0, 1) {
		if intSliceContains(entryRule.NthList, int(candidate.Weekday())) {
			if day := data.MatchBarDate(candidate, barDates, entryRule.DateMatchType); !day.IsZero() {
				out = append(out, day)
			}
		}
	}
	return out, nil
}

func scheduleDaily(entryRule EntryRule, barDates []time.Time) ([]time.Time, error) {
	var out []time.Time
	for candidate := entryRule.StartDate; !candidate.After(entryRule.EndDate); candidate = candidate.AddDate(0, 0, 1) {
		day := data.MatchBarDate(candidate, barDates, entryRule.DateMatchType)
		if !day.IsZero() {
			finalDay, err := CombineDateTime(day, entryRule.TimeOfDay, entryRule.Timezone)
			if err != nil {
				return nil, err
			}
			out = append(out, finalDay)
		}
	}
	return out, nil
}

// --- Internal Helpers ---

func validateEntryRule(mode string, entryRule EntryRule) error {
	if len(entryRule.NthList) == 0 && !(mode == "" || mode == ModeDailyTime || mode == "default") {
		return fmt.Errorf("nth_list is required for mode %s", entryRule.Mode)
	}
	return nil
}

func extractBarDates(barMap []data.Bar) []time.Time {
	dates := make([]time.Time, len(barMap))
	for i, b := range barMap {
		dates[i] = b.Date
	}
	return dates
}

func isWithinRange(candidate, startDate, endDate time.Time) bool {
	return !candidate.Before(startDate) && !candidate.After(endDate)
}

func finalizeDates(out []time.Time) []time.Time {
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return deduplicateDates(out)
}
