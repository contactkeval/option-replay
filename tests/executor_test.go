package tests

import (
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
	sch "github.com/contactkeval/option-replay/internal/backtest/scheduler"
	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
)

// executor tests focus on entry/exit over synthetic data
func TestProfitTargetExit(t *testing.T) {
	cfg := &engine.Config{
		Underlying: "SYN",
		Entry:      sch.EntryRule{Mode: "daily_time"},
		Strategy:   []st.LegSpec{{Side: "sell", OptionType: "call", StrikeRule: "ATM", Qty: 1, Expiration: "NDAYS:30"}},
		Exit:       engine.ExitSpec{ProfitTargetPct: func() *float64 { v := 50.0; return &v }()},
	}

	prov := data.NewSyntheticProvider()
	eng := engine.NewEngine(cfg, prov)
	res, err := eng.Run()
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if len(res.Trades) == 0 {
		t.Fatalf("expected at least one trade")
	}
	// ensure each trade closed with some ClosedBy value
	for _, tr := range res.Trades {
		if tr.ClosedBy == "" {
			t.Fatalf("trade %d missing ClosedBy", tr.ID)
		}
	}
}
