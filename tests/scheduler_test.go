package tests

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest"
	"github.com/contactkeval/option-replay/internal/data"
)

var (
	locNY *time.Location
	start time.Time
	end   time.Time

	// struct massiveDataProvider is private, so we create via New...  --hence initialize and assign instead of just declaring prov variable
	prov  = data.NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
)

func init() {
	var err error
	locNY, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}

	start = time.Date(2025, 1, 1, 0, 0, 0, 0, locNY)
	end = time.Date(2026, 1, 1, 0, 0, 0, 0, locNY)
}

var update = flag.Bool("update", false, "update golden files")

//
// --- Golden file helpers ---
//

func writeGolden(t *testing.T, name string, v any) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	err = os.WriteFile(path, b, 0644)
	if err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}
}

func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}
	return b
}

func compareWithGolden(t *testing.T, name string, v any) {
	t.Helper()

	actual, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal actual JSON: %v", err)
	}

	if *update {
		writeGolden(t, name, v)
		return
	}

	expected := loadGolden(t, name)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s",
			name, string(expected), string(actual))
	}
}

//
// --- Tests rewritten to golden-file style
//

func TestBeforeEarningsSchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_earnings_schedule", dates)
}

func TestBeforeEarningsHigherSchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "GOOG",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchHigher,
		Start:         start,
		End:           end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_earnings_higher_schedule", dates)
}

func TestBeforeEarningsLowerSchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "META",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchLower,
		Start:         start,
		End:           end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_earnings_lower_schedule", dates)
}

func TestBeforeEarningsExactSchedule(t *testing.T) {
	// TODO: test with MSFT which has earnings on non-trading days, change it to different stock if that changes
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "MSFT",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchExact,
		Start:         start,
		End:           end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_earnings_exact_schedule", dates)
}

func TestBeforeEarningsNearestSchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "NVDA",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchNearest,
		Start:         start,
		End:           end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_earnings_nearest_schedule", dates)
}

func TestAfterEarningsSchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying: "TSLA",
		NthList:    []int{5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "after_earnings_schedule", dates)
}

func TestBeforeExpirySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "expiry_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	// get list of expiries for the underlying during backtest period
	expiries, err := backtest.GetRelevantExpiries(entryRule.Underlying, entryRule.Start, entryRule.End, prov)
	if err != nil {
		t.Fatalf("backtest scheduler error: get relevant expiries error, %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, expiries) // TODO: pass expiries instead of nil
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "before_expiry_schedule", dates)
}

func TestOnceMonthlySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "once_monthly_schedule", dates)
}

func TestThriceMonthlySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{10, 20, 30},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "thrice_monthly_schedule", dates)
}

func TestOnceWeeklySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "once_weekly_schedule", dates)
}

func TestThriceWeeklySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1, 3, 5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        start.AddDate(0, 3, -1)})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "thrice_weekly_schedule", dates)
}

func TestDailySchedule(t *testing.T) {
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "", // daily_time is default
		Underlying: "AAPL",
		TimeOfDay:  "10:00",
		Start:      start,
		End:        start.AddDate(0, 1, -1)})
	bars, err := prov.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "daily_schedule", dates)
}
