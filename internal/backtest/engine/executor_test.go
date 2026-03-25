package engine

import (
	"encoding/json"
	"flag"
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
	dataProv data.Provider
	engine   *Engine
	cfg      Config
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

	configFlag := flag.String("config", defaultConfig,
		"strategy config file name (input/strategies/) or full path")

	flag.Parse()

	if *configFlag == "" {
		logger.Fatalf("config required via -config or STRATEGY_CONFIG")
	}

	cfg, err := loadConfig(*configFlag)
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	engine = NewEngine(cfg, dataProv)

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

func TestsimulatedCloseTrade(t *testing.T) {

	// 1. Read file
	dataBytes, err := os.ReadFile("./out/trades.json")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// 2. Unmarshal
	var tf struct {
		Trades []Trade `json:"trades"`
	}

	if err := json.Unmarshal(dataBytes, &tf); err != nil {
		t.Fatalf("unmarshal trades: %v", err)
	}

	dailyBars, err := engine.fetchDailyData()
	if err != nil {
		t.Fatalf("Failed to fetch underlying data: %v", err)
	}

	// 3. Process each trade
	for i := range tf.Trades {
		trade := &tf.Trades[i] // IMPORTANT: pointer

		simulatedCloseTrade(trade, dailyBars, cfg, dataProv)
	}
}

func TestGetRelevantExpiries(t *testing.T) {
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
