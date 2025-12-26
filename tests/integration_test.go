package tests

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
	sch "github.com/contactkeval/option-replay/internal/backtest/scheduler"
	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
)

var (
	locNY  *time.Location
	start  time.Time
	end    time.Time
	update *bool

	dataProv data.Provider
)

func init() {
	var err error
	locNY, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}

	start = time.Date(2025, 1, 1, 0, 0, 0, 0, locNY)
	end = time.Date(2026, 1, 1, 0, 0, 0, 0, locNY)
}

// Full integration: run engine with synthetic provider and ensure outputs written
func TestIntegrationFullRun(t *testing.T) {
	cfg := &engine.Config{
		Underlying: "SYN",
		Entry:      sch.EntryRule{Mode: "daily_time"},
		Strategy:   []st.LegSpec{{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: "NDAYS:20"}},
		OutputDir:  "./test_out",
	}
	prov := data.NewSyntheticProvider()
	eng := engine.NewEngine(cfg, prov)
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
