package db

import (
	"fmt"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func (db *DB) InsertCandleStaging(
	serialNo int64,
	candle config.Candle,
	runNo int64,
	batchNo int,
) error {
	_, err := db.Exec(`
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
	`,
		serialNo,
		candle.Time,
		float64(candle.Open),
		float64(candle.High),
		float64(candle.Low),
		float64(candle.Close),
		float64(candle.Volume),
		runNo,
		batchNo,
	)

	if err != nil {
		return fmt.Errorf("insert candle staging: %w", err)
	}

	return nil
}
