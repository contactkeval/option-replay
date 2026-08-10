package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (db *DB) InsertContractIgnore(
	tx *sql.Tx,
	underlying string,
	expiry time.Time,
	strike float64,
	contractType string,
	groupNo int,
	firstSeenDate time.Time,
) error {
	seen := firstSeenDate.Format("2006-01-02")
	query := `
		INSERT OR IGNORE INTO contracts (
			underlying,
			expiry,
			strike,
			type,
			groupNo,
			firstSeenDate,
			lastDownloadedDate
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := tx.Exec(
		query,
		underlying,
		expiry.Format("2006-01-02"),
		strike,
		contractType,
		groupNo,
		seen,
		seen,
	)

	return err
}

func (db *DB) GetDistinctExpiries() ([]time.Time, error) {
	rows, err := db.Query(`
		SELECT DISTINCT expiry
		FROM contracts
		WHERE expiry < date('now')
			AND archived = 0
		ORDER BY expiry DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expiries []time.Time

	for rows.Next() {
		var expiry string

		if err := rows.Scan(&expiry); err != nil {
			return nil, err
		}

		t, err := time.Parse("2006-01-02", expiry)
		if err != nil {
			return nil, err
		}

		expiries = append(expiries, t)
	}

	return expiries, rows.Err()
}

func (db *DB) GetContractsByExpiries(
	expiries []string,
) ([]Contract, error) {
	if len(expiries) == 0 {
		return nil, nil
	}

	query := `
		SELECT
			serialNo,
			underlying,
			expiry,
			type,
			strike,
			groupNo,
			barCount,
			lastDownloadedDate,
			downloadAttempts,
			archived
		FROM contracts
		WHERE archived = 0
			AND expiry IN (
	`

	args := make([]any, 0, len(expiries))

	for i, expiry := range expiries {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, expiry)
	}

	query += ")"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) GetContractsByGroupNo(
	groupNo int,
) ([]Contract, error) {
	rows, err := db.Query(`
		SELECT
			serialNo,
			underlying,
			expiry,
			type,
			strike,
			groupNo,
			barCount,
			lastDownloadedDate,
			downloadAttempts,
			archived
		FROM contracts
		WHERE
			archived = 0
			AND expiry > date('now', '+1 month')
			AND groupNo = ?
	`, groupNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContracts(rows)
}

// ListActiveContracts returns non-archived contracts with fetch metadata.
func (db *DB) ListActiveContracts() ([]Contract, error) {
	rows, err := db.Query(`
		SELECT
			serialNo,
			underlying,
			expiry,
			type,
			strike,
			groupNo,
			barCount,
			lastDownloadedDate,
			downloadAttempts,
			archived
		FROM contracts
		WHERE archived = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) GetBatchContracts(
	runNo int64,
	batchNo int,
) ([]Contract, error) {
	rows, err := db.Query(`
		SELECT
			c.serialNo,
			c.underlying,
			c.expiry,
			c.type,
			c.strike,
			c.groupNo,
			c.barCount,
			c.lastDownloadedDate,
			c.downloadAttempts,
			c.archived
		FROM batch_contracts bc
		JOIN contracts c
			ON c.serialNo = bc.serialNo
		WHERE
			bc.runNo = ?
			AND bc.batchNo = ?
		ORDER BY bc.listNo
	`, runNo, batchNo)
	if err != nil {
		return nil, fmt.Errorf("query batch contracts: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) CountContracts() (int, error) {
	var count int

	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
	`).Scan(&count)

	return count, err
}

func (db *DB) GetRunBatchCount(runNo int64) (int, error) {
	var count int

	err := db.QueryRow(`
		SELECT batchCount
		FROM runs
		WHERE runNo = ?
	`, runNo).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("query run batch count: %w", err)
	}

	return count, nil
}

// RecordContractFetch updates bar metadata after a successful download.
// Expired contracts are archived once downloadAttempts reaches 3.
func (db *DB) RecordContractFetch(
	serialNo int64,
	barCount int,
	fetchDate time.Time,
) error {
	_, err := db.Exec(`
		UPDATE contracts
		SET
			barCount = ?,
			lastDownloadedDate = ?,
			downloadAttempts = downloadAttempts + 1,
			archived = CASE
				WHEN expiry < date(?)
					AND downloadAttempts + 1 >= 3
				THEN 1
				ELSE archived
			END
		WHERE serialNo = ?
	`,
		barCount,
		fetchDate.Format("2006-01-02"),
		fetchDate.Format("2006-01-02"),
		serialNo,
	)
	if err != nil {
		return fmt.Errorf("record contract fetch: %w", err)
	}

	return nil
}

func (db *DB) CountCandlesForSerial(serialNo int64) (int, error) {
	var count int

	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM candle_staging
		WHERE serialNo = ?
	`, serialNo).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count candles: %w", err)
	}

	return count, nil
}

func scanContracts(rows *sql.Rows) ([]Contract, error) {
	var contracts []Contract

	for rows.Next() {
		var c Contract
		var expiry string
		var lastDownloaded sql.NullString
		var archived int

		if err := rows.Scan(
			&c.SerialNo,
			&c.Underlying,
			&expiry,
			&c.Type,
			&c.Strike,
			&c.GroupNo,
			&c.BarCount,
			&lastDownloaded,
			&c.DownloadAttempts,
			&archived,
		); err != nil {
			return nil, err
		}

		var err error
		c.Expiry, err = time.Parse("2006-01-02", expiry)
		if err != nil {
			return nil, fmt.Errorf("parse expiry %q for serial %d: %w", expiry, c.SerialNo, err)
		}

		if lastDownloaded.Valid {
			if parsed, ok := parseContractDate(lastDownloaded.String); ok {
				c.LastDownloadedDate = parsed
			}
		}

		c.Archived = archived != 0

		contracts = append(contracts, c)
	}

	return contracts, rows.Err()
}

// parseContractDate parses yyyy-mm-dd (optionally with a time suffix).
// Returns ok=false for empty or malformed values instead of panicking.
func parseContractDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if len(raw) >= 10 {
		raw = raw[:10]
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
