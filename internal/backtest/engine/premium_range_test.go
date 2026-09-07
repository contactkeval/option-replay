package engine

import (
	"testing"
	"time"

	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
)

func TestSpotPriceAtUsesMinuteBarNotDailyClose(t *testing.T) {
	entry := time.Date(2025, 12, 26, 15, 40, 0, 0, time.UTC)
	fallback := 7000.0
	prov := &spotStub{bars: []data.Bar{
		{Date: entry.Add(-2 * time.Minute), Close: 6910},
		{Date: entry.Add(-1 * time.Minute), Close: 6920},
		{Date: entry, Close: 6925},
	}}

	got := spotPriceAt(prov, "I:SPX", entry, fallback)
	if got != 6925 {
		t.Fatalf("spotPriceAt=%v want 6925 (not daily close %v)", got, fallback)
	}
}

type spotStub struct {
	bars []data.Bar
}

func (*spotStub) GetName() string                          { return "spot-stub" }
func (*spotStub) GetSecondary() data.Provider              { return nil }
func (*spotStub) SetSecondary(data.Provider)               {}
func (*spotStub) parseExpiryFromSymbol(string) time.Time   { return time.Time{} }
func (*spotStub) OptionSymbolFromParts(string, time.Time, string, float64) string {
	return ""
}
func (*spotStub) GetStrikeIntervals(string, time.Time) []float64 { return nil }
func (*spotStub) RoundToNearestStrike(string, time.Time, time.Time, float64) float64 {
	return 0
}
func (*spotStub) GetATMOptionPrices(string, time.Time, time.Time, float64) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (*spotStub) GetContracts(string, float64, time.Time, time.Time, bool) ([]data.OptionContract, error) {
	return nil, nil
}
func (s *spotStub) GetBars(_ string, from, to time.Time, _ int, _ string) ([]data.Bar, error) {
	var out []data.Bar
	for _, b := range s.bars {
		if (from.IsZero() || !b.Date.Before(from)) && (to.IsZero() || !b.Date.After(to)) {
			out = append(out, b)
		}
	}
	return out, nil
}
func (*spotStub) GetOptionPrice(string, float64, time.Time, string, time.Time) (float64, error) {
	return 0, nil
}
func (*spotStub) GetRelevantExpiries(string, time.Time, time.Time) ([]time.Time, error) {
	return nil, nil
}

func TestRecordStrategyPremiumRange(t *testing.T) {
	open := time.Date(2025, 1, 2, 15, 40, 0, 0, time.UTC)
	trade := Trade{
		OpenPremium:  -10,
		HighPremium:  -10,
		LowPremium:   -10,
		ClosePremium: -8,
		Legs: []st.TradeLeg{
			{OpenPremium: 10, Spec: st.LegSpec{Side: "sell", Qty: 1}},
		},
	}

	minuteData := []MinuteRow{
		{Timestamp: open, LegBars: []data.Bar{{Date: open, Close: 10}}},
		{Timestamp: open.Add(time.Minute), LegBars: []data.Bar{{Date: open.Add(time.Minute), Close: 14}}},
		{Timestamp: open.Add(2 * time.Minute), LegBars: []data.Bar{{Date: open.Add(2 * time.Minute), Close: 6}}},
		{Timestamp: open.Add(3 * time.Minute), LegBars: []data.Bar{{Date: open.Add(3 * time.Minute), Close: 20}}},
	}

	recordStrategyPremiumRange(&trade, minuteData, open, open.Add(2*time.Minute))
	includeClosePremiumInRange(&trade)

	// sell 10 -> -10; 14 -> -14; 6 -> -6; bar at +3m is after close and ignored
	if trade.HighPremium != -6 {
		t.Fatalf("HighPremium=%v want -6", trade.HighPremium)
	}
	if trade.LowPremium != -14 {
		t.Fatalf("LowPremium=%v want -14", trade.LowPremium)
	}

	trade.ClosePremium = -20
	includeClosePremiumInRange(&trade)
	if trade.LowPremium != -20 {
		t.Fatalf("LowPremium after close=%v want -20", trade.LowPremium)
	}
}
