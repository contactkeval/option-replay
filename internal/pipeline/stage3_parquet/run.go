package stage3_parquet

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	logger.Infof("stage3 parquet started")

	transientDBPath := filepath.Join(
		cfg.SQLiteRoot,
		"transient.db",
	)

	metadataDBPath := filepath.Join(
		cfg.MetadataRoot,
		"metadata.db",
	)

	transientDB, err := OpenSQLiteDB(
		transientDBPath,
	)

	if err != nil {
		return fmt.Errorf(
			"open transient db: %w",
			err,
		)
	}

	defer transientDB.Close()

	metadataDB, err := OpenSQLiteDB(
		metadataDBPath,
	)

	if err != nil {
		return fmt.Errorf(
			"open metadata db: %w",
			err,
		)
	}

	defer metadataDB.Close()

	err = EnsureMetadataTable(
		metadataDB,
	)

	if err != nil {
		return fmt.Errorf(
			"ensure metadata table: %w",
			err,
		)
	}

	// -------------------------------------------------
	// Discover newly expired expiry tables
	// -------------------------------------------------

	today := time.Now().UTC() // TODO: for testing use a fixed date
	err = DiscoverExpiredTables(
		transientDB,
		metadataDB,
		today,
	)

	if err != nil {
		return fmt.Errorf(
			"discover expired tables: %w",
			err,
		)
	}

	workDone := true

	for workDone {

		workDone = false

		// -------------------------------------------------
		// Load active metadata rows
		// -------------------------------------------------

		activeRows, err := LoadCreatedRows(
			metadataDB,
		)

		if err != nil {
			return fmt.Errorf(
				"load active metadata rows: %w",
				err,
			)
		}
		grouped := GroupMetadataRowsByTicker(
			activeRows,
		)

		// -------------------------------------------------
		// Process ticker by ticker
		// -------------------------------------------------

		for ticker, rows := range grouped {

			eligibleRows := SelectEligibleMetadataRows(
				rows,
				config.TargetRowsPerRowGroup,
				config.MaxTrailingRows,
			)

			// ---------------------------------------------
			// nothing eligible yet
			// ---------------------------------------------

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

			// ---------------------------------------------
			// load rows from transient sqlite
			// ---------------------------------------------

			allRows, err := LoadTickerRows(
				transientDB,
				eligibleRows,
			)

			if err != nil {
				return fmt.Errorf(
					"load ticker rows for %s: %w",
					ticker,
					err,
				)
			}

			// ---------------------------------------------
			// sanity validation
			// ---------------------------------------------

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

			// ---------------------------------------------
			// build physical parquet rowgroups
			// ---------------------------------------------

			rowGroups := BuildPhysicalRowGroups(
				allRows,
			)

			if len(rowGroups) == 0 {

				logger.Infof(
					"ticker=%s no rowgroups built",
					ticker,
				)

				continue
			}

			// ---------------------------------------------
			// parquet write
			// ---------------------------------------------

			parquetPath, err := WriteTinyParquet(
				cfg,
				ticker,
				eligibleRows[0].ExpiryDate,
				rowGroups,
			)

			if err != nil {
				return fmt.Errorf(
					"write parquet for %s: %w",
					ticker,
					err,
				)
			}

			// ---------------------------------------------
			// metadata update
			// ---------------------------------------------

			metadataDBtx, err := metadataDB.Begin()
			if err != nil {
				return fmt.Errorf(
					"begin metadata update transaction: %w",
					err,
				)
			}
			defer metadataDBtx.Rollback()

			for _, row := range eligibleRows {

				err := UpdateMetadataProcessed(
					metadataDBtx,
					ticker,
					row.ExpiryDate,
					parquetPath,
					len(rowGroups),
				)

				if err != nil {
					return fmt.Errorf(
						"update metadata processed ticker=%s expiry=%s: %w",
						ticker,
						row.ExpiryDate.Format("2006-01-02"),
						err,
					)
				}
			}

			// ---------------------------------------------
			// cleanup processed transient sqlite rows
			// ---------------------------------------------

			transientDBtx, err := transientDB.Begin()
			if err != nil {
				return fmt.Errorf(
					"begin transient cleanup transaction: %w",
					err,
				)
			}
			defer transientDBtx.Rollback()

			for _, row := range eligibleRows {

				err := DeleteProcessedTickerRows(
					transientDBtx,
					ticker,
					row.ExpiryDate,
				)

				if err != nil {
					return fmt.Errorf(
						"delete transient rows ticker=%s expiry=%s: %w",
						ticker,
						row.ExpiryDate.Format("2006-01-02"),
						err,
					)
				}
			}

			err = transientDBtx.Commit()
			if err != nil {
				transientDBtx.Rollback()
				return fmt.Errorf(
					"commit transient cleanup transaction: %w",
					err,
				)
			}

			err = metadataDBtx.Commit()
			if err != nil {
				metadataDBtx.Rollback()
				return fmt.Errorf(
					"commit metadata update transaction: %w",
					err,
				)
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
