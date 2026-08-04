package db

import (
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

func (db *DB) UpdateBatchEndTime(
	runNo int64,
	batchNo int,
	endTime string,
	candleCount int64,
) error {
	_, err := db.Exec(`
		UPDATE batches
		SET
			endTime = ?,
			candleCount = ?
		WHERE
			runNo = ?
			AND batchNo = ?
	`,
		endTime,
		candleCount,
		runNo,
		batchNo,
	)

	if err != nil {
		return fmt.Errorf("update batch end time: %w", err)
	}

	return nil
}
