package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 23, 13, 30, 0, 0, loc)
	days := calendarDays(now, 7, loc)
	if len(days) != 7 {
		t.Fatalf("len=%d", len(days))
	}
	if got, want := days[0].Format("2006-01-02"), "2026-08-17"; got != want {
		t.Fatalf("first=%s want %s", got, want)
	}
	if got, want := days[6].Format("2006-01-02"), "2026-08-23"; got != want {
		t.Fatalf("last=%s want %s", got, want)
	}
}

func TestCountBarsByDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "AAPL.csv")
	body := strings.Join([]string{
		"date,open,high,low,close,volume",
		"2026-08-20T13:30:00Z,1,1,1,1,1",
		"2026-08-20T13:31:00Z,1,1,1,1,1",
		"2026-08-21T20:00:00Z,1,1,1,1,1",
		"2026-08-23T14:00:00Z,1,1,1,1,1",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 23, 13, 30, 0, 0, loc)
	days := calendarDays(now, 7, loc)
	counts, err := countBarsByDay(path, days, loc)
	if err != nil {
		t.Fatal(err)
	}
	// 08-20 09:30 and 09:31 ET, 08-21 16:00 ET, 08-23 10:00 ET
	want := []int{0, 0, 0, 2, 1, 0, 1}
	if len(counts) != len(want) {
		t.Fatalf("counts=%v", counts)
	}
	for i := range want {
		if counts[i] != want[i] {
			t.Fatalf("counts=%v want %v", counts, want)
		}
	}
}

func TestWriteBarCountTableCSV(t *testing.T) {
	loc := time.UTC
	days := calendarDays(time.Date(2026, 8, 23, 0, 0, 0, 0, loc), 2, loc)
	var buf strings.Builder
	err := writeBarCountTable(&buf, []string{"AAPL", "XSP"}, days, map[string][]int{
		"AAPL": {1, 2},
	}, map[string]error{
		"XSP": fmt.Errorf("status 403"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "symbol,2026-08-22,2026-08-23,total,error") {
		t.Fatalf("header: %q", got)
	}
	if !strings.Contains(got, "AAPL,1,2,3,") {
		t.Fatalf("aapl: %q", got)
	}
	if !strings.Contains(got, "XSP,,,") || !strings.Contains(got, "status 403") {
		t.Fatalf("xsp: %q", got)
	}
}
