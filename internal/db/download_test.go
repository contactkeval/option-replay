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
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES (1, 'A', '2026-12-01', 'call', 1, '2026-01-01', '2026-01-01', 10, 0)
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

func TestRecordContractFetch_DownloadAttemptsByExpiry(t *testing.T) {
	database := openDBTest(t)
	mustExecDB(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, downloadAttempts, archived
		) VALUES
			(1, 'A', '2026-12-01', 'call', 1, '2026-01-01', '2026-01-01', 0, 0, 0),
			(2, 'A', '2026-08-14', 'call', 1, '2026-01-01', '2026-01-01', 0, 0, 0),
			(3, 'A', '2026-08-01', 'call', 1, '2026-01-01', '2026-01-01', 0, 2, 0)
	`)

	fetchDate := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	for _, serial := range []int64{1, 2, 3} {
		if err := database.RecordContractFetch(serial, 1, fetchDate); err != nil {
			t.Fatal(err)
		}
	}

	var futureAttempts, todayAttempts, pastAttempts float64
	var pastArchived int
	if err := database.QueryRow(`SELECT downloadAttempts FROM contracts WHERE serialNo = 1`).Scan(&futureAttempts); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT downloadAttempts FROM contracts WHERE serialNo = 2`).Scan(&todayAttempts); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`
		SELECT downloadAttempts, archived FROM contracts WHERE serialNo = 3
	`).Scan(&pastAttempts, &pastArchived); err != nil {
		t.Fatal(err)
	}

	if futureAttempts < 0.0009 || futureAttempts > 0.0011 {
		t.Fatalf("future expiry attempts=%v want ~0.001", futureAttempts)
	}
	if todayAttempts != 1 {
		t.Fatalf("current expiry attempts=%v want 1", todayAttempts)
	}
	if pastAttempts != 3 {
		t.Fatalf("past expiry attempts=%v want 3", pastAttempts)
	}
	if pastArchived != 1 {
		t.Fatal("past expiry with 3 attempts should be archived")
	}
}

func TestDownloadBarCountStats(t *testing.T) {
	database := openDBTest(t)
	mustExecDB(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES
			(1, 'A', '2026-12-01', 'call', 1, '2026-01-01', '2026-01-01', 0, 0),
			(2, 'A', '2026-12-01', 'put', 1, '2026-01-01', '2026-01-01', 0, 0)
	`)
	mustExecDB(t, database, `
		INSERT INTO runs (runNo, groupNo, runDateTime, contractCount, batchCount)
		VALUES (1, -1, '2026-08-14T00:00:00Z', 2, 1)
	`)
	mustExecDB(t, database, `
		INSERT INTO batches (runNo, batchNo, contractCount)
		VALUES (1, 1, 2)
	`)
	mustExecDB(t, database, `
		INSERT INTO batch_contracts (runNo, batchNo, serialNo, listNo)
		VALUES (1, 1, 1, 1), (1, 1, 2, 2)
	`)

	if err := database.UpdateBatchDownloadStats(1, 1, "2026-08-14T01:00:00Z", 10, 7); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateBatchContractDownloadStats(1, 1, 1, 6, 4); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateBatchContractDownloadStats(1, 1, 2, 4, 3); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordContractFetch(1, 4, time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordContractFetch(2, 3, time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshRunDownloadStats(1); err != nil {
		t.Fatal(err)
	}

	var batchBars, batchNew int64
	if err := database.QueryRow(`
		SELECT barCount, newBarCount FROM batches WHERE runNo = 1 AND batchNo = 1
	`).Scan(&batchBars, &batchNew); err != nil {
		t.Fatal(err)
	}
	if batchBars != 10 || batchNew != 7 {
		t.Fatalf("batch stats bars=%d new=%d", batchBars, batchNew)
	}

	var sumNew int64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(newBarCount), 0) FROM batch_contracts WHERE runNo = 1
	`).Scan(&sumNew); err != nil {
		t.Fatal(err)
	}
	if sumNew != 7 {
		t.Fatalf("sum batch_contracts.newBarCount=%d", sumNew)
	}

	var contractBars int64
	if err := database.QueryRow(`
		SELECT COALESCE(SUM(barCount), 0) FROM contracts WHERE serialNo IN (1, 2)
	`).Scan(&contractBars); err != nil {
		t.Fatal(err)
	}
	if contractBars != sumNew {
		t.Fatalf("contracts.barCount sum=%d want %d", contractBars, sumNew)
	}

	var runBars, runNew int64
	if err := database.QueryRow(`
		SELECT barCount, newBarCount FROM runs WHERE runNo = 1
	`).Scan(&runBars, &runNew); err != nil {
		t.Fatal(err)
	}
	if runBars != 10 || runNew != 7 {
		t.Fatalf("run stats bars=%d new=%d", runBars, runNew)
	}

	if has, err := tableHasColumn(database.DB, "batches", "candleCount"); err != nil {
		t.Fatal(err)
	} else if has {
		t.Fatal("batches.candleCount should be dropped")
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
