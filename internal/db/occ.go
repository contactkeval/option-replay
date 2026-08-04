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

func (db *DB) StartImport(fileName string) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO occ_imports (
			file_name,
			started_at,
			status
		)
		VALUES (?, ?, ?)
	`,
		fileName,
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
			inserted = ?,
			deleted = ?,
			updated = ?,
			skipped = ?,
			errors = ?,
			status = ?
		WHERE id = ?
	`,
		stats.EndedAt.Format(time.RFC3339),
		stats.RecordsRead,
		stats.Inserted,
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
	_, err := db.Exec(`
		UPDATE occ_imports
		SET
			ended_at = ?,
			records_read = ?,
			inserted = ?,
			deleted = ?,
			updated = ?,
			skipped = ?,
			errors = ?,
			status = ?
		WHERE id = ?
	`,
		stats.EndedAt.Format(time.RFC3339),
		stats.RecordsRead,
		stats.Inserted,
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
) error {
	return db.insertOCCContractTx(tx, record, groupNo)
}

func (db *DB) HandleOCCDelete(
	tx *sql.Tx,
	record OCCRecord,
) error {
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
) error {
	if err := db.deleteContractTx(
		tx,
		record.Underlying,
		record.ExpiryDate,
		record.Strike,
		record.Type,
	); err != nil {
		return err
	}

	return db.insertOCCContractTx(tx, record, groupNo)
}

func (db *DB) insertOCCContractTx(
	tx *sql.Tx,
	record OCCRecord,
	groupNo int,
) error {
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO contracts (
			underlying,
			expiry,
			strike,
			type,
			groupNo,
			firstSeenDate
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		record.Underlying,
		record.ExpiryDate.Format("2006-01-02"),
		record.Strike,
		record.Type,
		groupNo,
		record.ActivityDate.Format("2006-01-02"),
	)

	return err
}

func (db *DB) deleteContractTx(
	tx *sql.Tx,
	underlying string,
	expiry time.Time,
	strike float64,
	contractType string,
) error {
	_, err := tx.Exec(`
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

	return err
}
