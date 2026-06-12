package stage4_compact

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(
	cfg config.Config,
) error {

	logger.Infof(
		"Stage 4 compact started",
	)

	metadataPath := filepath.Join(
		cfg.MetadataRoot,
		"metadata.db",
	)

	db, err := sql.Open(
		"sqlite",
		metadataPath,
	)

	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}

	defer db.Close()

	for {

		rows, err := LoadCompactCandidates(
			db,
		)

		logger.Infof(
			"loaded candidates=%d",
			len(rows),
		)

		if err != nil {
			return fmt.Errorf("load compact candidates: %w", err)
		}

		candidateGroups := SelectCompactCandidates(
			rows,
			config.TargetRowGroupsPerFile,
		)

		logger.Infof(
			"candidate groups=%d",
			len(candidateGroups),
		)

		if len(candidateGroups) == 0 {
			break
		}

		for _, group := range candidateGroups {

			if len(group) == 0 {
				continue
			}

			ticker := group[0].Ticker

			firstExpiry := group[0].FirstExpiry.Format(
				"20060102",
			)

			outputDir := filepath.Join(
				cfg.ParquetRoot,
				ticker,
			)

			err := os.MkdirAll(
				outputDir,
				0755,
			)

			if err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}

			finalPath := filepath.Join(
				outputDir,
				fmt.Sprintf(
					"%s_%s.parquet",
					ticker,
					firstExpiry,
				),
			)

			compactingPath := finalPath + ".compacting"

			inputs := make(
				[]string,
				0,
			)

			pendingFiles := make(
				[]string,
				0,
			)

			for _, file := range group {

				// logger.Infof(
				// 	"candidate parquet=%s expiry=%s rowgroups=%d",
				// 	file.ParquetPath,
				// 	file.FirstExpiry.Format("2006-01-02"),
				// 	file.RowGroupCount,
				// )

				if _, err := os.Stat(file.ParquetPath); err != nil {
					logger.Infof(
						"candidate file missing=%s",
						file.ParquetPath,
					)
				}

				pending, err := RenamePending(
					file.ParquetPath,
				)

				if err != nil {
					return fmt.Errorf("rename pending file: %w", err)
				}

				inputs = append(
					inputs,
					pending,
				)

				pendingFiles = append(
					pendingFiles,
					pending,
				)
			}

			err = CompactParquetFiles(
				compactingPath,
				inputs,
			)

			if err != nil {
				return fmt.Errorf("compact parquet files: %w", err)
			}

			for _, pending := range pendingFiles {

				err := os.Remove(
					pending,
				)

				if err != nil {
					return fmt.Errorf("remove pending file: %w", err)
				}
			}

			err = os.Rename(
				compactingPath,
				finalPath,
			)

			if err != nil {
				return fmt.Errorf("rename compacting file: %w", err)
			}

			tx, err := db.Begin()

			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}

			err = MoveMetadataToArchive(
				tx,
				group,
				finalPath,
			)

			if err != nil {

				tx.Rollback()

				return fmt.Errorf("move metadata to archive: %w", err)
			}

			err = tx.Commit()

			if err != nil {
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
