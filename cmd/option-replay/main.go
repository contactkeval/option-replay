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

// main initializes and runs the option replay application.
// It supports two modes of operation: backtesting and REST server.
//
// Configuration is loaded from the file path specified via the -config flag
// or the STRATEGY_CONFIG environment variable. The -config flag takes precedence
// if both are provided.
//
// Flags:
//   - config: strategy config file name (input/strategies/) or full path
//   - rest: run as REST server (default: false)
//   - port: REST server listen address (default: ":8080")
//
// If the -rest flag is set, the application starts a REST server on the specified port.
// Otherwise, it runs a backtest using the loaded configuration and data provider.
//
// The function will fatally exit if the config file is not provided or cannot be loaded.
func main() {
	defaultConfig := os.Getenv("STRATEGY_CONFIG")

	configFlag := flag.String("config", defaultConfig,
		"strategy config file name (input/strategies/) or full path")

	rest := flag.Bool("rest", false, "run as REST server")
	report := flag.Bool("report", false, "serve reusable report HTML from existing out/ data")
	port := flag.String("port", ":8080", "REST server listen address")
	reportPort := flag.String("report-port", ":8090", "report server listen address")
	flag.Parse()

	if *configFlag == "" {
		logger.Fatalf("config required via -config or STRATEGY_CONFIG")
	}

	cfg, err := loadConfig(*configFlag)
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	dataProv := buildProvider()
	engine := engine.NewEngine(cfg, dataProv)

	if *rest {
		startServer(*port, engine)
		return
	}

	if *report {
		startReportServer(*reportPort, cfg)
		return
	}

	runBacktest(engine, cfg)
}

// runBacktest executes a backtest using the provided engine and configuration.
// It runs the backtest, creates the output directory if needed, and writes
// the results to JSON and CSV files in the configured report directory.
// Errors during directory creation or file writing are logged as warnings
// but do not stop execution. The total execution time is logged upon completion.
func runBacktest(eng *engine.Engine, cfg *engine.Config) {
	start := time.Now()

	if cfg.ReportDir == "" {
		cfg.ReportDir = "./out"
	}
	if err := os.MkdirAll(cfg.ReportDir, 0750); err != nil {
		logger.Errorf("could not create output dir %s: %v", cfg.ReportDir, err)
	}

	runID, err := report.NextRunID(cfg.ReportDir)
	if err != nil {
		logger.Warnf("could not allocate report run id: %v", err)
		runID = time.Now().Format("20060102") + "00001"
	}
	cfg.ReportRunID = runID

	res, err := eng.Run()
	if err != nil {
		logger.Errorf("backtest failed: %v", err)
		return
	}

	dataDir := filepath.Join(cfg.ReportDir, "data_"+runID)
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		logger.Errorf("could not create data dir %s: %v", dataDir, err)
	}

	_ = report.WriteJSON(res, dataDir, runID)
	_ = report.WriteCSV(res.Trades, dataDir, runID, cfg.Entry.Timezone)

	_ = report.WriteHTMLReport(res.Trades, dataDir, cfg.ReportDir, runID, cfg.Report)

	logger.Infof("backtest completed in %v, results written to %s (run %s)",
		time.Since(start), cfg.ReportDir, runID)
}

// buildProvider creates and returns a data provider based on environment configuration.
// It checks for the MASSIVE_API_KEY environment variable and returns a MassiveDataProvider
// if the key is available, otherwise it returns a SyntheticProvider as a fallback.
func buildProvider() data.Provider {
	var secondary data.Provider
	if apiKey := os.Getenv("MASSIVE_API_KEY"); apiKey != "" {
		logger.Infof("massive provider enabled")
		secondary = data.NewLocalFileDataProvider(
			"input/data",
			data.NewMassiveDataProvider(apiKey),
		)
	} else {
		logger.Infof("synthetic provider enabled")
		secondary = data.NewSyntheticProvider()
	}

	parquetProv, err := data.NewParquetDataProviderFromConfig(secondary)
	if err != nil {
		logger.Infof("parquet provider unavailable: %v", err)
		return secondary
	}
	logger.Infof("parquet provider enabled")
	return parquetProv
}

// loadConfig reads and parses a configuration file from the specified path.
// It resolves the config path, reads the file, and unmarshals the JSON data
// into an engine.Config struct. Returns a pointer to the Config or an error
// if the file cannot be read or parsed.
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

// startServer starts an HTTP server listening on the given port with the specified engine.
// It registers two endpoints:
//   - POST /run: executes engine.Run() and returns the result as JSON
//   - GET /health: returns a simple "ok" status check
//
// The function blocks indefinitely while the server is running.
// If the server fails to start or encounters a fatal error, it logs and exits.
//
// Parameters:
//   - port: the network address to listen on (e.g., ":8080")
//   - engine: the Engine instance used to process run requests
func startServer(port string, engine *engine.Engine) {
	mux := http.NewServeMux()

	mux.HandleFunc("/run", func(w http.ResponseWriter, _ *http.Request) {
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
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	logger.Infof("starting REST server on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		logger.Fatalf("server failed: %v", err)
	}
}

// startReportServer serves a reusable report HTML page that reads the latest
// out/data_* CSVs at view time. The page is served at "/" and loads /api/data
// which parses the CSVs into the chart payload. Use it after a backtest has
// produced CSVs, e.g.: option-replay -config custom -report
func startReportServer(addr string, cfg *engine.Config) {
	outdir := cfg.ReportDir
	if outdir == "" {
		outdir = "./out"
	}

	handler, err := report.ReportHandler(outdir, nil, cfg.Report)
	if err != nil {
		logger.Fatalf("report handler error: %v", err)
	}

	logger.Infof("starting report server on %s (serving %s)", addr, outdir)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatalf("report server failed: %v", err)
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
//  - No error should be silently ignored.  If an error is returned then it must be wrapped.

// ---------------------------------------------------------------------------------------
// Looking Ahead

// After v1.0, I'd enjoy helping with:

// Comprehensive unit and integration tests
// Benchmark suite (go test -bench)
// Performance profiling
// Documentation (code documentation as well as flow chart, wiki and maintaining open API docs)
// Transaction fencing and processing (session management, idempotency, retries, etc.)
// Recoverability (fail safe)
// GitHub Actions (lint, test, build)
// Logging/tracing information
// Release process
// A small terminal dashboard for monitoring runs

// Consistent coding style:
// 	naming
// 	formatting
// 	logging
// 	SQL style
// 	error handling
// 	transaction handling
// ---------------------------------------------------------------------------------------

//https://api.massive.com/v2/aggs/ticker/O:SPXW260807C07750000/range/1/minute/2026-08-07/2026-08-10?adjusted=true&sort=asc&limit=5000&apiKey=bHkHZvzoQauBG0B1fAgiR4gOuuxXz2Md
