package data

import (
	"os"
	"testing"
	"time"
)

func testDateRange() (time.Time, time.Time) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	return start, end
}

func TestDataProviderContract_GetDailyBars(t *testing.T) {
	start, end := testDateRange()

	providers := []struct {
		name     string
		provider Provider
	}{
		{
			name:     "massive",
			provider: NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")),
		},
		// TODO: add more providers here
	}

	for _, prov := range providers {
		t.Run(prov.name, func(t *testing.T) {
			bars, err := prov.provider.GetDailyBars("AAPL", start, end)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(bars) == 0 {
				t.Fatalf("expected non-empty bars")
			}

			for _, b := range bars {
				if b.Date.Before(start) || b.Date.After(end) {
					t.Fatalf("bar date out of range: %v", b.Date)
				}
			}
		})
	}
}
