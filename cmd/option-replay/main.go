package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/report"
)

func main() {
	configPath := flag.String("config", filepath.Join("..", "..", "strategies", "covered_call.json"), "path to JSON config")
	rest := flag.Bool("rest", false, "run as REST server (accept backtest jobs)")
	port := flag.String("port", ":8080", "REST server listen address")
	flag.Parse()

	cfgData, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("reading config: %v", err)
	}

	var cfg engine.Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	// choose provider
	var prov data.Provider
	apiKey := os.Getenv("POLYGON_API_KEY")
	if apiKey != "" {
		prov = data.NewMassiveDataProvider(apiKey)
		log.Printf("[info] polygon provider enabled")
	} else {
		prov = data.NewSyntheticProvider()
		log.Printf("[info] synthetic provider enabled")
	}

	engine := engine.NewEngine(&cfg, prov)

	if *rest {
		mux := http.NewServeMux()
		mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
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
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte("ok")) })
		log.Printf("[info] starting REST server on %s", *port)
		log.Fatal(http.ListenAndServe(*port, mux))
		return
	}

	start := time.Now()
	res, err := engine.Run()
	if err != nil {
		log.Fatalf("backtest failed: %v", err)
	}
	// write outputs to cfg.OutputDir
	if err := os.MkdirAll(cfg.ReportDir, 0755); err != nil {
		log.Printf("[warn] could not create output dir %s: %v", cfg.ReportDir, err)
	}
	_ = report.WriteJSON(res, cfg.ReportDir)
	_ = report.WriteCSV(res.Trades, cfg.ReportDir)
	log.Printf("[done] finished in %v, wrote %d trades to %s", time.Since(start), len(res.Trades), cfg.ReportDir)
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
