package scheduler

import (
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	locNY      *time.Location
	startDate  time.Time
	endDate    time.Time
	timespan   int
	multiplier string

	dataProv data.Provider
)

func init() {
	var err error
	locNY, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}

	startDate = time.Date(2025, 1, 1, 0, 0, 0, 0, locNY)
	endDate = time.Date(2026, 1, 1, 0, 0, 0, 0, locNY)

	timespan = 1
	multiplier = "day"

	dataProv = data.GetMassiveDataProvider()
}

func TestBeforeEarningsSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_earnings_schedule", dates)
}

func TestBeforeEarningsHigherSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying:    "GOOG",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: data.MatchHigher,
		StartDate:     startDate,
		EndDate:       endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_earnings_higher_schedule", dates)
}

func TestBeforeEarningsLowerSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying:    "META",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: data.MatchLower,
		StartDate:     startDate,
		EndDate:       endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_earnings_lower_schedule", dates)
}

func TestBeforeEarningsExactSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying:    "MSFT",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: data.MatchExact,
		StartDate:     startDate,
		EndDate:       endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_earnings_exact_schedule", dates)
}

func TestBeforeEarningsNearestSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying:    "NVDA",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: data.MatchNearest,
		StartDate:     startDate,
		EndDate:       endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_earnings_nearest_schedule", dates)
}

func TestAfterEarningsSchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "earnings_offset",
		Underlying: "TSLA",
		NthList:    []int{5},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "after_earnings_schedule", dates)
}

func TestBeforeExpirySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "expiry_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	// get list of expiries for the underlying during back test period
	expiries, err := dataProv.GetRelevantExpiries(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate)
	if err != nil {
		t.Fatalf("back test scheduler error: get relevant expiries error, %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, expiries)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "before_expiry_schedule", dates)
}

func TestOnceMonthlySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "once_monthly_schedule", dates)
}

func TestThriceMonthlySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{10, 20, 30},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "thrice_monthly_schedule", dates)
}

func TestOnceWeeklySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    endDate})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "once_weekly_schedule", dates)
}

func TestThriceWeeklySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1, 3, 5},
		TimeOfDay:  "10:00",
		StartDate:  startDate,
		EndDate:    startDate.AddDate(0, 3, -1)})
	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "thrice_weekly_schedule", dates)
}

func TestDailySchedule(t *testing.T) {
	entryRule := NewEntryRule(EntryRule{Mode: "", // daily_time is default
		Underlying: "AAPL",
		TimeOfDay:  "10:00",
		Timezone:   "America/New_York",
		StartDate:  startDate,
		EndDate:    startDate.AddDate(0, 1, -1)})

	bars, err := dataProv.GetBars(entryRule.Underlying, entryRule.StartDate, entryRule.EndDate, timespan, multiplier)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := ScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to schedule dates: %v", err)
	}

	tests.CompareWithGolden(t, "daily_schedule", dates)
}
