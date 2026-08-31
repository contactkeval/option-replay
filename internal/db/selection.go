package db

import (
	"fmt"
	"time"
)

const contractSelectCols = `
	serialNo,
	underlying,
	expiry,
	type,
	strike,
	barCount,
	lastDownloadedDate,
	downloadAttempts,
	archived
`

// optionContractsFilter excludes underlying spot rows from option selection.
const optionContractsFilter = `AND type != 'spot'`

// CountExpiredContracts returns contracts with expiry before runDate.
func (db *DB) CountExpiredContracts(runDate time.Time) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry < ?
	`, formatDate(runDate)).Scan(&count)
	return count, err
}

// CountFarExpiryContracts returns contracts with expiry after runDate+1 month.
func (db *DB) CountFarExpiryContracts(runDate time.Time) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry > date(?, '+1 month')
	`, formatDate(runDate)).Scan(&count)
	return count, err
}

// CountExpiredOnDate returns expired contracts with the given expiry date.
func (db *DB) CountExpiredOnDate(expiry time.Time) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry = ?
	`, formatDate(expiry)).Scan(&count)
	return count, err
}

// SelectExpiredPreviousDay returns up to limit contracts that expired on
// previousDay, ordered by barCount ascending.
func (db *DB) SelectExpiredPreviousDay(
	previousDay time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry = ?
		ORDER BY barCount ASC, serialNo ASC
		LIMIT ?
	`, formatDate(previousDay), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired previous day: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectExpiredOldestExpiry returns up to limit contracts with expiry before
// beforeDate (typically yesterday), ordered by expiry ASC, barCount DESC.
func (db *DB) SelectExpiredOldestExpiry(
	beforeDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry < ?
		ORDER BY expiry ASC, barCount DESC, serialNo ASC
		LIMIT ?
	`, formatDate(beforeDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired oldest expiry: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectExpiredHighestBar returns up to limit contracts with expiry in
// [fromDate, toDate] inclusive, ordered by barCount DESC.
// Used for date-band partitioning after the oldest-expiry slice:
// fromDate = maxExpiryFromOldest+1, toDate = yesterday-1.
func (db *DB) SelectExpiredHighestBar(
	fromDate time.Time,
	toDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	if fromDate.After(toDate) {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry >= ?
			AND expiry <= ?
		ORDER BY barCount DESC, serialNo ASC
		LIMIT ?
	`, formatDate(fromDate), formatDate(toDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired highest bar: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarOldestFetch returns up to limit stale far-expiry contracts ordered
// by lastDownloadedDate ASC (NULLs first), barCount DESC.
func (db *DB) SelectFarOldestFetch(
	runDate time.Time,
	staleBefore time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry > date(?, '+1 month')
			AND (
				lastDownloadedDate IS NULL
				OR TRIM(lastDownloadedDate) = ''
				OR date(lastDownloadedDate) < date(?)
			)
		ORDER BY
			CASE
				WHEN lastDownloadedDate IS NULL OR TRIM(lastDownloadedDate) = ''
				THEN 0
				ELSE 1
			END ASC,
			date(lastDownloadedDate) ASC,
			barCount DESC,
			serialNo ASC
		LIMIT ?
	`, formatDate(runDate), formatDate(staleBefore), limit)
	if err != nil {
		return nil, fmt.Errorf("select far oldest fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarHighestBarAfterFetch returns up to limit stale far-expiry contracts
// with lastDownloadedDate strictly after afterDate, excluding excludeSerials,
// ordered by barCount DESC.
func (db *DB) SelectFarHighestBarAfterFetch(
	runDate time.Time,
	staleBefore time.Time,
	afterDate time.Time,
	excludeSerials []int64,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := db.replaceExcludeTemp(excludeSerials); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry > date(?, '+1 month')
			AND lastDownloadedDate IS NOT NULL
			AND TRIM(lastDownloadedDate) != ''
			AND date(lastDownloadedDate) > date(?)
			AND date(lastDownloadedDate) < date(?)
			AND serialNo NOT IN (SELECT serialNo FROM sel_exclude)
		ORDER BY barCount DESC, serialNo ASC
		LIMIT ?
	`,
		formatDate(runDate),
		formatDate(afterDate),
		formatDate(staleBefore),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("select far highest bar after fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarLeastBarAfterFetch returns up to limit stale far-expiry contracts
// with lastDownloadedDate strictly after afterDate, excluding excludeSerials,
// ordered by barCount ASC, lastDownloadedDate ASC.
func (db *DB) SelectFarLeastBarAfterFetch(
	runDate time.Time,
	staleBefore time.Time,
	afterDate time.Time,
	excludeSerials []int64,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := db.replaceExcludeTemp(excludeSerials); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			AND expiry > date(?, '+1 month')
			AND lastDownloadedDate IS NOT NULL
			AND TRIM(lastDownloadedDate) != ''
			AND date(lastDownloadedDate) > date(?)
			AND date(lastDownloadedDate) < date(?)
			AND serialNo NOT IN (SELECT serialNo FROM sel_exclude)
		ORDER BY barCount ASC, date(lastDownloadedDate) ASC, serialNo ASC
		LIMIT ?
	`,
		formatDate(runDate),
		formatDate(afterDate),
		formatDate(staleBefore),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("select far least bar after fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// replaceExcludeTemp rebuilds a session temp table of serial numbers to exclude.
// Using a temp table avoids SQLite host-parameter limits on large NOT IN lists.
func (db *DB) replaceExcludeTemp(excludeSerials []int64) error {
	if _, err := db.Exec(`
		CREATE TEMP TABLE IF NOT EXISTS sel_exclude (
			serialNo INTEGER PRIMARY KEY
		)
	`); err != nil {
		return fmt.Errorf("create sel_exclude: %w", err)
	}

	if _, err := db.Exec(`DELETE FROM sel_exclude`); err != nil {
		return fmt.Errorf("clear sel_exclude: %w", err)
	}

	if len(excludeSerials) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO sel_exclude (serialNo) VALUES (?)`)
	if err != nil {
		return fmt.Errorf("prepare sel_exclude insert: %w", err)
	}
	defer stmt.Close()

	for _, serial := range excludeSerials {
		if _, err := stmt.Exec(serial); err != nil {
			return fmt.Errorf("insert sel_exclude %d: %w", serial, err)
		}
	}

	return tx.Commit()
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}
