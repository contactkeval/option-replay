package stage2_dxfeeddatadownloader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
)

type chunkJob struct {
	runNo          int64
	batchNo        int
	chunkNo        int
	chunkCount     int
	symbols        []string
	symbolToSerial map[string]int64
}

type batchProgress struct {
	contracts []db.Contract
	inserted  map[int64]int64
	candles   int64
}

func downloadWithPool(
	metadataDB *db.DB,
	runNo int64,
	batchNos []int,
) error {
	jobs, progress, err := buildDownloadJobs(metadataDB, runNo, batchNos)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}

	fromTime := time.Now().AddDate(-2, 0, 0).UnixMilli()
	logger.Infof(
		"Download pool: %d workers, %d chunks of up to %d symbols",
		DownloadWorkers,
		len(jobs),
		MaxCandleSubscribe,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobCh := make(chan chunkJob)
	var mu sync.Mutex

	var wg sync.WaitGroup
	for workerID := 1; workerID <= DownloadWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Stagger startup so workers do not all dial DXLink at once.
			delay := time.Duration(id-1) * 5 * time.Second
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			runDownloadWorker(
				ctx,
				id,
				jobCh,
				metadataDB,
				fromTime,
				progress,
				&mu,
			)
		}(workerID)
	}

	for i, job := range jobs {
		select {
		case <-ctx.Done():
			logger.Warnf(
				"download cancelled while queueing jobs; queued=%d remaining=%d: %v",
				i,
				len(jobs)-i,
				ctx.Err(),
			)
			goto waitWorkers
		case jobCh <- job:
		}
	}
waitWorkers:
	close(jobCh)
	wg.Wait()

	// Always persist endTime/candleCount for work already done. Previously this
	// ran only after a fully clean pool exit, so auth/chunk failures left
	// startTime set and endTime/candleCount NULL.
	if err := finalizeBatchProgress(metadataDB, runNo, batchNos, progress); err != nil {
		return err
	}

	return nil
}

func finalizeBatchProgress(
	metadataDB *db.DB,
	runNo int64,
	batchNos []int,
	progress map[int]*batchProgress,
) error {
	fetchDate := time.Now()
	endTime := fetchDate.Format(time.RFC3339)

	for _, batchNo := range batchNos {
		p := progress[batchNo]
		if p == nil {
			continue
		}

		if err := metadataDB.UpdateBatchEndTime(runNo, batchNo, endTime, p.candles); err != nil {
			return err
		}
		for _, contract := range p.contracts {
			if err := metadataDB.RecordContractFetch(
				contract.SerialNo,
				int(p.inserted[contract.SerialNo]),
				fetchDate,
			); err != nil {
				return fmt.Errorf(
					"record fetch for serial %d: %w",
					contract.SerialNo,
					err,
				)
			}
		}
		logger.Infof(
			"Batch %d, contracts %d, downloaded candles=%d",
			batchNo,
			len(p.contracts),
			p.candles,
		)
	}

	return nil
}

func buildDownloadJobs(
	metadataDB *db.DB,
	runNo int64,
	batchNos []int,
) ([]chunkJob, map[int]*batchProgress, error) {
	jobs := make([]chunkJob, 0)
	progress := make(map[int]*batchProgress, len(batchNos))
	startTime := time.Now().Format(time.RFC3339)

	for _, batchNo := range batchNos {
		contracts, err := metadataDB.GetBatchContracts(runNo, batchNo)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get batch contracts: %w", err)
		}
		if len(contracts) == 0 {
			progress[batchNo] = &batchProgress{
				inserted: map[int64]int64{},
			}
			continue
		}

		if err := metadataDB.UpdateBatchStartTime(runNo, batchNo, startTime); err != nil {
			return nil, nil, err
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

		chunks := chunkSymbols(symbols, MaxCandleSubscribe)
		logger.Infof(
			"Batch %d: %d contracts in %d chunks of up to %d",
			batchNo,
			len(symbols),
			len(chunks),
			MaxCandleSubscribe,
		)

		progress[batchNo] = &batchProgress{
			contracts: contracts,
			inserted:  make(map[int64]int64, len(contracts)),
		}

		for i, chunk := range chunks {
			jobs = append(jobs, chunkJob{
				runNo:          runNo,
				batchNo:        batchNo,
				chunkNo:        i + 1,
				chunkCount:     len(chunks),
				symbols:        chunk,
				symbolToSerial: symbolToSerial,
			})
		}
	}

	return jobs, progress, nil
}

