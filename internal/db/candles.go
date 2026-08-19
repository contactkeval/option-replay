package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

const candleInsertSQL = `
	INSERT OR IGNORE INTO candle_staging (
		serialNo,
		candleTime,
		open,
		high,
		low,
		close,
		volume,
		runNo,
		batchNo
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`

type CandleStagingRow struct {
	SerialNo int64
	Candle   config.Candle
	RunNo    int64
	BatchNo  int
}

func (db *DB) InsertCandleStaging(
	serialNo int64,
	candle config.Candle,
	runNo int64,
	batchNo int,
) (bool, error) {
	n, perSerial, err := db.InsertCandleStagingBatch([]CandleStagingRow{{
		SerialNo: serialNo,
		Candle:   candle,
		RunNo:    runNo,
		BatchNo:  batchNo,
	}})
	if err != nil {
		return false, err
	}
	return n == 1 && perSerial[serialNo] == 1, nil
}

func (db *DB) InsertCandleStagingBatch(
	rows []CandleStagingRow,
) (int64, map[int64]int64, error) {
	inserted := make(map[int64]int64)
	if len(rows) == 0 {
		return 0, inserted, nil
	}

	var total int64
	err := retryBusy(func() error {
		total = 0
		inserted = make(map[int64]int64, len(rows))

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.Prepare(candleInsertSQL)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, row := range rows {
			res, err := stmt.Exec(
				row.SerialNo,
				row.Candle.Time,
				float64(row.Candle.Open),
				float64(row.Candle.High),
				float64(row.Candle.Low),
				float64(row.Candle.Close),
				float64(row.Candle.Volume),
				row.RunNo,
				row.BatchNo,
			)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n == 1 {
				inserted[row.SerialNo]++
				total++
			}
		}

		return tx.Commit()
	})
	if err != nil {
		return 0, nil, fmt.Errorf("insert candle staging: %w", err)
	}

	return total, inserted, nil
}

func retryBusy(fn func() error) error {
	var last error
	for attempt := 0; attempt < 12; attempt++ {
		last = fn()
		if last == nil || !isSQLiteBusy(last) {
			return last
		}
		time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
	}
	return last
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "database is locked")
}
