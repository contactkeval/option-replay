package stage3_parquet

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	_ "modernc.org/sqlite"
)

func SelectEligibleMetadataRows(
	rows []config.ActiveMetadataRow,
	maxRowsPerRowGroup int,
	maxShortRows int,
) []config.ActiveMetadataRow {

	if len(rows) == 0 {
		return nil
	}

	selected := []config.ActiveMetadataRow{
		rows[0],
	}

	pendingRows := rows[0].RowCount

	for i := 1; i < len(rows); i++ {

		nextRows := rows[i].RowCount

		// ------------------------------------
		// Current parquet candidate capacity
		// ------------------------------------
		//
		// Example:
		// pendingRows=414k -> capacity=512k
		// pendingRows=620k -> capacity=768k
		//
		capacity :=
			((pendingRows + maxRowsPerRowGroup - 1) /
				maxRowsPerRowGroup) *
				maxRowsPerRowGroup

		// ------------------------------------
		// Next expiry still fits
		// ------------------------------------
		if pendingRows+nextRows <= capacity {

			selected = append(
				selected,
				rows[i],
			)

			pendingRows += nextRows

			continue
		}

		// ------------------------------------
		// Next expiry would overflow
		// ------------------------------------

		shortfall :=
			capacity - pendingRows

		// Candidate is sufficiently full.
		//
		// IMPORTANT:
		// Only cut when:
		// 1. next expiry would overflow
		// 2. remaining space <= maxShortRows
		//
		if shortfall <= maxShortRows {

			return selected
		}

		// ------------------------------------
		// Candidate not full enough yet.
		// Add next expiry and continue.
		// ------------------------------------

		selected = append(
			selected,
			rows[i],
		)

		pendingRows += nextRows
	}

	// No next expiry available.
	// Can't safely create a parquet candidate.
	return nil
}

func LoadTickerRows(
	db *sql.DB,
	metadataRows []config.ActiveMetadataRow,
) ([]config.ParquetRow, error) {

	var result []config.ParquetRow

	for _, meta := range metadataRows {

		expiryString := meta.ExpiryDate.Format(
			"20060102",
		)
		expiry := uint32((meta.ExpiryDate.Year() * 100000) + (int(meta.ExpiryDate.Month()) * 100) + meta.ExpiryDate.Day())

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

			var row config.ParquetRow

			row.ExpiryDate = expiry
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
	rows []config.ActiveMetadataRow,
) map[string][]config.ActiveMetadataRow {

	result := make(
		map[string][]config.ActiveMetadataRow,
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
	if err != nil {
		return fmt.Errorf("create active_metadata table: %w", err)
	}

	query = `
	CREATE TABLE IF NOT EXISTS archive_metadata (
		ticker TEXT NOT NULL,
		expiry_date DATE NOT NULL,

		row_count INTEGER NOT NULL,

		parquet_path TEXT NOT NULL,

		start_row_group INTEGER NOT NULL,
		row_group_count INTEGER NOT NULL,

		created_at DATETIME NOT NULL,
		archived_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	
		PRIMARY KEY (
			ticker,
			expiry_date
		)
	);
	`

	if _, err = db.Exec(query); err != nil {
		return fmt.Errorf("create archive_metadata table: %w", err)
	}

	return nil
}

func InsertMetadataRow(
	db *sql.DB,
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

func LoadCreatedRows(
	db *sql.DB,
) ([]config.ActiveMetadataRow, error) {

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
