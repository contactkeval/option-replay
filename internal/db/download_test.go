package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func TestRecordContractFetch_AddsNewBars(t *testing.T) {
	database := openDBTest(t)
	mustExecDB(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike, groupNo,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES (1, 'A', '2026-12-01', 'call', 1, 0, '2026-01-01', '2026-01-01', 10, 0)
	`)

	if err := database.RecordContractFetch(1, 3, time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	var barCount int
	if err := database.QueryRow(`SELECT barCount FROM contracts WHERE serialNo = 1`).Scan(&barCount); err != nil {
		t.Fatal(err)
	}
	if barCount != 13 {
		t.Fatalf("want barCount 13, got %d", barCount)
	}
}

func TestInsertCandleStaging_CountsNewInsertsOnly(t *testing.T) {
	database := openDBTest(t)
	candle := config.Candle{Time: 1_700_000_000_000, Open: 1, High: 1, Low: 1, Close: 1}

	inserted, err := database.InsertCandleStaging(1, candle, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first insert should be new")
	}

	inserted, err = database.InsertCandleStaging(1, candle, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate candle should be ignored")
	}
}

func TestInsertCandleStagingBatch(t *testing.T) {
	database := openDBTest(t)
	rows := []CandleStagingRow{
		{SerialNo: 1, Candle: config.Candle{Time: 1, Open: 1, High: 1, Low: 1, Close: 1}, RunNo: 1, BatchNo: 1},
		{SerialNo: 1, Candle: config.Candle{Time: 2, Open: 1, High: 1, Low: 1, Close: 1}, RunNo: 1, BatchNo: 1},
		{SerialNo: 1, Candle: config.Candle{Time: 1, Open: 1, High: 1, Low: 1, Close: 1}, RunNo: 1, BatchNo: 1},
	}
	n, perSerial, err := database.InsertCandleStagingBatch(rows)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 new rows, got %d", n)
	}
	if perSerial[1] != 2 {
		t.Fatalf("want 2 for serial 1, got %d", perSerial[1])
	}
}

func TestGetLatestRunNo(t *testing.T) {
	database := openDBTest(t)

	if _, err := database.GetLatestRunNo(); err == nil {
		t.Fatal("expected error when no runs exist")
	}

	mustExecDB(t, database, `
		INSERT INTO runs (runNo, groupNo, runDateTime, contractCount, batchCount)
		VALUES (3, -1, '2026-08-14T00:00:00Z', 0, 1)
	`)
	mustExecDB(t, database, `
		INSERT INTO runs (runNo, groupNo, runDateTime, contractCount, batchCount)
		VALUES (7, -1, '2026-08-14T01:00:00Z', 0, 2)
	`)

	runNo, err := database.GetLatestRunNo()
	if err != nil {
		t.Fatal(err)
	}
	if runNo != 7 {
		t.Fatalf("want latest run 7, got %d", runNo)
	}
}

func openDBTest(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(Options{
		Path:    path,
		Schemas: SchemaContracts | SchemaDownload,
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func mustExecDB(t *testing.T, database *DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, query)
	}
}
