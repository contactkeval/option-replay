package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func (db *DB) EnsureExpiryTable(
	tx *sql.Tx,
	expiry string,
) error {
	table := expiryTableName(expiry)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			ticker       TEXT NOT NULL,
			strike       INTEGER NOT NULL,
			option_type  INTEGER NOT NULL,
			window_start INTEGER NOT NULL,

			open  INTEGER NOT NULL,
			high  INTEGER NOT NULL,
			low   INTEGER NOT NULL,
			close INTEGER NOT NULL,

			volume       INTEGER NOT NULL,
			transactions INTEGER NOT NULL,

			PRIMARY KEY (
				ticker,
				strike,
				option_type,
				window_start
			)
		)
	`, table)

	if _, err := tx.Exec(query); err != nil {
		return err
	}

	return createExpiryIndexes(tx, table)
}

func createExpiryIndexes(tx *sql.Tx, table string) error {
	queries := []string{
		fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS idx_%s_ticker
			ON %s(ticker)
		`, table, table),
		fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS idx_%s_scan
			ON %s(
				ticker,
				strike,
				option_type,
				window_start
			)
		`, table, table),
	}

	for _, q := range queries {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) InsertBars(
	tx *sql.Tx,
	expiry string,
	bars []config.TransientRow,
) error {
	table := expiryTableName(expiry)

	query := fmt.Sprintf(`
		INSERT OR IGNORE INTO %s (
			ticker,
			strike,
			option_type,
			window_start,
			open,
			high,
			low,
			close,
			volume,
			transactions
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, table)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	ignored := 0

	for _, bar := range bars {
		result, err := stmt.Exec(
			bar.Ticker,
			bar.Strike,
			bar.OptionType,
			bar.WindowStart,
			bar.Open,
			bar.High,
			bar.Low,
			bar.Close,
			bar.Volume,
			bar.Transactions,
		)
		if err != nil {
			return fmt.Errorf("execute statement: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		if affected == 1 {
			inserted++
		} else {
			ignored++
		}
	}

	if ignored > 0 {
		logger.Warnf("Expiry=%s: Inserted: %d, Ignored: %d", expiry, inserted, ignored)
	}

	return nil
}

func (db *DB) InsertRejectedRow(
	tx *sql.Tx,
	rawLine string,
	reason string,
	sourceFile string,
) error {
	query := `
		INSERT INTO rejected_rows (
			raw_line,
			reason,
			source_file
		)
		VALUES (?, ?, ?)
	`

	_, err := tx.Exec(query, rawLine, reason, sourceFile)
	return err
}

func (db *DB) ListExpiryTables() ([]string, error) {
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

		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan expiry tables: %w", err)
		}

		tables = append(tables, table)
	}

	return tables, rows.Err()
}

func (db *DB) CountTickerRowsInExpiryTable(
	table string,
) (map[string]int, error) {
	query := fmt.Sprintf(`
		SELECT ticker, COUNT(*)
		FROM %s
		GROUP BY ticker
	`, table)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)

	for rows.Next() {
		var ticker string
		var rowCount int

		if err := rows.Scan(&ticker, &rowCount); err != nil {
			return nil, err
		}

		counts[ticker] = rowCount
	}

	return counts, rows.Err()
}

func (db *DB) LoadTickerBars(
	metadataRows []config.ActiveMetadataRow,
) ([]config.ParquetRow, error) {
	var result []config.ParquetRow

	for _, meta := range metadataRows {
		expiryString := meta.ExpiryDate.Format("20060102")
		expiry := util.EncodeExpiryDate(meta.ExpiryDate)

		table := fmt.Sprintf("options_%s", expiryString)

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

		rows, err := db.Query(query, meta.Ticker)
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

			result = append(result, row)
		}

		if err := rows.Close(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (db *DB) DeleteProcessedTickerRows(
	tx *sql.Tx,
	ticker string,
	expiryDate time.Time,
) error {
	tableName := fmt.Sprintf(
		"options_%s",
		expiryDate.Format("20060102"),
	)

	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE ticker = ?
	`, tableName)

	if _, err := tx.Exec(query, ticker); err != nil {
		return err
	}

	return dropTableIfEmpty(tx, tableName)
}

func dropTableIfEmpty(tx *sql.Tx, tableName string) error {
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM %s
	`, tableName)

	var count int

	if err := tx.QueryRow(query).Scan(&count); err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	dropQuery := fmt.Sprintf(`DROP TABLE %s`, tableName)
	_, err := tx.Exec(dropQuery)
	return err
}

func ParseExpiryFromTable(table string) (time.Time, error) {
	expiryString := strings.TrimPrefix(table, "options_")
	return time.Parse("20060102", expiryString)
}
