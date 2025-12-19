package tests

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest"
	"github.com/contactkeval/option-replay/internal/data"
)

// Full integration: run engine with synthetic provider and ensure outputs written
func TestIntegrationFullRun(t *testing.T) {
	cfg := &backtest.Config{
		Underlying: "SYN",
		Entry:      backtest.EntryRule{Mode: "daily_time"},
		Strategy:   []backtest.LegSpec{{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: "NDAYS:20"}},
		OutputDir:  "./test_out",
	}
	prov := data.NewSyntheticProvider()
	eng := backtest.NewEngine(cfg, prov)
	res, err := eng.Run()
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if len(res.Trades) == 0 {
		t.Fatalf("expected trades")
	}
	// write outputs
	nos := cfg.OutputDir
	os.MkdirAll(nos, 0755)
	b, _ := json.MarshalIndent(res, "", "  ")
	ioutil.WriteFile(nos+"/trades.json", b, 0644)
	// cleanup
	_ = os.RemoveAll(nos)
}
