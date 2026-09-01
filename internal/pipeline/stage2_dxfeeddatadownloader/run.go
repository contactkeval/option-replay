package stage2_dxfeeddatadownloader

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Serialize dials: concurrent DXLink handshakes from the same IP often
	// time out while siblings succeed.
	dxFeedDialMu.Lock()
	defer dxFeedDialMu.Unlock()

	dialCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := connectAndHandshake(dialCtx)
	if err == nil {
		// Keepalive/channel lifetime should follow the worker ctx, not dial timeout.
		client.StartKeepalive(ctx)
		return client, nil
	}
	// Only retry when OAuth can still mint a new quote token. A revoked grant
	// or env-token fallback cannot be healed by another handshake attempt.
	if !isUnauthorizedErr(err) || !canRefreshTastyOAuth() || oauthGrantIsDead() {
		return nil, err
	}
	logger.Warnf("DXFeed AUTH expired; refreshing Tastyworks quote token")
	invalidateDxLinkAuth()

	dialCtx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()
	client, err = connectAndHandshake(dialCtx2)
	if err != nil {
		return nil, err
	}
	client.StartKeepalive(ctx)
	return client, nil
}

var dxFeedDialMu sync.Mutex

func openDXFeedWithRetry(ctx context.Context, attempts int) (*DXFeedClient, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, err := openDXFeed(ctx)
		if err == nil {
			return client, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		logger.Warnf("DXFeed connect attempt %d/%d failed: %v", i, attempts, err)
		time.Sleep(time.Duration(i) * 3 * time.Second)
	}
	return nil, lastErr
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
	receivedBySerial map[int64]int64,
) (newCount int64, receivedCount int64, alive bool, pending []string, err error) {
	if err := client.SubscribeCandles(chunk, fromTime); err != nil {
		return 0, 0, false, nil, fmt.Errorf("failed to subscribe to candles: %w", err)
	}
	logger.Debugf("Batch %d chunk %d subscribed, sample=%s", batchNo, chunkNo, chunk[0])

	const flushSize = 250

	buf := make([]db.CandleStagingRow, 0, flushSize)
	var inserted int64
	var received int64

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

	alive, pending, err = client.ReadLoop(ctx, chunk, func(candle config.Candle) error {
		serialNo, ok := symbolToSerial[candle.EventSymbol]
		if !ok {
			return nil
		}

		received++
		receivedBySerial[serialNo]++

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
		return inserted, received, false, pending, err
	}

	if alive {
		_ = client.UnsubscribeCandles(chunk)
	}

	return inserted, received, alive, pending, nil
}
