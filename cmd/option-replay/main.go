package main

import (
	"encoding/json"
	"flag"
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

	strategyConfig := flag.String(
		"config",
		defaultConfig,
		"strategy config file name (looked up in input/strategies/) or full path",
	)

	rest := flag.Bool("rest", false, "run as REST server (accept backtest jobs)")
	port := flag.String("port", ":8080", "REST server listen address")
	flag.Parse()

	if *strategyConfig == "" {
		logger.Errorf("config path required via -config flag or STRATEGY_CONFIG env")
		os.Exit(1)
	}

	// 👇 Resolve path
	configPath := resolveConfigPath(*strategyConfig)

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		logger.Errorf("reading config (%s): %v", configPath, err)
		os.Exit(1)
	}
	var cfg engine.Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		logger.Errorf("parsing config: %v", err)
	}

	// choose provider
	var dataProv data.Provider
	apiKey := os.Getenv("MASSIVE_API_KEY")
	if apiKey != "" {
		dataProv = data.NewMassiveDataProvider(apiKey)
		logger.Infof("massive provider enabled")
	} else {
		dataProv = data.NewSyntheticProvider()
		logger.Infof("synthetic provider enabled")
	}

	engine := engine.NewEngine(&cfg, dataProv)

	if *rest {
		mux := http.NewServeMux()
		mux.HandleFunc("/run", func(w http.ResponseWriter, _ *http.Request) {
			// quick endpoint to run a backtest once with the loaded config
			logger.Infof("received run request")
			res, err := engine.Run()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
		})
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("ok"))
		})
		logger.Infof("starting REST server on %s", *port)
		return
	}

	start := time.Now()
	res, err := engine.Run()
	if err != nil {
		logger.Errorf("backtest failed: %v", err)
	}
	// write outputs to cfg.OutputDir
	if err := os.MkdirAll(cfg.ReportDir, 0750); err != nil {
		logger.Warnf("could not create output dir %s: %v", cfg.ReportDir, err)
	}
	_ = report.WriteJSON(res, cfg.ReportDir)
	_ = report.WriteCSV(res.Trades, cfg.ReportDir)
	logger.Infof("backtest completed in %v, results written to %s", time.Since(start), cfg.ReportDir)
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
