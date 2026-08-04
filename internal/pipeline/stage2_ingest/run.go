package stage2_ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {
	logger.Infof("Stage 2 processing started")

	transientDBPath := filepath.Join(
		cfg.SQLiteRoot,
		"transient.db",
	)

	database, err := db.Open(db.Options{
		Path:    transientDBPath,
		Schemas: db.SchemaTransient,
	})
	if err != nil {
		return fmt.Errorf("open transient database: %w", err)
	}
	defer database.Close()

	return filepath.Walk(
		cfg.RawRoot,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("walk directory %s: %w", cfg.RawRoot, err)
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".csv.gz") {
				return nil
			}

			logger.Infof("processing: %s", path)

			expiryRows, err := ProcessRawFile(path)
			if err != nil {
				return fmt.Errorf("process raw file %s: %w", path, err)
			}

			tx, err := database.Begin()
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}

			for expiry, rows := range expiryRows {
				if err := database.EnsureExpiryTable(tx, expiry); err != nil {
					tx.Rollback()
					return fmt.Errorf("ensure expiry table for %s: %w", expiry, err)
				}

				if err := database.InsertBars(tx, expiry, rows); err != nil {
					tx.Rollback()
					return fmt.Errorf("insert bars for %s: %w", expiry, err)
				}
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit transaction: %w", err)
			}

			if err := ArchiveRawFile(path, cfg.RawRoot, cfg.ArchiveRawRoot); err != nil {
				return fmt.Errorf("archive raw file %s: %w", path, err)
			}

			return nil
		},
	)
}

func ArchiveRawFile(
	sourcePath string,
	rawRoot string,
	archivedRoot string,
) error {
	rel, err := filepath.Rel(rawRoot, sourcePath)
	if err != nil {
		return fmt.Errorf("calculate relative path: %w", err)
	}

	targetPath := filepath.Join(archivedRoot, rel)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	if err := os.Rename(sourcePath, targetPath); err != nil {
		return fmt.Errorf("move raw file to archive: %w", err)
	}

	return nil
}
