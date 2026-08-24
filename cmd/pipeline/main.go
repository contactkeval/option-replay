package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage0_occ "github.com/contactkeval/option-replay/internal/pipeline/stage0_OCCSync"
	stage2b "github.com/contactkeval/option-replay/internal/pipeline/stage2_dxfeeddatadownloader"
)

// Command pipeline syncs OCC contracts, builds a run/batch plan (including
// all allowed underlyings as spot symbols when SPY spot bars are stale), then
// downloads DXFeed candles for that plan.
//
//	go run ./cmd/pipeline
//	go run ./cmd/pipeline -batch 3
//	go run ./cmd/pipeline -run 21 -batch 3
func main() {
	occDateFlag := flag.String(
		"occ-date",
		"",
		"OCC file date as yyyy-mm-dd or mm/dd/yyyy (default: previous calendar day)",
	)
	runDateFlag := flag.String(
		"date",
		"",
		"run/selection date as yyyy-mm-dd (default: today)",
	)
	typesFlag := flag.String(
		"types",
		"A,D,M",
		"comma-separated OCC download types: A (add), D (delete), M (modify)",
	)
	underlyingsFlag := flag.String(
		"underlyings",
		"",
		"path to allowed underlyings file (default: internal/pipeline/config/allowed_underlyings.txt)",
	)
	runFlag := flag.Int64(
		"run",
		0,
		"download this run number and skip creating a new plan (default: create a plan after OCC sync)",
	)
	batchFlag := flag.Int(
		"batch",
		0,
		"batch number to download (default: all batches of the run)",
	)
	dbPathFlag := flag.String(
		"db",
		"",
		"path to metadata.db (default: <MetadataRoot>/metadata.db)",
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg := config.Load()

	dbPath := *dbPathFlag
	if dbPath == "" {
		dbPath = filepath.Join(cfg.MetadataRoot, "metadata.db")
	} else {
		cfg.MetadataRoot = filepath.Dir(dbPath)
	}

	underlyingsPath := *underlyingsFlag
	if underlyingsPath == "" {
		underlyingsPath = defaultUnderlyingsPath()
	}
	if err := config.LoadAllowedUnderlyings(underlyingsPath); err != nil {
		logger.Fatalf("load allowed underlyings: %v", err)
	}

	occDate, err := parseOCCDate(*occDateFlag)
	if err != nil {
		logger.Fatalf("invalid -occ-date: %v", err)
	}
	downloadTypes, err := parseDownloadTypes(*typesFlag)
	if err != nil {
		logger.Fatalf("invalid -types: %v", err)
	}

	runDate, err := parseRunDate(*runDateFlag)
	if err != nil {
		logger.Fatalf("invalid -date: %v", err)
	}

	logger.Infof("pipeline metadata DB: %s", dbPath)

	if err := stage0_occ.Run(ctx, cfg, occDate, downloadTypes); err != nil {
		logger.Fatalf("OCC sync failed: %v", err)
	}

	runNo := *runFlag
	if runNo == 0 {
		database, err := db.Open(db.Options{
			Path:    dbPath,
			Schemas: db.SchemaMetadata,
		})
		if err != nil {
			logger.Fatalf("open metadata DB: %v", err)
		}

		logger.Infof("building run plan for %s", runDate.Format("2006-01-02"))
		runNo, err = stage2b.BuildRunPlan(database, runDate)
		database.Close()
		if err != nil {
			logger.Fatalf("build run plan: %v", err)
		}
		logger.Infof("run plan ready: runNo=%d", runNo)
	}

	if err := ctx.Err(); err != nil {
		logger.Fatalf("pipeline cancelled: %v", err)
	}

	if err := stage2b.Run(cfg, dbPath, runNo, *batchFlag); err != nil {
		logger.Fatalf("dxfeed download failed: %v", err)
	}
}

func parseOCCDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		now := time.Now()
		return time.Date(
			now.Year(),
			now.Month(),
			now.Day()-1,
			0, 0, 0, 0,
			now.Location(),
		), nil
	}

	return parseDate(raw, time.Local)
}

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

	return parseDate(raw, time.UTC)
}

func parseDate(raw string, loc *time.Location) (time.Time, error) {
	layouts := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("want yyyy-mm-dd or mm/dd/yyyy, got %q", raw)
}

func parseDownloadTypes(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		t := strings.ToUpper(strings.TrimSpace(part))
		if t == "" {
			continue
		}
		switch t {
		case stage0_occ.ActionAdd, stage0_occ.ActionDelete, stage0_occ.ActionModify:
			out = append(out, t)
		default:
			return nil, fmt.Errorf("unknown type %q (want A, D, or M)", part)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("at least one download type is required")
	}
	return out, nil
}

func defaultUnderlyingsPath() string {
	candidates := []string{
		filepath.Join("internal", "pipeline", "config", "allowed_underlyings.txt"),
		filepath.Join("..", "..", "internal", "pipeline", "config", "allowed_underlyings.txt"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pipeline [flags]\n\n")
		fmt.Fprintf(os.Stderr, "1) Sync OCC contracts\n")
		fmt.Fprintf(os.Stderr, "2) Create a run/batch plan\n")
		fmt.Fprintf(os.Stderr, "   (adds all allowed underlyings as spot if SPY bars are stale)\n")
		fmt.Fprintf(os.Stderr, "3) Download DXFeed candles for that run\n\n")
		flag.PrintDefaults()
	}
}
