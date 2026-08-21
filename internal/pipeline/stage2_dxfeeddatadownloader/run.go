package stage2_dxfeeddatadownloader

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config, dbPath string, runNo int64, batchNo int) error {
	if dbPath == "" {
		dbPath = filepath.Join(
			cfg.MetadataRoot,
			"metadata.db",
		)
	}

	metadataDB, err := db.Open(db.Options{
		Path:    dbPath,
		Schemas: db.SchemaMetadata,
	})
	if err != nil {
		return fmt.Errorf("failed to open metadata DB: %w", err)
	}
	defer metadataDB.Close()

	runNo, batchNos, err := ResolveDownloadTarget(metadataDB, runNo, batchNo)
	if err != nil {
		return fmt.Errorf("failed to resolve download target: %w", err)
	}

	from := time.Now().AddDate(-2, 0, 0)
	logger.Infof(
		"Downloading run=%d batches=%v from=%s",
		runNo,
		batchNos,
		from.Format("2006-01-02"),
	)

	return DownloadRun(metadataDB, runNo, batchNos)
}

func openDXFeed(ctx context.Context) (*DXFeedClient, error) {
	client, err := connectAndHandshake(ctx)
	if err != nil && isUnauthorizedErr(err) && canRefreshTastyOAuth() {
		logger.Warnf("DXFeed AUTH expired; refreshing Tastyworks tokens")
		invalidateDxLinkAuth()
		return connectAndHandshake(ctx)
	}
	return client, err
}

func connectAndHandshake(ctx context.Context) (*DXFeedClient, error) {
	client, err := Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to DXFeed: %w", err)
	}

	if err := client.Handshake(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}

	client.StartKeepalive(ctx)

	if err := client.OpenFeedChannel(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to open feed channel: %w", err)
	}

	if err := client.WaitForChannel(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to wait for channel: %w", err)
	}

	return client, nil
}

func readChunk(
	ctx context.Context,
	client *DXFeedClient,
	metadataDB *db.DB,
	runNo int64,
	batchNo int,
	chunkNo int,
	chunk []string,
	symbolToSerial map[string]int64,
	fromTime int64,
	insertedBySerial map[int64]int64,
) (int64, bool, error) {
	if err := client.SubscribeCandles(chunk, fromTime); err != nil {
		return 0, false, fmt.Errorf("failed to subscribe to candles: %w", err)
	}
	logger.Debugf("Batch %d chunk %d subscribed, sample=%s", batchNo, chunkNo, chunk[0])

	const flushSize = 250

	buf := make([]db.CandleStagingRow, 0, flushSize)
	var inserted int64

	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		n, perSerial, err := metadataDB.InsertCandleStagingBatch(buf)
		if err != nil {
			return err
		}
		inserted += n
		for serial, count := range perSerial {
			insertedBySerial[serial] += count
		}
		buf = buf[:0]
		return nil
	}

	alive, err := client.ReadLoop(ctx, chunk, func(candle config.Candle) error {
		serialNo, ok := symbolToSerial[candle.EventSymbol]
		if !ok {
			return nil
		}

		buf = append(buf, db.CandleStagingRow{
			SerialNo: serialNo,
			Candle:   candle,
			RunNo:    runNo,
			BatchNo:  batchNo,
		})
		if len(buf) >= flushSize {
			return flush()
		}
		return nil
	})
	if flushErr := flush(); flushErr != nil && err == nil {
		err = flushErr
	}
	if err != nil {
		return inserted, false, err
	}

	if alive {
		_ = client.UnsubscribeCandles(chunk)
	}

	return inserted, alive, nil
}
