package engine

import (
	"os"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	tests "github.com/contactkeval/option-replay/internal/testutil"
)

func TestExecuteBacktest(_ *testing.T) {
	// cfg := &Config{
	// 	Underlying: "SPY",
	// 	Entry:      EntryRule{Mode: "daily_time"},
	// 	Strategy:   StrategySpec{Legs: []LegSpec{{Side: "buy", OptionType: "put", StrikeRule: "ATM", Qty: 1, Expiration: 20}}, DateMatchType: "Nearest"},
	// 	ReportDir:  "./test_out",
	// }
	// prov := data.NewSyntheticProvider()
	// eng := NewEngine(cfg, prov)
	// res, err := eng.Run()
	// if err != nil {
	// 	t.Fatalf("engine run failed: %v", err)
	// }
}

func TestGetRelevantExpiries(t *testing.T) {
	dataProv := data.NewLocalFileDataProvider(
		"../../../input/data",
		data.NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")))
	startDate := time.Date(2026, 3, 1, 9, 45, 0, 0, time.UTC)
	endDate := time.Date(2026, 3, 15, 15, 40, 0, 0, time.UTC)
	expiries, err := dataProv.GetRelevantExpiries("I:NDX", startDate, endDate)
	if err != nil {
		t.Fatalf("getRelevantExpiries failed: %v", err)
	}
	if len(expiries) == 0 {
		t.Fatal("expected at least one expiry but got none")
	}

	tests.CompareWithGolden(t, "RelevantExpiries", expiries)
}
