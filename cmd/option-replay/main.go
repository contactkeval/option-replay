package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/report"
)

func main() {
	defaultConfig := os.Getenv("STRATEGY_CONFIG")

	configFlag := flag.String("config", defaultConfig,
		"strategy config file name (input/strategies/) or full path")

	rest := flag.Bool("rest", false, "run as REST server")
	port := flag.String("port", ":8080", "REST server listen address")
	flag.Parse()

	if *configFlag == "" {
		logger.Fatalf("config required via -config or STRATEGY_CONFIG")
	}

	cfg, err := loadConfig(*configFlag)
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	prov := buildProvider()
	eng := engine.NewEngine(cfg, prov)

	if *rest {
		startServer(*port, eng)
		return
	}

	runBacktest(eng, cfg)
}

func runBacktest(eng *engine.Engine, cfg *engine.Config) {
	start := time.Now()

	res, err := eng.Run()
	if err != nil {
		logger.Errorf("backtest failed: %v", err)
		return
	}

	if err := os.MkdirAll(cfg.ReportDir, 0750); err != nil {
		logger.Warnf("could not create output dir %s: %v", cfg.ReportDir, err)
	}

	_ = report.WriteJSON(res, cfg.ReportDir)
	_ = report.WriteCSV(res.Trades, cfg.ReportDir)

	logger.Infof("backtest completed in %v, results written to %s",
		time.Since(start), cfg.ReportDir)
}

func buildProvider() data.Provider {
	if apiKey := os.Getenv("MASSIVE_API_KEY"); apiKey != "" {
		logger.Infof("massive provider enabled")
		return data.NewMassiveDataProvider(apiKey)
	}
	logger.Infof("synthetic provider enabled")
	return data.NewSyntheticProvider()
}

func loadConfig(path string) (*engine.Config, error) {
	configPath := resolveConfigPath(path)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config (%s): %w", configPath, err)
	}

	var cfg engine.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

func startServer(port string, eng *engine.Engine) {
	mux := http.NewServeMux()

	mux.HandleFunc("/run", func(w http.ResponseWriter, _ *http.Request) {
		logger.Infof("received run request")

		res, err := eng.Run()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	logger.Infof("starting REST server on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		logger.Fatalf("server failed: %v", err)
	}
}

// resolveConfigPath resolves a config file path by handling both absolute paths
// and filenames. If the input is an absolute path or contains directory separators,
// it is returned as-is. Otherwise, the input is treated as a filename and is
// joined with the default directory path "input/strategies/".
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

// TODO: add REST endpoint to accept dynamic configs
//  - accept JSON config in POST body
//  - validate config
//  - run backtest
//  - return results as JSON response
//  - consider adding job queue for longer backtests
//  - add logging and error handling
//  - secure endpoint with basic auth or API key
//  - add example curl command in README
//  - consider adding WebSocket support for real-time updates
//  - add metrics endpoint for monitoring
//  - add graceful shutdown handling
//  - add rate limiting to prevent abuse
//  - add CORS headers if needed for browser clients
//  - add unit tests for REST handlers
//  - document REST API endpoints and usage
//  - consider adding Swagger/OpenAPI spec for the API
//  - add Dockerfile for easy deployment
//  - consider adding Kubernetes deployment manifests
//  - add logging middleware for request tracing
//  - consider adding authentication/authorization for secure access
//  - add configuration options for REST server (port, timeouts, etc.)
