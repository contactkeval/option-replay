package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func TestEnsureSpotContractsAndLatestBar(t *testing.T) {
	database := openSpotTestDB(t)

	if err := database.EnsureSpotContracts([]string{"spy", "QQQ", "SPY"}); err != nil {
		t.Fatal(err)
	}
	spots, err := database.ListSpotContracts()
	if err != nil {
		t.Fatal(err)
	}
	if len(spots) != 2 {
		t.Fatalf("spots=%d want 2", len(spots))
	}
	if spots[0].Underlying != "QQQ" || spots[1].Underlying != "SPY" {
		t.Fatalf("order %+v", spots)
	}
	if !IsSpotContract(spots[0]) || spots[0].Strike != 0 {
		t.Fatalf("spot contract %+v", spots[0])
	}

	stale, last, err := database.SpotBarsStale("SPY", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !stale || !last.IsZero() {
		t.Fatalf("expected missing/stale, stale=%v last=%v", stale, last)
	}

	fresh := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	windowStart := uint32(fresh.Unix())
	bar := config.TransientRow{
		Ticker: "SPY",
		ParquetRow: config.ParquetRow{
			WindowStart: windowStart,
			Open:        10000,
			High:        10000,
			Low:         10000,
			Close:       10000,
			Volume:      100,
		},
	}

	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InsertSpotBars(tx, []config.TransientRow{bar}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	stale, last, err = database.SpotBarsStale("SPY", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("expected fresh")
	}
	if last.Unix() != int64(windowStart) {
		t.Fatalf("last=%s want %s", last, fresh)
	}
}

func TestSpotBarsStale_OlderThanMonth(t *testing.T) {
	database := openSpotTestDB(t)
	if err := database.EnsureSpotContracts([]string{"SPY"}); err != nil {
		t.Fatal(err)
	}

	old := time.Now().UTC().AddDate(0, 0, -45).Truncate(time.Second)
	windowStart := uint32(old.Unix())
	bar := config.TransientRow{
		Ticker: "SPY",
		ParquetRow: config.ParquetRow{
			WindowStart: windowStart,
			Open:        10000,
			High:        10000,
			Low:         10000,
			Close:       10000,
			Volume:      100,
		},
	}

	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InsertSpotBars(tx, []config.TransientRow{bar}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	stale, last, err := database.SpotBarsStale("SPY", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("expected stale")
	}
	if last.Unix() != int64(windowStart) {
		t.Fatalf("last=%s want %s", last, old)
	}
}

func openSpotTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spot.db")
	database, err := Open(Options{
		Path:    path,
		Schemas: SchemaContracts | SchemaDownload | SchemaTransient,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
