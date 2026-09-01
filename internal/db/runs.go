package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Batch struct {
	BatchNo   int
	Contracts []Contract
}

func (db *DB) GetNextRunNo() (int64, error) {
	var nextRunNo int64

	err := db.QueryRow(`
		SELECT COALESCE(MAX(runNo), 0) + 1
		FROM runs
	`).Scan(&nextRunNo)

	return nextRunNo, err
}

func (db *DB) GetLatestRunNo() (int64, error) {
	var runNo sql.NullInt64

	err := db.QueryRow(`
		SELECT MAX(runNo)
		FROM runs
	`).Scan(&runNo)
	if err != nil {
		return 0, fmt.Errorf("query latest run: %w", err)
	}
	if !runNo.Valid || runNo.Int64 == 0 {
		return 0, fmt.Errorf("no runs found")
	}

	return runNo.Int64, nil
}

func (db *DB) CreateRun(
	contractCount int,
	batchCount int,
) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO runs (
			groupNo,
			runDateTime,
			contractCount,
			batchCount
		)
		VALUES (?, ?, ?, ?)
	`,
		-1,
		time.Now().Format(time.RFC3339),
		contractCount,
		batchCount,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (db *DB) PersistBatchPlan(
	runNo int64,
	batches []Batch,
) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, batch := range batches {
		_, err := tx.Exec(`
			INSERT INTO batches (
				runNo,
				batchNo,
				contractCount
			)
			VALUES (?, ?, ?)
		`,
			runNo,
			batch.BatchNo,
			len(batch.Contracts),
		)
		if err != nil {
			return err
		}

		for idx, contract := range batch.Contracts {
			_, err := tx.Exec(`
				INSERT INTO batch_contracts (
					runNo,
					batchNo,
					serialNo,
					listNo
				)
				VALUES (?, ?, ?, ?)
			`,
				runNo,
				batch.BatchNo,
				contract.SerialNo,
				idx+1,
			)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (db *DB) UpdateBatchStartTime(
	runNo int64,
	batchNo int,
	startTime string,
) error {
	_, err := db.Exec(`
		UPDATE batches
		SET startTime = ?
		WHERE
			runNo = ?
			AND batchNo = ?
	`,
		startTime,
		runNo,
		batchNo,
	)

	if err != nil {
		return fmt.Errorf("update batch start time: %w", err)
	}

	return nil
}

// UpdateBatchDownloadStats writes endTime plus total/new bar counts for a batch.
// barCount is candles received from dxFeed; newBarCount is newly inserted rows.
func (db *DB) UpdateBatchDownloadStats(
	runNo int64,
	batchNo int,
	endTime string,
	barCount int64,
	newBarCount int64,
) error {
	_, err := db.Exec(`
		UPDATE batches
		SET
			endTime = ?,
			barCount = ?,
			newBarCount = ?
		WHERE
			runNo = ?
			AND batchNo = ?
	`,
		endTime,
		barCount,
		newBarCount,
		runNo,
		batchNo,
	)

	if err != nil {
		return fmt.Errorf("update batch download stats: %w", err)
	}

	return nil
}

// UpdateBatchContractDownloadStats sets total/new bars for one batch_contracts row.
func (db *DB) UpdateBatchContractDownloadStats(
	runNo int64,
	batchNo int,
	serialNo int64,
	barCount int64,
	newBarCount int64,
) error {
	_, err := db.Exec(`
		UPDATE batch_contracts
		SET
			barCount = ?,
			newBarCount = ?
		WHERE
			runNo = ?
			AND batchNo = ?
			AND serialNo = ?
	`,
		barCount,
		newBarCount,
		runNo,
		batchNo,
		serialNo,
	)
	if err != nil {
		return fmt.Errorf(
			"update batch_contracts download stats run=%d batch=%d serial=%d: %w",
			runNo,
			batchNo,
			serialNo,
			err,
		)
	}
	return nil
}

// RefreshRunDownloadStats sets runs.barCount/newBarCount from the sum of batches.
func (db *DB) RefreshRunDownloadStats(runNo int64) error {
	_, err := db.Exec(`
		UPDATE runs
		SET
			barCount = (
				SELECT COALESCE(SUM(barCount), 0)
				FROM batches
				WHERE runNo = ?
			),
			newBarCount = (
				SELECT COALESCE(SUM(newBarCount), 0)
				FROM batches
				WHERE runNo = ?
			)
		WHERE runNo = ?
	`, runNo, runNo, runNo)
	if err != nil {
		return fmt.Errorf("refresh run download stats: %w", err)
	}
	return nil
}
