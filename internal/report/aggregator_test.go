package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNextRunIDIncrementsFromExistingOutputs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.Local)
	day := now.Format("20060102")

	if err := os.WriteFile(filepath.Join(dir, "trades_"+day+"00001.csv"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "data_"+day+"00002"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := nextRunID(dir, now)
	if err != nil {
		t.Fatalf("nextRunID: %v", err)
	}
	want := day + "00003"
	if got != want {
		t.Fatalf("nextRunID=%s want %s", got, want)
	}
}

func TestNextRunIDStartsAtOne(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.Local)

	got, err := nextRunID(dir, now)
	if err != nil {
		t.Fatalf("nextRunID: %v", err)
	}
	if got != now.Format("20060102")+"00001" {
		t.Fatalf("nextRunID=%s", got)
	}
}

func TestFormatReportTime(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2025, 12, 26, 15, 40, 0, 0, loc)
	if got := formatReportTime(ts, loc); got != "2025-12-26 15:40" {
		t.Fatalf("got %q", got)
	}
	utc := time.Date(2025, 12, 26, 20, 40, 0, 0, time.UTC)
	if got := formatReportTime(utc, loc); got != "2025-12-26 15:40" {
		t.Fatalf("utc convert got %q", got)
	}
}
