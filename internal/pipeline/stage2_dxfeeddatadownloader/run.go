package stage2_dxfeeddatadownloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	metadataDBPath := filepath.Join(
		cfg.MetadataRoot,
		"metadata.db",
	)

	db, err := OpenMetadataDB(
		metadataDBPath,
	)
	if err != nil {
		return err
	}
	defer db.Close()

	return BuildRunPlan(
		db,
	)
}

func (m *MetadataDB) GetBatchContracts(
	runNo int64,
	batchNo int,
) ([]Contract, error) {

	rows, err := m.db.Query(`
		SELECT
			c.serialNo,
			c.underlying,
			c.expiry,
			c.type,
			c.strike,
			c.groupNo
		FROM batch_contracts bc
		JOIN contracts c
			ON c.serialNo = bc.serialNo
		WHERE
			bc.runNo = ?
			AND bc.batchNo = ?
		ORDER BY bc.listNo
	`,
		runNo,
		batchNo,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []Contract

	for rows.Next() {

		var c Contract
		var expiry string

		err := rows.Scan(
			&c.SerialNo,
			&c.Underlying,
			&expiry,
			&c.Type,
			&c.Strike,
			&c.GroupNo,
		)
		if err != nil {
			return nil, err
		}

		c.Expiry, err = time.Parse(
			"2006-01-02",
			expiry,
		)
		if err != nil {
			return nil, err
		}

		contracts = append(
			contracts,
			c,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return contracts, nil
}

func DownloadBatch(
	metadataDB *MetadataDB,
	runNo int64,
	batchNo int,
) error {

	contracts, err :=
		metadataDB.GetBatchContracts(
			runNo,
			batchNo,
		)
	if err != nil {
		return err
	}

	fmt.Printf(
		"Batch %d contracts=%d\n",
		batchNo,
		len(contracts),
	)

	if len(contracts) == 0 {
		return nil
	}

	symbols :=
		make(
			[]string,
			0,
			len(contracts),
		)

	symbolToSerial :=
		make(
			map[string]int64,
			len(contracts),
		)

	for _, contract := range contracts {

		symbol :=
			ToDXFeedSymbol(
				contract,
			)

		if symbol == "" {
			continue
		}

		symbols =
			append(
				symbols,
				symbol,
			)

		symbolToSerial[symbol] =
			contract.SerialNo
	}

	ctx := context.Background()

	client, err := Connect(
		ctx,
		"wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime",
	)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Setup(); err != nil {
		return err
	}

	if err := client.Auth(
		os.Getenv("dxFeed_Token"),
	); err != nil {
		return err
	}

	if err := client.WaitForAuth(
		ctx,
	); err != nil {
		return err
	}

	if err := client.OpenFeedChannel(); err != nil {
		return err
	}

	if err := client.WaitForChannel(
		ctx,
		1,
	); err != nil {
		return err
	}

	fromTime :=
		time.Now().
			Add(
				-7 * 24 * time.Hour,
			).
			UnixMilli()

	if err := client.SubscribeCandles(
		symbols,
		fromTime,
	); err != nil {
		return err
	}

	var candleCount int64

	startTime :=
		time.Now().
			Format(time.RFC3339)

	_, err =
		metadataDB.db.Exec(`
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
		return err
	}

	err = client.ReadLoop(
		ctx,
		func(
			candle config.Candle,
		) error {

			serialNo,
				ok :=
				symbolToSerial[candle.EventSymbol]

			if !ok {
				return nil
			}

			_, err :=
				metadataDB.db.Exec(`
				INSERT OR IGNORE
				INTO candle_staging (
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
				VALUES (
					?,
					?,
					?,
					?,
					?,
					?,
					?,
					?,
					?
				)
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
				return err
			}

			candleCount++

			return nil
		},
	)

	endTime :=
		time.Now().
			Format(time.RFC3339)

	_, _ =
		metadataDB.db.Exec(`
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

		fmt.Printf(
			"Batch %d ended with error: %v\n",
			batchNo,
			err,
		)

		return err
	}

	fmt.Printf(
		"Batch %d downloaded candles=%d\n",
		batchNo,
		candleCount,
	)

	return nil
}
