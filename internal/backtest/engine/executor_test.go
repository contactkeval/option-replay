package engine

import (
	"testing"

	"github.com/contactkeval/option-replay/internal/data"
)

func TestExecuteBacktest(t *testing.T) {
	cfg := &Config{
		Underlying: "SPY",
		Entry:      EntryRule{Mode: "daily_time"},
		Strategy:   StrategySpec{Legs: []LegSpec{{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 20}}, DateMatchType: "Nearest"},
		ReportDir:  "./test_out",
	}
	prov := data.NewSyntheticProvider()
	eng := NewEngine(cfg, prov)
	res, err := eng.Run()
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

}
