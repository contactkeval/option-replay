package backtest

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
)

type DateMatchType string

const (
	MatchExact   DateMatchType = "exact"   // must match exactly
	MatchHigher  DateMatchType = "higher"  // next available date after target
	MatchLower   DateMatchType = "lower"   // last available date before target
	MatchNearest DateMatchType = "nearest" // closest available date (default)
)

type EarningsResponse struct {
	// TODO : remove commented code if not needed
	// AnnualEarnings []struct {
	// 	FiscalDateEnding string `json:"fiscalDateEnding"`
	// } `json:"annualEarnings"`

	// Need quarterly earnings dates only
	QuarterlyEarnings []struct {
		ReportedDate string `json:"reportedDate"`
	} `json:"quarterlyEarnings"`
}

type EntryRule struct {
	Start             time.Time     `json:"start,omitempty"`           // inclusive, default: one year before now
	End               time.Time     `json:"end,omitempty"`             // inclusive, default: now
	Underlying        string        `json:"underlying,omitempty"`      // e.g., "AAPL", "SPY", etc.
	Mode              string        `json:"mode"`                      // "earnings_offset", "expiry_offset", "nth_weekday", "nth_month_day", "daily_time"
	NthList           []int         `json:"nth_list,omitempty"`        // e.g., [-5] or [5] for 5 days prior or after respectively (for earnings_offset, expiry_offset), [1,3], etc. for nth_weekday or nth_month_day
	DateMatchType     DateMatchType `json:"date_match_type,omitempty"` // "exact", "higher", "lower", "nearest"
	TimeOfDay         string        `json:"time_of_day,omitempty"`     // "09:30", "10:00", etc.
	Timezone          string        `json:"timezone,omitempty"`        // "EST", "PST", etc.
	MonthlyExpiryOnly bool          `json:"monthly_only,omitempty"`    // for expiry_offset mode, default: false
}

// NewEntryRule constructs and returns a *EntryRule populated with sensible defaults
// and normalized date ordering.
//
// The function accepts a EntryRule by value, applies the following rules to the
// copy, and returns a pointer to the modified copy:
//
// - If Start is the zero time, it is set to one year before the current time (UTC).
// - If End is the zero time, it is set to the current time (UTC).
// - If Start is after End, Start and End are swapped so that Start <= End.
// - If Timezone is empty, it defaults to "EST".
// - If Underlying is empty, it defaults to "SPY".
// - Monthly expiry remains false when left at its zero value (no explicit change).
//
// Notes:
//   - The function uses time.Now().UTC() to derive default Start and End values.
//   - Because the parameter is passed by value, the original EntryRule argument is
//     not mutated; a pointer to the modified copy is returned.
func NewEntryRule(w EntryRule) *EntryRule {
	now := time.Now().UTC()

	// Apply defaults if zero dates provided
	if w.Start.IsZero() {
		w.Start = now.AddDate(-1, 0, 0)
	}
	if w.End.IsZero() {
		w.End = now
	}

	// If start > end, swap
	if w.Start.After(w.End) {
		w.Start, w.End = w.End, w.Start
	}

	// Set default timezone if missing
	if w.Timezone == "" {
		w.Timezone = "EST"
	}

	// Set default underlying if missing
	if w.Underlying == "" {
		w.Underlying = "SPY"
	}

	// Set default date match type
	if w.DateMatchType == "" {
		w.DateMatchType = MatchNearest
	}

	// Set default monthly expiry only to false
	// (no action needed as bool zero value is false)

	return &w
}

