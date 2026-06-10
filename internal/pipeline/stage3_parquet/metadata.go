package stage3_parquet

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
	_ "modernc.org/sqlite"
)

func SelectEligibleMetadataRows(
	rows []model.ActiveMetadataRow,
	targetRows int,
	maxTrailingRows int,
) []model.ActiveMetadataRow {

	selected := make(
		[]model.ActiveMetadataRow,
		0,
	)

	total := 0

	for _, row := range rows {

		selected = append(
			selected,
			row,
		)

		total += row.RowCount

		quotient := total / targetRows
		remainder := total % targetRows

		if quotient > 0 &&
			remainder < maxTrailingRows {

			return selected
		}
	}

	return nil
}

func LoadTickerRows(
	db *sql.DB,
	metadataRows []model.ActiveMetadataRow,
) ([]model.ParquetRow, error) {

	var result []model.ParquetRow

	for _, meta := range metadataRows {

		expiryString := meta.ExpiryDate.Format(
			"20060102",
		)

		table := fmt.Sprintf(
			"options_%s",
			expiryString,
		)

		query := fmt.Sprintf(`
		SELECT
			strike,
			option_type,
			window_start,
			open,
			high,
			low,
			close,
			volume,
			transactions
		FROM %s
		WHERE ticker = ?
		ORDER BY
			strike,
			option_type,
			window_start
		`, table)

		rows, err := db.Query(
			query,
			meta.Ticker,
		)

		if err != nil {
			return nil, err
		}

		for rows.Next() {

			var row model.ParquetRow

			err := rows.Scan(
				&row.Strike,
				&row.OptionType,
				&row.WindowStart,
				&row.Open,
				&row.High,
				&row.Low,
				&row.Close,
				&row.Volume,
				&row.Transactions,
			)

			if err != nil {
				rows.Close()
				return nil, err
			}

			result = append(
				result,
				row,
			)
		}

		rows.Close()
	}

	return result, nil
}

func GroupMetadataRowsByTicker(
	rows []model.ActiveMetadataRow,
) map[string][]model.ActiveMetadataRow {

	result := make(
		map[string][]model.ActiveMetadataRow,
	)

	for _, row := range rows {

		result[row.Ticker] = append(
			result[row.Ticker],
			row,
		)
	}

	return result
}

func EnsureMetadataTable(db *sql.DB) error {

	query := `
	CREATE TABLE IF NOT EXISTS active_metadata (
		ticker TEXT NOT NULL,
		expiry_date DATE NOT NULL,

		row_count INTEGER NOT NULL,

		status TEXT NOT NULL,

		parquet_path TEXT,
		row_group_count INTEGER,

		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

		PRIMARY KEY (
			ticker,
			expiry_date
		)
	);
	`

	_, err := db.Exec(query)

	return err
}

func InsertMetadataRow(
	db *sql.DB,
	row model.ActiveMetadataRow,
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

func LoadCreatedRows(
	db *sql.DB,
) ([]model.ActiveMetadataRow, error) {

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

	var result []model.ActiveMetadataRow

	for rows.Next() {

		var row model.ActiveMetadataRow

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

		row.ExpiryDate, err = time.Parse(
			time.RFC3339,
			expiry,
		)

		if err != nil {
			return nil, err
		}

		result = append(result, row)
	}

	return result, rows.Err()
}

func UpdateMetadataProcessed(
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

func UpdateMetadataPending(
	db *sql.DB,
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
