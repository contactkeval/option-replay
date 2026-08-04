package db

import (
	"database/sql"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func (db *DB) InsertActiveRow(
	row config.ActiveMetadataRow,
) error {
	query := `
		INSERT OR IGNORE INTO active_metadata (
			ticker,
			expiry_date,
			row_count,
			status
		)
		VALUES (?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		row.Ticker,
		row.ExpiryDate.Format("2006-01-02"),
		row.RowCount,
		row.Status,
	)

	return err
}

func (db *DB) LoadCreatedRows() ([]config.ActiveMetadataRow, error) {
	query := `
		SELECT
			ticker,
			expiry_date,
			row_count,
			status,
			parquet_path,
			row_group_count
		FROM active_metadata
		WHERE status IN ('created', 'pending')
		ORDER BY ticker, expiry_date
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []config.ActiveMetadataRow

	for rows.Next() {
		var row config.ActiveMetadataRow
		var expiry string

		err := rows.Scan(
			&row.Ticker,
			&expiry,
			&row.RowCount,
			&row.Status,
			&row.ParquetPath,
			&row.RowGroupCount,
		)
		if err != nil {
			return nil, err
		}

		row.ExpiryDate, err = time.Parse(time.RFC3339, expiry)
		if err != nil {
			row.ExpiryDate, err = time.Parse("2006-01-02", expiry[:10])
			if err != nil {
				return nil, err
			}
		}

		result = append(result, row)
	}

	return result, rows.Err()
}

func (db *DB) UpdateActiveProcessed(
	tx *sql.Tx,
	ticker string,
	expiryDate time.Time,
	parquetPath string,
	rowGroups int,
) error {
	query := `
		UPDATE active_metadata
		SET
			status = 'processed',
			parquet_path = ?,
			row_group_count = ?
		WHERE
			ticker = ?
			AND expiry_date = ?
	`

	_, err := tx.Exec(
		query,
		parquetPath,
		rowGroups,
		ticker,
		expiryDate.Format("2006-01-02"),
	)

	return err
}

func (db *DB) UpdateActivePending(
	ticker string,
	expiryDate time.Time,
) error {
	query := `
		UPDATE active_metadata
		SET status = 'pending'
		WHERE
			ticker = ?
			AND expiry_date = ?
	`

	_, err := db.Exec(
		query,
		ticker,
		expiryDate.Format("2006-01-02"),
	)

	return err
}
