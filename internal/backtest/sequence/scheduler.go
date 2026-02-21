// Package sequence computes the chronological timeline for a backtest.
// It maps high-level scheduling rules (e.g., "5 days before earnings")
// into a concrete, sorted sequence of opening trade timestamps.
package sequence

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
)

type EarningsResponse struct {
	// Need quarterly earnings dates only
	QuarterlyEarnings []struct {
		ReportedDate string `json:"reportedDate"`
	} `json:"quarterlyEarnings"`
}

type EntryRule struct {
	StDt              string             `json:"start,omitempty"` // inclusive, default: one year before now
	EnDt              string             `json:"end,omitempty"`   // inclusive, YYYY-MM-DD format date only, default: now
	StartDate         time.Time          // inclusive, default: one year before now
	EndDate           time.Time          // inclusive, YYYY-MM-DD format date only, default: now
	Underlying        string             `json:"underlying,omitempty"`      // e.g., "AAPL", "SPY", etc.
	Mode              string             `json:"mode"`                      // "earnings_offset", "expiry_offset", "nth_weekday", "nth_month_day", "daily_time"
	NthList           []int              `json:"nth_list,omitempty"`        // e.g., [-5] or [5] for 5 days prior or after respectively (for earnings_offset, expiry_offset), [1,3], etc. for nth_weekday or nth_month_day
	DateMatchType     data.DateMatchType `json:"date_match_type,omitempty"` // "exact", "higher", "lower", "nearest", default: "nearest"
	TimeOfDay         string             `json:"time_of_day,omitempty"`     // "09:30", "10:00", etc., default: "09:30"
	Timezone          string             `json:"timezone,omitempty"`        // full IANA names (https://datetime.app/iana-timezones e.g. Asia/Kolkata), default: "America/New_York"
	MonthlyExpiryOnly bool               `json:"monthly_only,omitempty"`    // for expiry_offset mode, default: false
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
	if w.StartDate.IsZero() {
		w.StartDate = now.AddDate(-1, 0, 0)
	}
	if w.EndDate.IsZero() {
		w.EndDate = now
	}

	// If start > end, swap
	if w.StartDate.After(w.EndDate) {
		w.StartDate, w.EndDate = w.EndDate, w.StartDate
	}

	// Set default time zone if missing
	if w.TimeOfDay == "" {
		w.TimeOfDay = "09:30"
	}

	// Set default timezone if missing
	if w.Timezone == "" {
		w.Timezone = "America/New_York"
	}

	// Set default underlying if missing
	if w.Underlying == "" {
		w.Underlying = "SPY"
	}

	// Set default date match type
	if w.DateMatchType == "" {
		w.DateMatchType = data.MatchNearest
	}

	// Set default monthly expiry only to false
	// (no action needed as bool zero value is false)

	return &w
}

// GetEarningsDates retrieves reported quarterly earnings dates for the given
// underlying symbol from the Alpha Vantage "EARNINGS" API.
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
func GetEarningsDates(underlying string) ([]time.Time, error) {
	apiKey := os.Getenv("ALPHAVANTAGE_API_KEY")
	if apiKey == "" {
		logger.Warnf("ALPHAVANTAGE_API_KEY not set. Earnings-based scheduling will fail.")
		return nil, fmt.Errorf("environment: ALPHAVANTAGE_API_KEY is not set")
	}

	url := fmt.Sprintf("https://www.alphavantage.co/query?function=EARNINGS&symbol=%s&apikey=%s", underlying, apiKey)
	logger.Debugf("Querying AlphaVantage for %s earnings...", underlying)

	resp, err := http.Get(url)
	if err != nil {
		logger.Errorf("HTTP error fetching earnings for %s: %v", underlying, err)
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var er EarningsResponse
	if err := json.Unmarshal(body, &er); err != nil {
		logger.Errorf("Failed to parse earnings JSON: %v", err)
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	var out []time.Time
	for _, q := range er.QuarterlyEarnings {
		if t, err := time.Parse("2006-01-02", q.ReportedDate); err == nil {
			out = append(out, t)
		}
	}
	logger.Debugf("Found %d quarterly earnings dates for %s", len(out), underlying)
	return out, nil
}
