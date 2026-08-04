package stage3_parquet

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {
	logger.Infof("stage3 parquet started")

	transientDBPath := filepath.Join(cfg.SQLiteRoot, "transient.db")
	metadataDBPath := filepath.Join(cfg.MetadataRoot, "metadata.db")

	transientDB, err := db.Open(db.Options{
		Path:    transientDBPath,
		Schemas: db.SchemaTransient,
	})
	if err != nil {
		return fmt.Errorf("open transient db: %w", err)
	}
	defer transientDB.Close()

	metadataDB, err := db.Open(db.Options{
		Path:    metadataDBPath,
		Schemas: db.SchemaParquet,
	})
	if err != nil {
		return fmt.Errorf("open metadata db: %w", err)
	}
	defer metadataDB.Close()

	today := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)

	if err := DiscoverExpiredTables(transientDB, metadataDB, today); err != nil {
		return fmt.Errorf("discover expired tables: %w", err)
	}

	workDone := true

	for workDone {
		workDone = false

		activeRows, err := metadataDB.LoadCreatedRows()
		if err != nil {
			return fmt.Errorf("load active metadata rows: %w", err)
		}

		grouped := GroupMetadataRowsByTicker(activeRows)

		for ticker, rows := range grouped {
			eligibleRows := SelectEligibleMetadataRows(
				rows,
				config.MaxRowsPerRowGroup,
				config.MaxShortRows,
			)

			if len(eligibleRows) == 0 {
				logger.Infof(
					"ticker=%s pending rows not sufficient yet",
					ticker,
				)
				continue
			}

			logger.Infof(
				"ticker=%s eligible expiries=%d",
				ticker,
				len(eligibleRows),
			)

			allRows, err := transientDB.LoadTickerBars(eligibleRows)
			if err != nil {
				return fmt.Errorf("load ticker rows for %s: %w", ticker, err)
			}

			expectedRows := 0
			for _, row := range eligibleRows {
				expectedRows += row.RowCount
			}

			if expectedRows != len(allRows) {
				return fmt.Errorf(
					"row mismatch ticker=%s metadata=%d actual=%d",
					ticker,
					expectedRows,
					len(allRows),
				)
			}

			rowGroups := BuildPhysicalRowGroups(allRows)
			if len(rowGroups) == 0 {
				logger.Infof("ticker=%s no rowgroups built", ticker)
				continue
			}

			parquetPath, err := WriteTinyParquet(
				cfg,
				ticker,
				eligibleRows[0].ExpiryDate,
				rowGroups,
			)
			if err != nil {
				return fmt.Errorf("write parquet for %s: %w", ticker, err)
			}

			metadataDBtx, err := metadataDB.Begin()
			if err != nil {
				return fmt.Errorf("begin metadata update transaction: %w", err)
			}
			defer metadataDBtx.Rollback()

			for _, row := range eligibleRows {
				if err := metadataDB.UpdateActiveProcessed(
					metadataDBtx,
					ticker,
					row.ExpiryDate,
					parquetPath,
					len(rowGroups),
				); err != nil {
					return fmt.Errorf(
						"update metadata processed ticker=%s expiry=%s: %w",
						ticker,
						row.ExpiryDate.Format("2006-01-02"),
						err,
					)
				}
			}

			transientDBtx, err := transientDB.Begin()
			if err != nil {
				return fmt.Errorf("begin transient cleanup transaction: %w", err)
			}
			defer transientDBtx.Rollback()

			for _, row := range eligibleRows {
				if err := transientDB.DeleteProcessedTickerRows(
					transientDBtx,
					ticker,
					row.ExpiryDate,
				); err != nil {
					return fmt.Errorf(
						"delete transient rows ticker=%s expiry=%s: %w",
						ticker,
						row.ExpiryDate.Format("2006-01-02"),
						err,
					)
				}
			}

			if err := transientDBtx.Commit(); err != nil {
				return fmt.Errorf("commit transient cleanup transaction: %w", err)
			}

			if err := metadataDBtx.Commit(); err != nil {
				return fmt.Errorf("commit metadata update transaction: %w", err)
			}

			workDone = true

			logger.Infof(
				"ticker=%s parquet=%s rowgroups=%d rows=%d",
				ticker,
				parquetPath,
				len(rowGroups),
				len(allRows),
			)
		}
	}

	logger.Infof("stage3 parquet completed")
	return nil
}
