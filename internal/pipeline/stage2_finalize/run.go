package stage2_finalize

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	stage2Root := cfg.Stage2Root
	stage3Root := cfg.Stage3Root

	// today := time.Now()
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	logger.Infof("Stage 2 processing started")

	return filepath.Walk(
		stage2Root,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".csv") {
				return nil
			}

			base := filepath.Base(path)
			parts := strings.Split(base, "_")

			if len(parts) < 2 {
				return nil
			}

			ticker := parts[0]
			expiry := parts[1]
			expiry = strings.TrimSuffix(expiry, ".csv")
			expiryTime, err := time.Parse("060102", expiry)
			if err != nil {
				return nil
			}

			if !expiryTime.Before(today) {
				return nil
			}

			// logger.Infof(
			// 	"stage2 finalize start: %s",
			// 	path,
			// )

			rows, err := LoadRows(path)
			if err != nil {
				return err
			}
			optimizedRows := ConvertRows(rows)
			SortRows(optimizedRows)

			dedupedRows, duplicates := DedupeRows(optimizedRows)
			outputDir := filepath.Join(stage3Root, ticker)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return err
			}

			outputPath := filepath.Join(
				outputDir,
				base,
			)

			if err := WriteRows(outputPath, dedupedRows); err != nil {
				return err
			}

			logger.Infof(
				"duplicates=%d rows=%d, %s processed",
				duplicates,
				len(dedupedRows),
				path,
			)

			return nil
		},
	)
}
