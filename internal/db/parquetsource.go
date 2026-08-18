package db

import (
	"fmt"
	"time"
)

// ParquetSource locates a ticker/expiry inside a parquet file.
// For active files StartRowGroup is 0 and RowGroupCount may be 0 (scan all groups).
type ParquetSource struct {
	Ticker        string
	ExpiryDate    time.Time
	ParquetPath   string
	StartRowGroup int
	RowGroupCount int
}

func (db *DB) LookupParquetSources(
	ticker string,
	fromExpiry, toExpiry time.Time,
) ([]ParquetSource, error) {
	query := `
		SELECT
			ticker,
			expiry_date,
			parquet_path,
			0 AS start_row_group,
			COALESCE(row_group_count, 0) AS row_group_count
		FROM active_metadata
		WHERE
			ticker = ?
			AND parquet_path IS NOT NULL
			AND parquet_path != ''
			AND (? = '' OR expiry_date >= ?)
			AND (? = '' OR expiry_date <= ?)

		UNION ALL

		SELECT
			ticker,
			expiry_date,
			parquet_path,
			start_row_group,
			row_group_count
		FROM archive_metadata
		WHERE
			ticker = ?
			AND (? = '' OR expiry_date >= ?)
			AND (? = '' OR expiry_date <= ?)

		ORDER BY
			expiry_date,
			parquet_path,
			start_row_group
	`

	fromStr := formatOptionalDate(fromExpiry)
	toStr := formatOptionalDate(toExpiry)

	rows, err := db.Query(
		query,
		ticker,
		fromStr, fromStr,
		toStr, toStr,
		ticker,
		fromStr, fromStr,
		toStr, toStr,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup parquet sources: %w", err)
	}
	defer rows.Close()

	result := make([]ParquetSource, 0)
	for rows.Next() {
		var src ParquetSource
		var expiry string

		if err := rows.Scan(
			&src.Ticker,
			&expiry,
			&src.ParquetPath,
			&src.StartRowGroup,
			&src.RowGroupCount,
		); err != nil {
			return nil, fmt.Errorf("scan parquet source: %w", err)
		}

		src.ExpiryDate, err = parseMetadataDate(expiry)
		if err != nil {
			return nil, fmt.Errorf("parse parquet source expiry: %w", err)
		}

		result = append(result, src)
	}

	return result, rows.Err()
}

func (db *DB) ListTickerExpiries(
	ticker string,
	fromExpiry, toExpiry time.Time,
) ([]time.Time, error) {
	query := `
		SELECT DISTINCT expiry_date
		FROM (
			SELECT expiry_date
			FROM active_metadata
			WHERE
				ticker = ?
				AND parquet_path IS NOT NULL
				AND parquet_path != ''
				AND (? = '' OR expiry_date >= ?)
				AND (? = '' OR expiry_date <= ?)

			UNION

			SELECT expiry_date
			FROM archive_metadata
			WHERE
				ticker = ?
				AND (? = '' OR expiry_date >= ?)
				AND (? = '' OR expiry_date <= ?)
		)
		ORDER BY expiry_date
	`

	fromStr := formatOptionalDate(fromExpiry)
	toStr := formatOptionalDate(toExpiry)

	rows, err := db.Query(
		query,
		ticker,
		fromStr, fromStr,
		toStr, toStr,
		ticker,
		fromStr, fromStr,
		toStr, toStr,
	)
	if err != nil {
		return nil, fmt.Errorf("list ticker expiries: %w", err)
	}
	defer rows.Close()

	expiries := make([]time.Time, 0)
	for rows.Next() {
		var expiry string
		if err := rows.Scan(&expiry); err != nil {
			return nil, fmt.Errorf("scan ticker expiry: %w", err)
		}

		parsed, err := parseMetadataDate(expiry)
		if err != nil {
			return nil, fmt.Errorf("parse ticker expiry: %w", err)
		}
		expiries = append(expiries, parsed)
	}

	return expiries, rows.Err()
}

func formatOptionalDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func parseMetadataDate(expiry string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, expiry); err == nil {
		return t.UTC(), nil
	}
	if len(expiry) >= 10 {
		return time.Parse("2006-01-02", expiry[:10])
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", expiry)
}
