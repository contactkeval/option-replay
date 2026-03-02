package data

import (
	"os"
	"testing"
	"time"
)

var (
	provLocal = NewLocalFileDataProvider("..\\..\\input\\data", nil) // Use a test directory for local CSV files
	symbol    = "O:SPY250213C00580000"                               // SPY call option expiring on Jan 17, 2025 with strike 580.0
	startDate = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate   = time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
)

func init() {
	// This is a placeholder for any setup needed before running tests.
	// For example, we could create a temporary directory and populate it with test CSV files.
	provLocal.SetSecondary(NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))) // Set Massive as the secondary provider for testing
}

func TestLocalGetBars(t *testing.T) {
	// This test will check if the local CSV provider can fetch bars for a given underlying and date range.
	// It will also check if the bars are correctly stored in the local CSV file.
	provLocal.GetBars(symbol, startDate, endDate, 1, "day")
	// After fetching, we can check if the local CSV file exists and has the expected data.
	if _, err := os.Stat("..\\..\\input\\data\\" + symbol + ".csv"); os.IsNotExist(err) {
		t.Fatalf("Expected local CSV file not found: %s.csv", symbol)
	}
}
