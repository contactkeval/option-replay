package stage3_parquet

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func OpenMetadataDB(path string) (*sql.DB, error) {

	db, err := sql.Open(
		"sqlite",
		path,
	)

	if err != nil {
		return nil, err
	}

	queries := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return nil, err
		}
	}

	return db, nil
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
	db *sql.DB,
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

	_, err := db.Exec(
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
