package main

import (
	"reflect"
	"testing"
	"time"

	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

func TestNearestStep(t *testing.T) {
	cases := []struct {
		price, step, want float64
	}{
		{7648.4, 5, 7650},
		{7646.1, 5, 7645},
		{7642.5, 5, 7645}, // half away from zero
		{0, 5, 0},
	}
	for _, c := range cases {
		if got := nearestStep(c.price, c.step); got != c.want {
			t.Fatalf("nearestStep(%v, %v) = %v, want %v", c.price, c.step, got, c.want)
		}
	}
}

func TestStrikesAround(t *testing.T) {
	got := strikesAround(7645, 5, 3)
	want := []float64{7630, 7635, 7640, 7645, 7650, 7655, 7660}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strikesAround = %v, want %v", got, want)
	}
}

func TestNextWeekdayDates(t *testing.T) {
	// 2026-09-12 is a Saturday -> next weekdays are Mon 14 and Tue 15.
	from := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	got := nextWeekdayDates(from, 2)
	want := []time.Time{
		time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nextWeekdayDates = %v, want %v", got, want)
	}

	// 2026-09-11 is a Friday -> Fri 11 and Sat? no, Fri + Mon.
	from = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	got = nextWeekdayDates(from, 2)
	want = []time.Time{
		time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nextWeekdayDates = %v, want %v", got, want)
	}
}

func TestOptionSymbols(t *testing.T) {
	calls := optionSymbols("SPXW", []string{"260911"}, []float64{7640, 7645}, true)
	want := []string{".SPXW260911C7640", ".SPXW260911C7645"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}

	puts := optionSymbols("SPXW", []string{"260911"}, []float64{7640}, false)
	wantPut := []string{".SPXW260911P7640"}
	if !reflect.DeepEqual(puts, wantPut) {
		t.Fatalf("puts = %v, want %v", puts, wantPut)
	}
}

func TestStateApplyAndGrid(t *testing.T) {
	s := newState()
	s.apply(stage2.LiveEvent{
		Kind: "Quote", Symbol: "SPX",
		Bid: 7648.2, Ask: 7648.9, BidSize: 1, AskSize: 2,
		Time: time.UnixMilli(1000).UTC(),
	})
	s.setGrid(7645, []float64{7630, 7645, 7660}, []time.Time{
		time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
	}, "SPXW")
	s.setConnected(true)

	snap := s.snapshot()
	if snap.SPX.Bid != 7648.2 || snap.SPX.Ask != 7648.9 {
		t.Fatalf("spx snap mismatch: %+v", snap.SPX)
	}
	if snap.Anchor != 7645 {
		t.Fatalf("anchor mismatch: %v", snap.Anchor)
	}
	if len(snap.Calls) != 3 {
		t.Fatalf("want 3 calls, got %d", len(snap.Calls))
	}
	if !s.connected {
		t.Fatal("connection flag not set")
	}
}

func TestNextBoundary(t *testing.T) {
	loc := time.FixedZone("t", 5*60*60+30*60) // +05:30, matches the user's local window expectations
	cases := []struct {
		in, want string
	}{
		{"2026-09-14T08:52:00", "2026-09-14T09:00:00"},
		{"2026-09-14T09:08:00", "2026-09-14T09:10:00"},
		{"2026-09-14T09:00:00", "2026-09-14T09:00:00"},
		{"2026-09-14T09:19:59", "2026-09-14T09:20:00"},
		{"2026-09-14T23:55:00", "2026-09-15T00:00:00"},
	}
	for _, c := range cases {
		in, err := time.ParseInLocation("2006-01-02T15:04:05", c.in, loc)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		want, err := time.ParseInLocation("2006-01-02T15:04:05", c.want, loc)
		if err != nil {
			t.Fatalf("parse %q: %v", c.want, err)
		}
		if got := nextBoundary(in, 10*time.Minute); !got.Equal(want) {
			t.Errorf("nextBoundary(%s) = %s, want %s", c.in, got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	}
}
