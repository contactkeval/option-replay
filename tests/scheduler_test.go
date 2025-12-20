package tests

import (
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest"
)

func TestBeforeEarningsSchedule(t *testing.T) {
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "GOOG",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchHigher,
		Start:         start,
		End:           end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "META",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchLower,
		Start:         start,
		End:           end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	// TODO: test with MSFT which has earnings on non-trading days, change it to different stock if that changes
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "MSFT",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchExact,
		Start:         start,
		End:           end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying:    "NVDA",
		NthList:       []int{-5},
		TimeOfDay:     "10:00",
		DateMatchType: backtest.MatchNearest,
		Start:         start,
		End:           end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "earnings_offset",
		Underlying: "TSLA",
		NthList:    []int{5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "expiry_offset",
		Underlying: "AAPL",
		NthList:    []int{-5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	// get list of expiries for the underlying during backtest period
	expiries, err := backtest.GetRelevantExpiries(entryRule.Underlying, entryRule.Start, entryRule.End, dataProv)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_month_day",
		Underlying: "AAPL",
		NthList:    []int{10, 20, 30},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        end})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "nth_weekday",
		Underlying: "AAPL",
		NthList:    []int{1, 3, 5},
		TimeOfDay:  "10:00",
		Start:      start,
		End:        start.AddDate(0, 3, -1)})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
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
	dataProv = getMassiveDataProvider()
	entryRule := backtest.NewEntryRule(backtest.EntryRule{Mode: "", // daily_time is default
		Underlying: "AAPL",
		TimeOfDay:  "10:00",
		Start:      start,
		End:        start.AddDate(0, 1, -1)})
	bars, err := dataProv.GetDailyBars(entryRule.Underlying, entryRule.Start, entryRule.End)
	if err != nil {
		t.Fatalf("failed to get daily bars: %v", err)
	}

	dates, err := backtest.ResolveScheduleDates(*entryRule, bars, nil)
	if err != nil {
		t.Fatalf("failed to resolve schedule dates: %v", err)
	}

	compareWithGolden(t, "daily_schedule", dates)
}
