package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

// Command selector builds (or previews) the contract selection / batch plan
// used by the DXFeed download stage.
//
// It opens metadata.db, runs expired + far-expiry selection for a run date,
// optionally persists the run plan, and prints a summary.
//
// go run ./cmd/selector -dry-run
// go run ./cmd/selector -date 2026-08-06
// go run ./cmd/selector -db G:\data\metadata\metadata.db
func main() {
	dateFlag := flag.String(
		"date",
		"",
		"run date as yyyy-mm-dd (default: today)",
	)
	dryRun := flag.Bool(
		"dry-run",
		false,
		"select and print groups without persisting a run",
	)
	dbPathFlag := flag.String(
		"db",
		"",
		"path to metadata.db (default: <MetadataRoot>/metadata.db)",
	)
	flag.Parse()

	cfg := config.Load()

	dbPath := *dbPathFlag
	if dbPath == "" {
		dbPath = filepath.Join(cfg.MetadataRoot, "metadata.db")
	}

	runDate, err := parseRunDate(*dateFlag)
	if err != nil {
		logger.Fatalf("invalid -date: %v", err)
	}

	database, err := db.Open(db.Options{
		Path:    dbPath,
		Schemas: db.SchemaMetadata,
	})
	if err != nil {
		logger.Fatalf("open metadata DB: %v", err)
	}
	defer database.Close()

	fmt.Printf("metadata DB: %s\n", dbPath)
	fmt.Printf("run date:    %s\n", runDate.Format("2006-01-02"))

	if *dryRun {
		if err := printSelection(database, runDate); err != nil {
			logger.Fatalf("selection failed: %v", err)
		}
		return
	}

	runNo, err := stage2.BuildRunPlan(database, runDate)
	if err != nil {
		logger.Fatalf("build run plan: %v", err)
	}

	fmt.Printf("run plan ready: runNo=%d\n", runNo)
}

// printSelection runs selection without persisting and prints group sizes plus
// a short sample of the sorted contract list.
func printSelection(database *db.DB, runDate time.Time) error {
	contracts, err := stage2.GetContractsForRun(database, runDate)
	if err != nil {
		return err
	}

	stage2.SortContractsForGrouping(contracts)
	batches := stage2.CreateBatches(contracts)

	fmt.Printf("Selected contracts: %d\n", len(contracts))
	fmt.Printf("Groups: %d\n", len(batches))

	for _, batch := range batches {
		fmt.Printf(
			"  group %d: %d contracts\n",
			batch.BatchNo,
			len(batch.Contracts),
		)
	}

	limit := 20
	if len(contracts) < limit {
		limit = len(contracts)
	}
	if limit == 0 {
		return nil
	}

	fmt.Println("Sample (sorted for grouping):")
	for i := 0; i < limit; i++ {
		c := contracts[i]
		lastFetch := "-"
		if !c.LastDownloadedDate.IsZero() {
			lastFetch = c.LastDownloadedDate.Format("2006-01-02")
		}
		fmt.Printf(
			"  %d. %s %s %s %.2f bars=%d lastFetch=%s\n",
			i+1,
			c.Underlying,
			c.Expiry.Format("2006-01-02"),
			c.Type,
			c.Strike,
			c.BarCount,
			lastFetch,
		)
	}

	return nil
}

// parseRunDate parses yyyy-mm-dd or mm/dd/yyyy; empty means today (UTC date).
func parseRunDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		now := time.Now()
		return time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			0, 0, 0, 0,
			time.UTC,
		), nil
	}

	layouts := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf(
		"want yyyy-mm-dd or mm/dd/yyyy, got %q",
		raw,
	)
}

func init() {
	// Keep usage discoverable when run with -h.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: selector [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Build (or preview) the contract selection / batch plan.\n\n")
		flag.PrintDefaults()
	}
}
