package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func (db *DB) LoadCompactCandidates() ([]config.CompactCandidate, error) {
	query := `
		SELECT
			ticker,
			parquet_path,
			MAX(row_group_count) AS row_group_count,
			MIN(expiry_date) AS first_expiry
		FROM active_metadata
		WHERE
			status = 'processed'
			AND parquet_path IS NOT NULL
		GROUP BY
			ticker,
			parquet_path
		ORDER BY
			ticker,
			first_expiry
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query compact candidates: %w", err)
	}
	defer rows.Close()

	result := make([]config.CompactCandidate, 0)

	for rows.Next() {
		var row config.CompactCandidate
		var expiry string

		err := rows.Scan(
			&row.Ticker,
			&row.ParquetPath,
			&row.RowGroupCount,
			&expiry,
		)
		if err != nil {
			return nil, fmt.Errorf("scan compact candidate: %w", err)
		}

		row.FirstExpiry, err = time.Parse("2006-01-02", expiry[:10])
		if err != nil {
			return nil, fmt.Errorf("parse expiry: %w", err)
		}

		result = append(result, row)
	}

	return result, rows.Err()
}

func (db *DB) MoveActiveToArchive(
	tx *sql.Tx,
	files []config.CompactCandidate,
	compactedPath string,
) error {
	startRowGroup := 0

	for _, file := range files {
		_, err := tx.Exec(
			`
			INSERT INTO archive_metadata (
				ticker,
				expiry_date,
				row_count,
				parquet_path,
				start_row_group,
				row_group_count,
				created_at
			)
			SELECT
				ticker,
				expiry_date,
				row_count,
				?,
				?,
				row_group_count,
				created_at
			FROM active_metadata
			WHERE parquet_path = ?
			`,
			compactedPath,
			startRowGroup,
			file.ParquetPath,
		)
		if err != nil {
			return fmt.Errorf("insert archive metadata: %w", err)
		}

		startRowGroup += file.RowGroupCount

		_, err = tx.Exec(
			`
			DELETE FROM active_metadata
			WHERE parquet_path = ?
			`,
			file.ParquetPath,
		)
		if err != nil {
			return fmt.Errorf("delete active metadata: %w", err)
		}
	}

	return nil
}
