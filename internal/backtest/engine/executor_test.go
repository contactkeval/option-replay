package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	tests "github.com/contactkeval/option-replay/internal/testutil"
)

var (
	dataProv *data.Provider
	engine   *Engine
	cfg      *Config
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

func init() {
	dataProv := data.NewLocalFileDataProvider(
		"../../../input/data",
		data.NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")))

	defaultConfig := os.Getenv("STRATEGY_CONFIG")

	if defaultConfig == "" {
		logger.Fatalf("config required via -config or STRATEGY_CONFIG")
	}

	cfg, err := loadConfig(defaultConfig)
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	engine = NewEngine(cfg, dataProv)
	engine.initConfiguration()
}

func loadConfig(path string) (*Config, error) {
	configPath := resolveConfigPath(path)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config (%s): %w", configPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

func resolveConfigPath(input string) string {
	// Auto-append .json if missing from input filename
	if !strings.HasSuffix(input, ".json") {
		input += ".json"
	}

	// If absolute or contains directory, use as-is
	if filepath.IsAbs(input) || strings.Contains(input, string(os.PathSeparator)) {
		return input
	}

	// Otherwise assume it's just a filename
	return filepath.Join("input", "strategies", input)
}

func TestSimulatedCloseTrade(t *testing.T) {

	// 1. Read file
	dataBytes, err := os.ReadFile("../../.././out/trades.json")
	if err != nil {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		logger.Infof("Current directory: %s", dir)
		t.Fatalf("read file: %v", err)
	}

	// 2. Unmarshal
	var trades struct {
		Trades []Trade `json:"trades"`
	}

	if err := json.Unmarshal(dataBytes, &trades); err != nil {
		t.Fatalf("unmarshal trades: %v", err)
	}

	dailyBars, err := engine.fetchDailyData()
	if err != nil {
		t.Fatalf("Failed to fetch underlying data: %v", err)
	}

	// 3. Process each trade
	for i := range trades.Trades {
		trade := &trades.Trades[i] // IMPORTANT: pointer

		simulatedCloseTrade(trade, dailyBars, *engine.cfg, engine.dataProv)
	}
}

func TestGetRelevantExpiries(t *testing.T) {
	var prov data.Provider
	prov = *dataProv
	startDate := time.Date(2026, 3, 1, 9, 45, 0, 0, time.UTC)
	endDate := time.Date(2026, 3, 15, 15, 40, 0, 0, time.UTC)
	expiries, err := prov.GetRelevantExpiries("I:NDX", startDate, endDate)
	if err != nil {
		t.Fatalf("getRelevantExpiries failed: %v", err)
	}
	if len(expiries) == 0 {
		t.Fatal("expected at least one expiry but got none")
	}

	tests.CompareWithGolden(t, "RelevantExpiries", expiries)
}
