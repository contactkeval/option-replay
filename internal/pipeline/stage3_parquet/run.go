package stage3_parquet

import (
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
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

	transientDB, err := OpenMetadataDB(
		transientDBPath,
	)

	if err != nil {
		return err
	}
	defer transientDB.Close()

	metadataDB, err := OpenMetadataDB(
		metadataDBPath,
	)

	if err != nil {
		return err
	}
	defer metadataDB.Close()

	err = EnsureMetadataTable(
		metadataDB,
	)

	if err != nil {
		return err
	}

	// today := time.Now().UTC()

	today := time.Date(
		2024,
		5,
		22,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	err = DiscoverExpiredTables(
		transientDB,
		metadataDB,
		today,
	)

	if err != nil {
		return err
	}

	activeRows, err := LoadCreatedRows(
		metadataDB,
	)

	if err != nil {
		return err
	}

	grouped := GroupMetadataRowsByTicker(
		activeRows,
	)

	for ticker, rows := range grouped {

		allRows, err := LoadTickerRows(
			transientDB,
			rows,
		)

		if err != nil {
			return err
		}

		rowGroups := BuildRowGroups(
			allRows,
			constants.TargetRowsPerRowGroup,
			constants.MaxTrailingRows,
		)

		if len(rowGroups) == 0 {

			for _, row := range rows {

				err := UpdateMetadataPending(
					metadataDB,
					ticker,
					row.ExpiryDate,
				)

				if err != nil {
					return err
				}
			}

			continue
		}

		parquetPath, err := WriteTinyParquet(
			cfg,
			ticker,
			rows[0].ExpiryDate,
			rowGroups,
		)

		if err != nil {
			return err
		}

		for _, row := range rows {

			err := UpdateMetadataProcessed(
				metadataDB,
				ticker,
				row.ExpiryDate,
				parquetPath,
				len(rowGroups),
			)

			if err != nil {
				return err
			}
		}

		logger.Infof(
			"ticker=%s rowgroups=%d parquet=%s",
			ticker,
			len(rowGroups),
			parquetPath,
		)
	}

	return nil
}
