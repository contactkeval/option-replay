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

// eligibleAttemptsFilter keeps contracts that are not yet archived by attempt count.
const eligibleAttemptsFilter = `AND downloadAttempts < 3`

// FarStaleFetchDays excludes far contracts fetched within this many days of runDate.
const FarStaleFetchDays = 17

// farStaleLastFetchFilter requires lastDownloadedDate older than the bound
// (or missing). Caller binds the stale-before date once.
const farStaleLastFetchFilter = `
			AND (
				lastDownloadedDate IS NULL
				OR TRIM(lastDownloadedDate) = ''
				OR date(lastDownloadedDate) < date(?)
			)`

// CountExpiredContracts returns eligible expired contracts:
// expiry < runDate, not archived, downloadAttempts < 3.
func (db *DB) CountExpiredContracts(runDate time.Time) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry < ?
	`, formatDate(runDate)).Scan(&count)
	return count, err
}

// CountFarExpiryAvailableContracts returns far-expiry contracts available to
// fetch: expiry > runDate+1 month, not archived, downloadAttempts < 3, and
// lastDownloadedDate missing or older than FarStaleFetchDays.
func (db *DB) CountFarExpiryAvailableContracts(runDate time.Time) (int, error) {
	staleBefore := runDate.AddDate(0, 0, -FarStaleFetchDays)
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry > date(?, '+1 month')
			`+farStaleLastFetchFilter+`
	`, formatDate(runDate), formatDate(staleBefore)).Scan(&count)
	return count, err
}

// ArchiveContractsByDownloadAttempts marks every contract with
// downloadAttempts >= minAttempts as archived.
func (db *DB) ArchiveContractsByDownloadAttempts(minAttempts float64) (int64, error) {
	res, err := db.Exec(`
		UPDATE contracts
		SET archived = 1
		WHERE archived = 0
			AND downloadAttempts >= ?
	`, minAttempts)
	if err != nil {
		return 0, fmt.Errorf("archive by downloadAttempts: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// SelectExpiredUnderFetched returns expired eligible contracts with
// downloadAttempts < 1, lowest barCount first.
func (db *DB) SelectExpiredUnderFetched(
	runDate time.Time,
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
			`+eligibleAttemptsFilter+`
			AND expiry < ?
			AND downloadAttempts < 1
		ORDER BY barCount ASC, serialNo ASC
		LIMIT ?
	`, formatDate(runDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired under-fetched: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectExpiredOldestLastFetch returns expired eligible contracts that have
// already been fetched after expiry (downloadAttempts >= 1), oldest
// lastDownloadedDate first.
func (db *DB) SelectExpiredOldestLastFetch(
	runDate time.Time,
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
			`+eligibleAttemptsFilter+`
			AND expiry < ?
			AND downloadAttempts >= 1
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
	`, formatDate(runDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired oldest last fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectExpiredHighestBarAfterFetch returns expired eligible contracts with
// expiry older than beforeDate (T-1) and lastDownloadedDate strictly after
// afterDate, highest barCount first.
func (db *DB) SelectExpiredHighestBarAfterFetch(
	runDate time.Time,
	beforeDate time.Time,
	afterDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	if afterDate.IsZero() {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry < ?
			AND expiry < ?
			AND downloadAttempts >= 1
			AND lastDownloadedDate IS NOT NULL
			AND TRIM(lastDownloadedDate) != ''
			AND date(lastDownloadedDate) > date(?)
		ORDER BY barCount DESC, serialNo ASC
		LIMIT ?
	`, formatDate(runDate), formatDate(beforeDate), formatDate(afterDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select expired highest bar after fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarNeverDownloaded returns available far-expiry contracts with
// downloadAttempts = 0.
func (db *DB) SelectFarNeverDownloaded(
	runDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	staleBefore := runDate.AddDate(0, 0, -FarStaleFetchDays)

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry > date(?, '+1 month')
			AND downloadAttempts = 0
			`+farStaleLastFetchFilter+`
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
		return nil, fmt.Errorf("select far never downloaded: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarOldestLastFetch returns available far-expiry contracts with
// downloadAttempts > 0, oldest lastDownloadedDate first.
func (db *DB) SelectFarOldestLastFetch(
	runDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	staleBefore := runDate.AddDate(0, 0, -FarStaleFetchDays)

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry > date(?, '+1 month')
			AND downloadAttempts > 0
			`+farStaleLastFetchFilter+`
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
		return nil, fmt.Errorf("select far oldest last fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// SelectFarHighestBarAfterFetch returns available far-expiry contracts with
// lastDownloadedDate strictly after afterDate and downloadAttempts > 0,
// highest barCount first.
func (db *DB) SelectFarHighestBarAfterFetch(
	runDate time.Time,
	afterDate time.Time,
	limit int,
) ([]Contract, error) {
	if limit <= 0 {
		return nil, nil
	}
	if afterDate.IsZero() {
		return nil, nil
	}
	staleBefore := runDate.AddDate(0, 0, -FarStaleFetchDays)

	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			`+optionContractsFilter+`
			`+eligibleAttemptsFilter+`
			AND expiry > date(?, '+1 month')
			AND downloadAttempts > 0
			`+farStaleLastFetchFilter+`
			AND lastDownloadedDate IS NOT NULL
			AND TRIM(lastDownloadedDate) != ''
			AND date(lastDownloadedDate) > date(?)
		ORDER BY barCount DESC, serialNo ASC
		LIMIT ?
	`, formatDate(runDate), formatDate(staleBefore), formatDate(afterDate), limit)
	if err != nil {
		return nil, fmt.Errorf("select far highest bar after fetch: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}
