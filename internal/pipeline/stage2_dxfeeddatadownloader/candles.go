package stage2_dxfeeddatadownloader

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func (m *MetadataDB) SaveCandles(
	runNo int64,
	batchNo int,
	candles []config.Candle,
) (int64, error) {

	tx, err := m.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO candles (
			symbol,
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
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}

	defer stmt.Close()

	var inserted int64

	for _, c := range candles {

		res, err := stmt.Exec(
			c.EventSymbol,
			c.Time,
			float64(c.Open),
			float64(c.High),
			float64(c.Low),
			float64(c.Close),
			float64(c.Volume),
			runNo,
			batchNo,
		)

		if err != nil {
			return 0, fmt.Errorf("failed to execute statement: %w", err)
		}

		rows, _ := res.RowsAffected()

		inserted += rows
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return inserted, nil
}

func (m *MetadataDB) CompleteBatch(
	runNo int64,
	batchNo int,
	candleCount int64,
) error {

	_, err := m.db.Exec(`
		UPDATE batches
		SET
			endTime = ?,
			candleCount = ?
		WHERE
			runNo = ?
			AND batchNo = ?
	`,
		time.Now().Format(time.RFC3339),
		candleCount,
		runNo,
		batchNo,
	)

	if err != nil {
		err = fmt.Errorf("failed to update batch end time: %w", err)
	}

	return err
}

func DownloadRun(
	metadataDB *MetadataDB,
	runNo int64,
) error {

	for batchNo := 1; batchNo <= 11; batchNo++ {

		// fmt.Printf(
		// 	"\n===== BATCH %d =====\n",
		// 	batchNo,
		// )

		err := DownloadBatch(
			metadataDB,
			runNo,
			batchNo,
		)

		if err != nil {

			fmt.Printf(
				"Batch %d failed: %v\n",
				batchNo,
				err,
			)

			continue
		}
	}

	logger.Infof("dxfeed data download complete")
	return nil
}
