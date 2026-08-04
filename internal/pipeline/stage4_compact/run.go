package stage4_compact

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {
	logger.Infof("Stage 4 compact started")

	if err := RecoverCompactions(cfg.ParquetRoot); err != nil {
		return fmt.Errorf("recover compactions: %w", err)
	}

	metadataPath := filepath.Join(cfg.MetadataRoot, "metadata.db")

	database, err := db.Open(db.Options{
		Path:    metadataPath,
		Schemas: db.SchemaParquet,
	})
	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}
	defer database.Close()

	for {
		rows, err := database.LoadCompactCandidates()
		if err != nil {
			return fmt.Errorf("load compact candidates: %w", err)
		}

		logger.Infof("loaded candidates=%d", len(rows))

		candidateGroups := SelectCompactCandidates(
			rows,
			config.TargetRowGroupsPerFile,
		)

		logger.Infof("candidate groups=%d", len(candidateGroups))

		if len(candidateGroups) == 0 {
			break
		}

		for _, group := range candidateGroups {
			if len(group) == 0 {
				continue
			}

			ticker := group[0].Ticker
			firstExpiry := group[0].FirstExpiry.Format("20060102")

			outputDir := filepath.Join(cfg.ParquetRoot, ticker)

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}

			finalPath := filepath.Join(
				outputDir,
				fmt.Sprintf("%s_%s.parquet", ticker, firstExpiry),
			)

			compactingPath := finalPath + ".compacting"

			inputs := make([]string, 0)
			pendingFiles := make([]string, 0)

			for _, file := range group {
				if _, err := os.Stat(file.ParquetPath); err != nil {
					logger.Infof("candidate file missing=%s", file.ParquetPath)
				}

				pending, err := RenamePending(file.ParquetPath)
				if err != nil {
					return fmt.Errorf("rename pending file: %w", err)
				}

				inputs = append(inputs, pending)
				pendingFiles = append(pendingFiles, pending)
			}

			if err := CompactParquetFiles(compactingPath, inputs); err != nil {
				return fmt.Errorf("compact parquet files: %w", err)
			}

			for _, pending := range pendingFiles {
				if err := os.Remove(pending); err != nil {
					return fmt.Errorf("remove pending file: %w", err)
				}
			}

			if err := os.Rename(compactingPath, finalPath); err != nil {
				return fmt.Errorf("rename compacting file: %w", err)
			}

			tx, err := database.Begin()
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}

			if err := database.MoveActiveToArchive(tx, group, finalPath); err != nil {
				tx.Rollback()
				return fmt.Errorf("move metadata to archive: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit transaction: %w", err)
			}

			logger.Infof(
				"compacted ticker=%s files=%d",
				ticker,
				len(group),
			)
		}
	}

	return nil
}
