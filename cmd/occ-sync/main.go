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

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage0_occ "github.com/contactkeval/option-replay/internal/pipeline/stage0_OCCSync"
)

func main() {
	dateFlag := flag.String(
		"date",
		"",
		"OCC file date as mm/dd/yyyy or yyyy-mm-dd (default: two calendar days ago, T-2)",
	)
	typesFlag := flag.String(
		"types",
		"A,B,D,M",
		"comma-separated OCC download types: A (add), D (delete), M (modify), B (both --add and delete)",
	)
	underlyingsFlag := flag.String(
		"underlyings",
		"",
		"path to allowed underlyings file (default: internal/pipeline/config/allowed_underlyings.txt)",
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg := config.Load()

	underlyingsPath := *underlyingsFlag
	if underlyingsPath == "" {
		underlyingsPath = defaultUnderlyingsPath()
	}

	if err := config.LoadAllowedUnderlyings(underlyingsPath); err != nil {
		logger.Fatalf("load allowed underlyings: %v", err)
	}

	fileDate, err := parseFileDate(*dateFlag)
	if err != nil {
		logger.Fatalf("invalid -date: %v", err)
	}

	downloadTypes, err := parseDownloadTypes(*typesFlag)
	if err != nil {
		logger.Fatalf("invalid -types: %v", err)
	}

	if err := stage0_occ.Run(ctx, cfg, fileDate, downloadTypes); err != nil {
		logger.Fatalf("OCC sync failed: %v", err)
	}
}

func parseFileDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		now := time.Now()
		t2 := time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			0, 0, 0, 0,
			now.Location(),
		).AddDate(0, 0, -2)
		return t2, nil
	}

	layouts := []string{
		"01/02/2006",
		"2006-01-02",
		"1/2/2006",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf(
		"want mm/dd/yyyy or yyyy-mm-dd, got %q",
		raw,
	)
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
		case stage0_occ.ActionAdd, stage0_occ.ActionDelete, stage0_occ.ActionModify, stage0_occ.ActionBoth:
			out = append(out, t)
		default:
			return nil, fmt.Errorf("unknown type %q (want A, B, D, or M)", part)
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
