package scheduler_test

// import (
// 	"testing"
// 	"time"

// 	"github.com/contactkeval/option-replay/internal/backtest/scheduler"
// 	"github.com/contactkeval/option-replay/internal/data"
// )

// func TestScheduleDates_PublicAPI(t *testing.T) {
// 	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
// 	endDate := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)

// 	// minimal bar map like a real caller would provide
// 	bars := []data.Bar{
// 		{Date: startDate},
// 		{Date: startDate.AddDate(0, 0, 1)},
// 		{Date: startDate.AddDate(0, 0, 2)},
// 	}

// 	entry := scheduler.EntryRule{
// 		Mode:      "daily_time",
// 		StartDate: startDate,
// 		EndDate:   endDate,
// 	}

// 	dates, err := scheduler.ScheduleDates(entry, bars, nil)
// 	if err != nil {
// 		t.Fatalf("unexpected error: %v", err)
// 	}

// 	if len(dates) == 0 {
// 		t.Fatalf("expected non-empty schedule")
// 	}

// 	for _, d := range dates {
// 		if d.Before(startDate) || d.After(endDate) {
// 			t.Fatalf("date out of range: %v", d)
// 		}
// 	}
// }
