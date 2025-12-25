package scheduler_test

import (
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest/scheduler"
	"github.com/contactkeval/option-replay/internal/data"
)

func TestResolveScheduleDates_PublicAPI(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)

	// minimal bar map like a real caller would provide
	bars := []data.Bar{
		{Date: start},
		{Date: start.AddDate(0, 0, 1)},
		{Date: start.AddDate(0, 0, 2)},
	}

	entry := scheduler.EntryRule{
		Mode:  "daily_time",
		Start: start,
		End:   end,
	}

	dates, err := scheduler.ResolveScheduleDates(entry, bars, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(dates) == 0 {
		t.Fatalf("expected non-empty schedule")
	}

	for _, d := range dates {
		if d.Before(start) || d.After(end) {
			t.Fatalf("date out of range: %v", d)
		}
	}
}
