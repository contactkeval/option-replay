package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func TestOptionDelta(t *testing.T) {
	cur := []string{".SPXW260915C7625", ".SPXW260915C7630", ".SPXW260915C7635", ".SPXW260915P7625"}
	want := []string{".SPXW260915C7630", ".SPXW260915C7635", ".SPXW260915C7640", ".SPXW260915P7630", ".SPXW260915P7635", ".SPXW260915P7640"}
	add, remove := optionDelta(cur, want)
	wantAdd := []string{".SPXW260915C7640", ".SPXW260915P7630", ".SPXW260915P7635", ".SPXW260915P7640"}
	wantRemove := []string{".SPXW260915C7625", ".SPXW260915P7625"}
	if !reflect.DeepEqual(add, wantAdd) {
		t.Fatalf("optionDelta add = %v, want %v", add, wantAdd)
	}
	if !reflect.DeepEqual(remove, wantRemove) {
		t.Fatalf("optionDelta remove = %v, want %v", remove, wantRemove)
	}
	if a, r := optionDelta(want, want); len(a) != 0 || len(r) != 0 {
		t.Fatalf("identical sets must have empty delta: add=%v remove=%v", a, r)
	}
}

func TestStrikeOf(t *testing.T) {
	cases := []struct {
		sym  string
		want float64
		ok   bool
	}{
		{".SPXW260915C7645", 7645, true},
		{".SPXW260915P7625", 7625, true},
		{"SPX", 0, false},
		{".SPXW260915C", 0, false},
	}
	for _, c := range cases {
		got, ok := strikeOf(c.sym)
		if got != c.want || ok != c.ok {
			t.Fatalf("strikeOf(%q) = (%v, %v), want (%v, %v)", c.sym, got, ok, c.want, c.ok)
		}
	}
}

func TestGridSnapshotRoundTrip(t *testing.T) {
	s := newState()
	expTimes := []time.Time{time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)}
	s.setGrid(7640.0, strikesAround(7640, 5, 3), expTimes, "SPXW")

	anchor, strikes, expiries, calls, puts := s.gridSnapshot()
	if anchor != 7640 {
		t.Fatalf("anchor = %v, want 7640", anchor)
	}
	if !reflect.DeepEqual(strikes, []float64{7625, 7630, 7635, 7640, 7645, 7650, 7655}) {
		t.Fatalf("strikes = %v", strikes)
	}
	if len(expiries) != 2 || !expiries[0].Equal(expTimes[0]) || !expiries[1].Equal(expTimes[1]) {
		t.Fatalf("expiries round-trip failed: %v", expiries)
	}
	if len(calls) != 14 || len(puts) != 14 {
		t.Fatalf("calls/puts sizes = %d/%d, want 14/14", len(calls), len(puts))
	}
	// optionSymbols iterates expiries (outer) then strikes; 7 strikes each.
	if calls[0] != ".SPXW260914C7625" || calls[6] != ".SPXW260914C7655" ||
		calls[7] != ".SPXW260915C7625" || calls[13] != ".SPXW260915C7655" {
		t.Fatalf("calls order wrong: [0]=%v [6]=%v [7]=%v [13]=%v", calls[0], calls[6], calls[7], calls[13])
	}
	if puts[0] != ".SPXW260914P7625" || puts[13] != ".SPXW260915P7655" {
		t.Fatalf("puts[0]/[13] = %v/%v", puts[0], puts[13])
	}
}

func candle(sym string, tMs int64, o, h, l, c float64) config.Candle {
	return config.Candle{
		EventSymbol: sym,
		Time:        tMs,
		Open:        config.DXFloat(o),
		High:        config.DXFloat(h),
		Low:         config.DXFloat(l),
		Close:       config.DXFloat(c),
	}
}

func TestMergeBarCandle(t *testing.T) {
	s := newState()

	// A live-style bar exists for SPX at bucket 610000 (10m10s); the backfill
	// must insert earlier buckets around it chronologically and never overwrite.
	s.mu.Lock()
	s.bars["SPX"] = []minuteBar{{T: 610000, Open: 100, High: 100, Low: 100, Close: 100}}
	s.mu.Unlock()

	// 607500000 ms -> 607500 s -> bucket 607500 (before the live bar).
	s.mergeBarCandle(candle("SPX{=m}", 607500000, 7, 9, 6, 8))
	// Same bucket again -> dedupe, existing bar preserved.
	s.mergeBarCandle(candle("SPX{=m}", 607500000, 1, 2, 1, 1))
	// 605000000 ms -> 605000 s -> 10083.33 min -> bucket 604980 (earliest).
	s.mergeBarCandle(candle("SPX{=m}", 605000000, 10, 12, 9, 11))
	// A separate option symbol with the {=m} aggregation suffix.
	s.mergeBarCandle(candle(".SPXW260915C7645{=m}", 603000000, 1.5, 2, 1, 1.8))

	got := s.barsFor([]string{"SPX", ".SPXW260915C7645"})
	spx := got["SPX"]
	if len(spx) != 3 {
		t.Fatalf("SPX bars = %v, want 3", spx)
	}
	if spx[0].T != 604980 || spx[1].T != 607500 || spx[2].T != 610000 {
		t.Fatalf("SPX bar order = %v, want chronological 604980/607500/610000", spx)
	}
	if spx[0].Close != 11 || spx[1].Open != 7 || spx[1].High != 9 {
		t.Fatalf("SPX[0]/[1] values = %+v / %+v", spx[0], spx[1])
	}
	// The live bucket at the end is preserved (108090 live bar wins).
	if spx[2].Open != 100 || spx[2].Close != 100 {
		t.Fatalf("SPX[2] should stay the live bar, got %+v", spx[2])
	}

	opt := got[".SPXW260915C7645"]
	if len(opt) != 1 || opt[0].T != 603000 || opt[0].High != 2 {
		t.Fatalf("option bars = %+v, want single 603000 high=2", opt)
	}
}

func TestMergeBarCandleIgnoresJunk(t *testing.T) {
	s := newState()
	before := len(s.barsFor([]string{"SPX"})["SPX"])
	s.mergeBarCandle(candle("", 600000000, 1, 2, 1, 2))           // empty symbol
	s.mergeBarCandle(candle("SPX{=m}", 0, 1, 2, 1, 2))            // zero time
	s.mergeBarCandle(candle("SPX{=m}", 600000000, 0, 2, 1, 2))    // zero open
	s.mergeBarCandle(candle("SPX{=m}", 597000000, 1, 2, 1, 0))    // zero close
	after := len(s.barsFor([]string{"SPX"})["SPX"])
	if after != before {
		t.Fatalf("junk candles changed bar count %d -> %d", before, after)
	}
}