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
			bars, err := prov.provider.GetBars("AAPL", start, end, 1, "day")

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

// --------------------------------------------------------------------------------------------
// Helper function tests
// --------------------------------------------------------------------------------------------

func TestOptionSymbolFromParts(t *testing.T) {
	expDt := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
	symbol := OptionSymbolFromParts("SPY", expDt, "put", 500.0)
	expected := "O:SPY250113P00500000"
	if symbol != expected {
		t.Fatalf("expected %s, got %s", expected, symbol)
	}

	symbol = OptionSymbolFromParts("SPXW", expDt, "c", 5000.0)
	expected = "O:SPXW250113C05000000"
	if symbol != expected {
		t.Fatalf("expected %s, got %s", expected, symbol)
	}
}

func TestClosest(t *testing.T) {
	numList := []float64{100, 110, 120, 130, 140}

	// test below the lowest
	target := 75.0
	closest := Closest(numList, target)
	expected := 100.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test just below lowest
	target = 99.0
	closest = Closest(numList, target)
	expected = 100.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test lowest
	target = 100.0
	closest = Closest(numList, target)
	expected = 100.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test just above lowest
	target = 101.0
	closest = Closest(numList, target)
	expected = 100.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test middle value
	target = 115.0
	closest = Closest(numList, target)
	expected = 120.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test middle value
	target = 123.0
	closest = Closest(numList, target)
	expected = 120.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test just below highest value
	target = 139.0
	closest = Closest(numList, target)
	expected = 140.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test at highest value
	target = 140.0
	closest = Closest(numList, target)
	expected = 140.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	// test above highest value
	target = 141.0
	closest = Closest(numList, target)
	expected = 140.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}

	target = 155.0 // test above highest
	closest = Closest(numList, target)
	expected = 140.0
	if closest != expected {
		t.Fatalf("expected %f, got %f", expected, closest)
	}
}
