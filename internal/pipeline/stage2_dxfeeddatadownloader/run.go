package stage2_dxfeeddatadownloader

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {
	metadataDBPath := filepath.Join(
		cfg.MetadataRoot,
		"metadata.db",
	)

	metadataDB, err := db.Open(db.Options{
		Path:    metadataDBPath,
		Schemas: db.SchemaMetadata,
	})
	if err != nil {
		return fmt.Errorf("failed to open metadata DB: %w", err)
	}
	defer metadataDB.Close()

	runNo, err := BuildRunPlan(metadataDB)
	if err != nil {
		return fmt.Errorf("failed to build run plan: %w", err)
	}

	return DownloadRun(metadataDB, runNo)
}

func DownloadBatch(
	metadataDB *db.DB,
	runNo int64,
	batchNo int,
) error {
	contracts, err := metadataDB.GetBatchContracts(runNo, batchNo)
	if err != nil {
		return fmt.Errorf("failed to get batch contracts: %w", err)
	}

	if len(contracts) == 0 {
		return nil
	}

	symbols := make([]string, 0, len(contracts))
	symbolToSerial := make(map[string]int64, len(contracts))

	for _, contract := range contracts {
		symbol := ToDXFeedSymbol(contract)
		if symbol == "" {
			continue
		}

		symbols = append(symbols, symbol)
		symbolToSerial[symbol] = contract.SerialNo
	}

	ctx := context.Background()

	client, err := Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to DXFeed: %w", err)
	}
	defer client.Close()

	if err := client.Setup(); err != nil {
		return fmt.Errorf("failed to setup DXFeed client: %w", err)
	}

	if err := client.Auth(); err != nil {
		return fmt.Errorf("failed to authenticate with DXFeed: %w", err)
	}

	if err := client.WaitForAuth(ctx); err != nil {
		return fmt.Errorf("failed to wait for authentication: %w", err)
	}

	if err := client.OpenFeedChannel(); err != nil {
		return fmt.Errorf("failed to open feed channel: %w", err)
	}

	if err := client.WaitForChannel(ctx); err != nil {
		return fmt.Errorf("failed to wait for channel: %w", err)
	}

	fromTime := time.Now().Add(-66 * 24 * time.Hour).UnixMilli()

	if err := client.SubscribeCandles(symbols, fromTime); err != nil {
		return fmt.Errorf("failed to subscribe to candles: %w", err)
	}

	var candleCount int64

	startTime := time.Now().Format(time.RFC3339)

	if err := metadataDB.UpdateBatchStartTime(runNo, batchNo, startTime); err != nil {
		return err
	}

	err = client.ReadLoop(ctx, func(candle config.Candle) error {
		serialNo, ok := symbolToSerial[candle.EventSymbol]
		if !ok {
			return nil
		}

		if err := metadataDB.InsertCandleStaging(serialNo, candle, runNo, batchNo); err != nil {
			return err
		}

		candleCount++
		return nil
	})

	endTime := time.Now().Format(time.RFC3339)

	_ = metadataDB.UpdateBatchEndTime(runNo, batchNo, endTime, candleCount)

	if err != nil {
		fmt.Printf(
			"Batch %d ended with error: %v\n",
			batchNo,
			err,
		)
		return fmt.Errorf("failed to update batch end time: %w", err)
	}

	fmt.Printf(
		"Batch %d, contracts %d, downloaded candles=%d\n",
		batchNo,
		len(contracts),
		candleCount,
	)

	return nil
}
