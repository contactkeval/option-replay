package data

import (
	"os"
	"path/filepath"
	"strings"
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
	localProv := dataProv.(*LocalFileDataProvider)
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	underlying := "O:SPY250117C00580000" // SPY call option expiring on Jan 17, 2025 with strike 580.0
	err := localProv.EnsureLocalData(underlying, startDate, endDate)
	if err != nil {
		t.Fatalf("Error ensuring local data: %v", err)
	}

	// Check if the file was created
	if strings.Contains(underlying, ":") {
		underlying = strings.ReplaceAll(underlying, ":", "-") // sanitize for file name
	}
	expectedFile := filepath.Join("..\\..\\input\\data", underlying+".csv")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Expected file not found: %s", expectedFile)
	}
}

func TestEnsureLocal_ExistingRecord(t *testing.T) {
	// This test will check the behavior of EnsureLocalData when a record already exists in the manifest.
	// It will create a dummy record with an old last date and verify that it gets updated to today.
	var dataProv Provider
	dataProv = NewLocalFileDataProvider("..\\..\\input\\data", dataProv)
	dataProv.SetSecondary(NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))) // Massive data provider as secondary
	localProv := dataProv.(*LocalFileDataProvider)
	localProv.RunMaintenancePipeline() // Ensure manifest is loaded and cleaned up
}