// ResolveScheduleDates computes a list of trading dates for a backtest entry rule
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
//     parameters for a mode (e.g., missing Underlying or NthList), and for
//     failures when fetching external data (earnings or expiries). Mode-specific
//     errors wrap the underlying error to aid diagnosis.
//
// Parameters:
//   - when: scheduling rule describing mode and parameters (Mode, Underlying, Nth,
//     ExpiryCycle, Weekday, EveryNCalendar, etc.).
//   - start: inclusive lower bound of the scheduling window.
//   - end: inclusive upper bound of the scheduling window.
//   - barMap: a map of available market bars (indexed by date) used to snap candidates
//     to the nearest available trading date.
//
// Returns:
//   - []time.Time: sorted, unique list of scheduled trading dates (as time.Time).
func ResolveScheduleDates(entry EntryRule, barMap []data.Bar, expiries []time.Time) ([]time.Time, error) {
	now := time.Now().UTC()

	barDates := make([]time.Time, 0, len(barMap))
	for _, b := range barMap {
		barDates = append(barDates, b.Date)
	}

	// Default start = today - 1 year
	if entry.Start.IsZero() {
		entry.Start = now.AddDate(-1, 0, 0)
	}

	// Default end = today
	if entry.End.IsZero() {
		entry.End = now
	}

	out := []time.Time{}
	mode := strings.ToLower(strings.TrimSpace(entry.Mode))

	// invalid range
	if entry.Start.After(entry.End) {
		return out, fmt.Errorf("backtest scheduler error: invalid date range: start %v is after end %v", entry.Start, entry.End)
	}

	switch mode {

	// ----------------------------------------------------------------------------------------
	// earnings_offset - e.g., NthList = [-5] means 5 days before earnings
	// ----------------------------------------------------------------------------------------
	case "earnings_offset":
		if entry.Underlying == "" {
			return out, fmt.Errorf("backtest scheduler error: earnings_offset mode requires non-empty underlying")
		}

		earnings, err := GetEarningsDates(entry.Underlying)
		if err != nil {
			return out, fmt.Errorf("backtest scheduler error: fetch earnings dates error, %w", err)
		}

		offset := entry.NthList[0]
		for _, e := range earnings {
			candidate := e.AddDate(0, 0, offset)

			// candidate must be within range
			if candidate.Before(entry.Start) || candidate.After(entry.End) {
				continue
			}

			day := findBarDate(candidate, barDates, entry.DateMatchType)
			if !day.IsZero() {
				out = append(out, day)
			}
		}

	// ----------------------------------------------------------------------------------------
	// expiry_offset - e.g., NthList = [-5] means 5 days before expiry
	// ----------------------------------------------------------------------------------------
	case "expiry_offset":
		offset := entry.NthList[0]
		for _, e := range expiries {
			candidate := e.AddDate(0, 0, offset)

			// candidate must be within range
			if candidate.Before(entry.Start) || candidate.After(entry.End) {
				continue
			}

			day := findBarDate(candidate, barDates, entry.DateMatchType)
			if !day.IsZero() {
				out = append(out, day)
			}
		}

	// ----------------------------------------------------------------------------------------
	// nth_month_day — e.g., 10th of month, or [5, 15] of every month
	// ----------------------------------------------------------------------------------------
	case "nth_month_day":
		if len(entry.NthList) == 0 {
			return out, fmt.Errorf("nth_month_day mode requires NthList")
		}

		for y := entry.Start.Year(); y <= entry.End.Year(); y++ {
			for m := time.January; m <= time.December; m++ {
				monthStart := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
				monthEnd := monthStart.AddDate(0, 1, -1)

				if monthEnd.Before(entry.Start) || monthStart.After(entry.End) {
					continue
				}

				for _, dayNum := range entry.NthList {
					if dayNum < 1 || dayNum > 31 {
						continue
					}

					d := time.Date(y, m, dayNum, 0, 0, 0, 0, time.UTC)
					if d.Month() != m {
						continue // invalid day (e.g., Feb 30)
					}
					if d.Before(entry.Start) || d.After(entry.End) {
						continue
					}

					bar := findBarDate(d, barDates, entry.DateMatchType)
					if !bar.IsZero() {
						out = append(out, bar)
					}
				}
			}
		}

	// ----------------------------------------------------------------------------------------
	// nth_weekday - e.g., every Tue, Thu or every Mon etc.
	// ----------------------------------------------------------------------------------------
	case "nth_weekday":
		if len(entry.NthList) == 0 {
			return out, fmt.Errorf("nth_weekday mode requires NthList")
		}

		// Iterate through full date range
		cur := entry.Start
		for !cur.After(entry.End) {

			// Accept if day-of-week position matches NthList
			if intSliceContains(entry.NthList, int(cur.Weekday())) {
				day := findBarDate(cur, barDates, entry.DateMatchType)
				if !day.IsZero() {
					out = append(out, day)
				}
			}

			// next day
			cur = cur.AddDate(0, 0, 1)
		}

	// ----------------------------------------------------------------------------------------
	// default → daily schedule
	// ----------------------------------------------------------------------------------------
	default:
		for d := entry.Start; !d.After(entry.End); d = d.AddDate(0, 0, 1) {
			day := findBarDate(d, barDates, entry.DateMatchType)
			if !day.IsZero() {
				out = append(out, day)
			}
		}
	}

	// Sort + unique
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })

	seen := map[string]bool{}
	final := []time.Time{}
	for _, d := range out {
		k := d.Format("2006-01-02")
		if !seen[k] {
			final = append(final, d)
			seen[k] = true
		}
	}
	return final, nil
}

// ResolveExpiration computes and returns the expiration date for an option given an open date,
// a day offset and a list of candidate expiries.
//
// It first constructs a candidate date by adding the given offset (in calendar days) to openDate.
// It then selects and returns a matching date from the expiries slice according to dateMatchType.
// The offset may be positive, zero, or negative. The expiries slice should contain the available
// expiration dates (typically sorted); the exact selection behavior (e.g. exact match, nearest prior,
// nearest next) is governed by the provided DateMatchType and implemented by the underlying matching
// routine.
//
// Note: if no expiry satisfies the matching rules, the result depends on the matching implementation
// (it may return the zero time).
func ResolveExpiration(openDate time.Time, offset int, expiries []time.Time, dateMatchType DateMatchType) time.Time {
	candidate := openDate.AddDate(0, 0, offset)
	day := findBarDate(candidate, expiries, dateMatchType)

	return day
}

