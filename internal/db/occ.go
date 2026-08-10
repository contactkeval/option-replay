package db

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	ImportStatusRunning   = "running"
	ImportStatusCompleted = "completed"
	ImportStatusFailed    = "failed"
)

func (db *DB) StartImport(
	fileName string,
	fileDate time.Time,
	downloadType string,
) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO occ_imports (
			file_name,
			file_date,
			download_type,
			started_at,
			status
		)
		VALUES (?, ?, ?, ?, ?)
	`,
		fileName,
		fileDate.Format("2006-01-02"),
		downloadType,
		time.Now().Format(time.RFC3339),
		ImportStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("start import: %w", err)
	}

	return res.LastInsertId()
}

func (db *DB) CompleteImport(
	importID int64,
	stats ImportStatistics,
) error {
	_, err := db.Exec(`
		UPDATE occ_imports
		SET
			ended_at = ?,
			records_read = ?,
			processed = ?,
			ignored = ?,
			inserted = ?,
			existing = ?,
			deleted = ?,
			updated = ?,
			skipped = ?,
			errors = ?,
			status = ?
		WHERE id = ?
	`,
		stats.EndedAt.Format(time.RFC3339),
		stats.RecordsRead,
		stats.Processed,
		stats.Ignored,
		stats.Inserted,
		stats.Existing,
		stats.Deleted,
		stats.Updated,
		stats.Skipped,
		stats.Errors,
		ImportStatusCompleted,
		importID,
	)
	if err != nil {
		return fmt.Errorf("complete import: %w", err)
	}

	return nil
}

func (db *DB) FailImport(
	importID int64,
	stats ImportStatistics,
) error {
	endedAt := stats.EndedAt
	if endedAt.IsZero() {
		endedAt = time.Now()
	}

	_, err := db.Exec(`
		UPDATE occ_imports
		SET
			ended_at = ?,
			records_read = ?,
			processed = ?,
			ignored = ?,
			inserted = ?,
			existing = ?,
			deleted = ?,
			updated = ?,
			skipped = ?,
			errors = ?,
			status = ?
		WHERE id = ?
	`,
		endedAt.Format(time.RFC3339),
		stats.RecordsRead,
		stats.Processed,
		stats.Ignored,
		stats.Inserted,
		stats.Existing,
		stats.Deleted,
		stats.Updated,
		stats.Skipped,
		stats.Errors,
		ImportStatusFailed,
		importID,
	)
	if err != nil {
		return fmt.Errorf("fail import: %w", err)
	}

	return nil
}

func (db *DB) HandleOCCAdd(
	tx *sql.Tx,
	record OCCRecord,
	groupNo int,
) (bool, error) {
	return db.insertOCCContractTx(tx, record, groupNo)
}

func (db *DB) HandleOCCDelete(
	tx *sql.Tx,
	record OCCRecord,
) (bool, error) {
	return db.deleteContractTx(
		tx,
		record.Underlying,
		record.ExpiryDate,
		record.Strike,
		record.Type,
	)
}

func (db *DB) HandleOCCModify(
	tx *sql.Tx,
	record OCCRecord,
	groupNo int,
) (bool, error) {
	deleted, err := db.deleteContractTx(
		tx,
		record.Underlying,
		record.ExpiryDate,
		record.Strike,
		record.Type,
	)
	if err != nil {
		return false, err
	}

	inserted, err := db.insertOCCContractTx(tx, record, groupNo)
	if err != nil {
		return false, err
	}

	return deleted || inserted, nil
}

func (db *DB) insertOCCContractTx(
	tx *sql.Tx,
	record OCCRecord,
	groupNo int,
) (bool, error) {
	seen := record.ActivityDate.Format("2006-01-02")
	res, err := tx.Exec(`
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
	`,
		record.Underlying,
		record.ExpiryDate.Format("2006-01-02"),
		record.Strike,
		record.Type,
		groupNo,
		seen,
		seen,
	)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return n == 1, nil
}

func (db *DB) deleteContractTx(
	tx *sql.Tx,
	underlying string,
	expiry time.Time,
	strike float64,
	contractType string,
) (bool, error) {
	res, err := tx.Exec(`
		DELETE FROM contracts
		WHERE
			underlying = ?
			AND expiry = ?
			AND strike = ?
			AND type = ?
	`,
		underlying,
		expiry.Format("2006-01-02"),
		strike,
		contractType,
	)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}
