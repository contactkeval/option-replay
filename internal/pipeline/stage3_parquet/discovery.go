package stage3_parquet

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func DiscoverExpiredTables(
	transientDB *db.DB,
	metadataDB *db.DB,
	today time.Time,
) error {
	tables, err := transientDB.ListExpiryTables()
	if err != nil {
		return fmt.Errorf("list expiry tables: %w", err)
	}

	for _, table := range tables {
		expiryDate, err := db.ParseExpiryFromTable(table)
		if err != nil {
			return fmt.Errorf("parse expiry date: %w", err)
		}

		if !expiryDate.Before(today) {
			continue
		}

		counts, err := transientDB.CountTickerRowsInExpiryTable(table)
		if err != nil {
			return fmt.Errorf("query transient db: %w", err)
		}

		for ticker, rowCount := range counts {
			err = metadataDB.InsertActiveRow(config.ActiveMetadataRow{
				Ticker:     ticker,
				ExpiryDate: expiryDate,
				RowCount:   rowCount,
				Status:     "created",
			})
			if err != nil {
				return fmt.Errorf("insert metadata row: %w", err)
			}
		}
	}

	return nil
}
