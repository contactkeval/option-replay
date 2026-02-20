package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureLocal(t *testing.T) {
	// This test is more of an integration test for the localCSV provider, but it also indirectly tests the helper functions.
	// It will check if the local CSV files are created and updated correctly when fetching data from the secondary provider.
	// Note: This test will actually create files in the "data" directory, so it should be run in a controlled environment.
	var dataProv Provider
	dataProv = NewLocalFileDataProvider("..\\..\\input\\data", dataProv)
	dataProv.SetSecondary(NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))) // Massive data provider as secondary
	localProv := dataProv.(*localFileDataProvider)
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	err := localProv.EnsureLocalData("SPY", startDate, endDate)
	if err != nil {
		t.Fatalf("Error ensuring local data: %v", err)
	}

	// Check if the file was created
	expectedFile := filepath.Join("..\\..\\input\\data", "SPY.csv")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Expected file not found: %s", expectedFile)
	}
}
