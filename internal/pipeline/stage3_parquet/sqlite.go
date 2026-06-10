package stage3_parquet

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func DiscoverExpiredTables(
	transientDB *sql.DB,
	metadataDB *sql.DB,
	today time.Time,
) error {

	tables, err := ListExpiryTables(
		transientDB,
	)

	if err != nil {
		return fmt.Errorf("list expiry tables: %w", err)
	}

	for _, table := range tables {

		expiryString := strings.TrimPrefix(
			table,
			"options_",
		)

		expiryDate, err := time.Parse(
			"20060102",
			expiryString,
		)

		if err != nil {
			return fmt.Errorf("parse expiry date: %w", err)
		}

		if !expiryDate.Before(today) {
			continue
		}

		query := fmt.Sprintf(`
		SELECT
			ticker,
			COUNT(*)
		FROM %s
		GROUP BY ticker
		`, table)

		rows, err := transientDB.Query(query)
		if err != nil {
			return fmt.Errorf("query transient db: %w", err)
		}

		for rows.Next() {

			var ticker string
			var rowCount int

			err := rows.Scan(
				&ticker,
				&rowCount,
			)

			if err != nil {
				rows.Close()
				return fmt.Errorf("scan transient db rows: %w", err)
			}

			err = InsertMetadataRow(
				metadataDB,
				model.ActiveMetadataRow{
					Ticker: ticker,

					ExpiryDate: expiryDate,

					RowCount: rowCount,

					Status: "created",
				},
			)

			if err != nil {
				rows.Close()
				return fmt.Errorf("insert metadata row: %w", err)
			}
		}

		rows.Close()
	}

	return nil
}

func ListExpiryTables(
	db *sql.DB,
) ([]string, error) {

	query := `
	SELECT name
	FROM sqlite_master
	WHERE type='table'
	AND name LIKE 'options_%'
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query expiry tables: %w", err)
	}
	defer rows.Close()

	var tables []string

	for rows.Next() {

		var table string

		err := rows.Scan(&table)
		if err != nil {
			return nil, fmt.Errorf("scan expiry tables: %w", err)
		}

		tables = append(tables, table)
	}

	return tables, rows.Err()
}

func OpenSQLiteDB(path string) (*sql.DB, error) {

	db, err := sql.Open(
		"sqlite",
		path,
	)

	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	queries := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return nil, fmt.Errorf("execute pragma: %w", err)
		}
	}

	return db, nil
}