// GetRelevantExpiries returns a sorted slice of unique option expiration dates
// for a given ticker within the specified time range.
//
// The function determines relevant option strike prices by analyzing spot price data
// and selecting three middle strike levels within the price range, then retrieves
// all available contracts for those strikes to extract their expiration dates.
//
// Parameters:
//   - ticker: The symbol identifier (e.g., "SPY")
//   - start: The beginning of the date range for analysis
//   - end: The end of the date range for analysis
//   - provider: A data provider that supplies daily bars and contract information
//
// Returns:
//   - A sorted slice of unique time.Time values representing option expiration dates
//   - An error if spot data cannot be fetched, no data is available, or contract
//     retrieval fails
//
// The algorithm works as follows:
//  1. Fetches daily bar data for the ticker within the date range
//  2. Determines the high and low prices from the bars
//  3. Selects a rounding multiplier based on the low price
//  4. Divides the price range into 5 equal intervals and selects the middle 3 levels
//  5. Rounds strike prices to the nearest multiplier
//  6. Retrieves all available contracts for the rounded strike prices
//  7. Extracts and deduplicates expiration dates
//  8. Returns the sorted, unique expiration dates
func GetRelevantExpiries(ticker string, start, end time.Time, provider data.Provider) ([]time.Time, error) {

	// Step 1: Load spot bars
	bars, err := provider.GetDailyBars(ticker, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spot data: %w", err)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no spot data found")
	}

	// Step 2: Compute high & low
	low := bars[0].Low
	high := bars[0].High
	for _, b := range bars {
		if b.Low < low {
			low = b.Low
		}
		if b.High > high {
			high = b.High
		}
	}

	// Step 3: Determine multiplier
	multiplier := 1.0
	switch {
	case low >= 100 && low < 1000:
		multiplier = 10
	case low >= 1000 && low < 10000:
		multiplier = 100
	case low >= 10000:
		multiplier = 1000
	}

	// Step 4: Divide range into 5 equal intervals
	step := (high - low) / 5

	// Step 5: Pick middle 3 levels
	levels := []float64{
		low + step, // level 1
		// low + 2*step, // level 2
		low + 3*step, // level 3
	}

	// Step 6: Round levels to nearest multiplier
	roundedStrikes := make([]float64, len(levels))
	for i, v := range levels {
		roundedStrikes[i] = math.Round(v/multiplier) * multiplier
	}

	// Step 7: Fetch contracts for each strike
	expiryMap := map[string]time.Time{}

	for _, strike := range roundedStrikes {
		contracts, err := provider.GetContracts(ticker, strike, start, end)
		if err != nil {
			return nil, fmt.Errorf("fetch contracts strike %.2f: %w", strike, err)
		}

		for _, c := range contracts {
			key := c.ExpirationDate.Format("2006-01-02")
			expiryMap[key] = c.ExpirationDate
		}
	}

	// Step 8: Unique expiries & sorted slice
	expiries := make([]time.Time, 0, len(expiryMap))
	for _, dt := range expiryMap {
		expiries = append(expiries, dt)
	}

	sort.Slice(expiries, func(i, j int) bool {
		return expiries[i].Before(expiries[j])
	})

	return expiries, nil
}

// GetEarningsDates retrieves reported quarterly earnings dates for the given
// symbol from the Alpha Vantage "EARNINGS" API.
//
// The function expects the ALPHAVANTAGE_API_KEY environment variable to be set;
// it will return an error if the key is missing. It issues an HTTP GET to the
// Alpha Vantage earnings endpoint and attempts to decode the JSON response.
//
// On success, it returns a slice of time.Time values parsed from the
// "reportedDate" fields of the API's quarterly earnings. Dates are parsed
// using the layout "2006-01-02"; any quarterly entry whose date cannot be
// parsed is skipped. The order of returned dates matches the order provided by
// the API. If no parsable dates are found, an empty slice is returned.
//
// Possible errors include missing API key, network/HTTP errors, and JSON
// unmarshal errors from the API response.
func GetEarningsDates(symbol string) ([]time.Time, error) {
	apiKey := os.Getenv("ALPHAVANTAGE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("missing ALPHAVANTAGE_API_KEY")
	}

	url := fmt.Sprintf(
		"https://www.alphavantage.co/query?function=EARNINGS&symbol=%s&apikey=%s",
		symbol, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var er EarningsResponse
	if err := json.Unmarshal(body, &er); err != nil {
		return nil, err
	}

	out := []time.Time{}
	for _, q := range er.QuarterlyEarnings {
		if t, err := time.Parse("2006-01-02", q.ReportedDate); err == nil {
			out = append(out, t)
		}
	}
	return out, nil
}
