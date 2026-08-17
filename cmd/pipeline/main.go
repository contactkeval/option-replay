package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage2b "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

// Command pipeline downloads DXFeed candles for a persisted run plan.
//
// Use selector first to create the run/batch plan, then:
//
//	go run ./cmd/pipeline
//	go run ./cmd/pipeline -run 12
//	go run ./cmd/pipeline -run 12 -batch 3
func main() {
	runFlag := flag.Int64(
		"run",
		0,
		"run number (default: latest run)",
	)
	batchFlag := flag.Int(
		"batch",
		0,
		"batch number (default: all batches of the run)",
	)
	dbPathFlag := flag.String(
		"db",
		"",
		"path to metadata.db (default: <MetadataRoot>/metadata.db)",
	)
	flag.Parse()

	cfg := config.Load()

	if err := stage2b.Run(cfg, *dbPathFlag, *runFlag, *batchFlag); err != nil {
		logger.Fatalf("dxfeed download failed: %v", err)
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pipeline [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Download DXFeed candles for a run plan.\n")
		fmt.Fprintf(os.Stderr, "Start date is 2 years before today. New inserts are added to contracts.barCount.\n\n")
		flag.PrintDefaults()
	}
}