func runDownloadWorker(
	ctx context.Context,
	workerID int,
	jobs <-chan chunkJob,
	metadataDB *db.DB,
	fromTime int64,
	progress map[int]*batchProgress,
	mu *sync.Mutex,
) {
	var client *DXFeedClient
	closeClient := func() {
		if client == nil {
			return
		}
		_ = client.Close()
		client = nil
	}
	defer closeClient()

	ensureClient := func() error {
		if client != nil {
			logger.Debugf("worker %d reusing session", workerID)
			return nil
		}
		// Keep trying to connect without failing the pool. One worker's dial
		// timeouts must not cancel siblings that are already downloading.
		round := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			round++
			logger.Infof(
				"worker %d opening session (SETUP/AUTH/CHANNEL_REQUEST) round=%d",
				workerID,
				round,
			)
			c, err := openDXFeedWithRetry(ctx, ConnectAttempts)
			if err == nil {
				client = c
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warnf(
				"worker %d could not open DXLink session (round %d): %v; retrying without cancelling pool",
				workerID,
				round,
				err,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(15 * time.Second):
			}
		}
	}

	for job := range jobs {
		if ctx.Err() != nil {
			return
		}

		symbols := append([]string(nil), job.symbols...)
		var skipped []string
		var lastErr error
		succeeded := false
		for attempt := 1; attempt <= ChunkAttempts; attempt++ {
			if ctx.Err() != nil {
				return
			}
			if len(symbols) == 0 {
				succeeded = true
				break
			}
			if err := ensureClient(); err != nil {
				// Only parent cancel stops the worker here.
				return
			}

			logger.Infof(
				"worker %d batch %d chunk %d/%d (%d symbols) attempt=%d/%d",
				workerID,
				job.batchNo,
				job.chunkNo,
				job.chunkCount,
				len(symbols),
				attempt,
				ChunkAttempts,
			)

			local := make(map[int64]int64)
			n, alive, pending, err := readChunk(
				ctx,
				client,
				metadataDB,
				job.runNo,
				job.batchNo,
				job.chunkNo,
				symbols,
				job.symbolToSerial,
				fromTime,
				local,
			)

			mu.Lock()
			p := progress[job.batchNo]
			p.candles += n
			for serial, count := range local {
				p.inserted[serial] += count
			}
			mu.Unlock()

			if err == nil && alive {
				succeeded = true
				break
			}

			lastErr = err
			if err == nil {
				lastErr = fmt.Errorf("session closed before chunk snapshot completed")
			}
			logger.Warnf(
				"worker %d batch %d chunk %d attempt %d failed: %v",
				workerID,
				job.batchNo,
				job.chunkNo,
				attempt,
				lastErr,
			)

			// Symbols that never sent SNAPSHOT_END are treated as problematic:
			// drop them and retry the remainder so one sticky symbol cannot
			// fail the whole chunk (candles for completed symbols are already saved).
			if len(pending) > 0 {
				next := excludeSymbols(symbols, pending)
				logger.Warnf(
					"worker %d batch %d chunk %d: excluding %d problematic symbol(s) from retry (sample=%v); remaining=%d",
					workerID,
					job.batchNo,
					job.chunkNo,
					len(pending),
					sampleSymbols(pending, 5),
					len(next),
				)
				skipped = append(skipped, pending...)
				symbols = next
				if len(symbols) == 0 {
					succeeded = true
					break
				}
			}

			closeClient()
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		if succeeded {
			if len(skipped) > 0 {
				logger.Warnf(
					"worker %d batch %d chunk %d accepted with %d symbol(s) skipped: %v",
					workerID,
					job.batchNo,
					job.chunkNo,
					len(skipped),
					sampleSymbols(skipped, 8),
				)
			}
			continue
		}

		if ctx.Err() != nil {
			return
		}
		// Soft-fail: keep the pool alive so sibling workers can finish.
		logger.Errorf(
			"worker %d batch %d chunk %d: giving up after %d attempts: %v",
			workerID,
			job.batchNo,
			job.chunkNo,
			ChunkAttempts,
			lastErr,
		)
	}
}

func sampleSymbols(symbols []string, n int) []string {
	if n <= 0 || len(symbols) == 0 {
		return nil
	}
	if len(symbols) <= n {
		return append([]string(nil), symbols...)
	}
	return append([]string(nil), symbols[:n]...)
}
