package stage2_dxfeeddatadownloader

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestDecodeQuoteObject(t *testing.T) {
	raw := json.RawMessage(`{
		"eventType":"Quote",
		"eventSymbol":"SPX",
		"sequence":12345,
		"time":1781788800000,
		"bidPrice":7648.2,
		"bidSize":2,
		"askPrice":7648.9,
		"askSize":7
	}`)

	ev := decodeLiveEvent(raw, &streamFields{})
	if ev == nil {
		t.Fatal("expected a decoded event")
	}
	if ev.Kind != "Quote" || ev.Symbol != "SPX" {
		t.Fatalf("unexpected kind/symbol: %s/%s", ev.Kind, ev.Symbol)
	}
	if ev.Bid != 7648.2 || ev.Ask != 7648.9 {
		t.Fatalf("unexpected bid/ask: %v/%v", ev.Bid, ev.Ask)
	}
	if ev.BidSize != 2 || ev.AskSize != 7 {
		t.Fatalf("unexpected sizes: %v/%v", ev.BidSize, ev.AskSize)
	}
	if want := time.UnixMilli(1781788800000).UTC(); !ev.Time.Equal(want) {
		t.Fatalf("unexpected time: %v, want %v", ev.Time, want)
	}
}

func TestDecodeTradeObject(t *testing.T) {
	raw := json.RawMessage(`{
		"eventType":"Trade",
		"eventSymbol":".SPXW260911C7640",
		"sequence":42,
		"time":1781788801000,
		"price":764.0,
		"size":1
	}`)

	ev := decodeLiveEvent(raw, &streamFields{})
	if ev == nil {
		t.Fatal("expected a decoded event")
	}
	if ev.Price != 764.0 || ev.Size != 1 || ev.Seq != 42 {
		t.Fatalf("unexpected trade fields: %+v", ev)
	}
}

func TestDecodeQuoteListFallback(t *testing.T) {
	fields := &streamFields{}
	fields.set(map[string][]string{
		"Quote": {"eventType", "eventSymbol", "sequence", "price", "size", "bid", "ask"},
	})

	// LIST form: [symbol, [values in eventFields order]].
	item := json.RawMessage(`["SPX",["Quote","SPX",7,"0","1","7650.5","7651.0"]]`)
	ev := decodeLiveEvent(item, fields)
	if ev == nil {
		t.Fatal("expected a decoded event")
	}
	if ev.Kind != "Quote" || ev.Symbol != "SPX" {
		t.Fatalf("unexpected kind/symbol: %s/%s", ev.Kind, ev.Symbol)
	}
	if ev.Seq != 7 || ev.Price != 0 {
		t.Fatalf("unexpected list fields: %+v", ev)
	}
}

func TestDecodeUnrelatedObjectIgnored(t *testing.T) {
	raw := json.RawMessage(`{"eventType":"Candle","eventSymbol":"SPX{=1m}"}`)
	if ev := decodeLiveEvent(raw, &streamFields{}); ev != nil {
		t.Fatalf("candle should not decode as live event: %+v", ev)
	}
}

func TestNumValRejectsNaNInf(t *testing.T) {
	// dxLink encodes "no value" as string NaN / Infinity.
	cases := map[string]float64{
		`"NaN"`:       0,
		`"Infinity"`:  0,
		`"-Infinity"`: 0,
		`"7648.2"`:    7648.2,
		`7648.2`:      7648.2,
		`""`:          0,
	}
	for in, want := range cases {
		if got := numVal(json.RawMessage(in)); got != want {
			t.Fatalf("numVal(%s) = %v, want %v", in, got, want)
		}
	}
	if math.IsNaN(numVal(json.RawMessage(`"NaN"`))) {
		t.Fatal("numVal still leaking NaN")
	}
}
