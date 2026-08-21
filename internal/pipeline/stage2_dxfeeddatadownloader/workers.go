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
	var firstErr error
	fail := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	var wg sync.WaitGroup
	for workerID := 1; workerID <= DownloadWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runDownloadWorker(
				ctx,
				id,
				jobCh,
				metadataDB,
				fromTime,
				progress,
				&mu,
				fail,
			)
		}(workerID)
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
		case jobCh <- job:
		}
	}
	close(jobCh)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	fetchDate := time.Now()
	endTime := fetchDate.Format(time.RFC3339)
	for _, batchNo := range batchNos {
		p := progress[batchNo]
		_ = metadataDB.UpdateBatchEndTime(runNo, batchNo, endTime, p.candles)
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
	fail func(error),
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

	for job := range jobs {
		if ctx.Err() != nil {
			return
		}

		if client == nil {
			logger.Infof(
				"worker %d opening session (SETUP/AUTH/CHANNEL_REQUEST)",
				workerID,
			)
			c, err := openDXFeed(ctx)
			if err != nil {
				fail(fmt.Errorf("worker %d: %w", workerID, err))
				return
			}
			client = c
		} else {
			logger.Debugf("worker %d reusing session", workerID)
		}

		logger.Infof(
			"worker %d batch %d chunk %d/%d (%d symbols)",
			workerID,
			job.batchNo,
			job.chunkNo,
			job.chunkCount,
			len(job.symbols),
		)

		local := make(map[int64]int64)
		n, alive, err := readChunk(
			ctx,
			client,
			metadataDB,
			job.runNo,
			job.batchNo,
			job.chunkNo,
			job.symbols,
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

		if err != nil {
			fail(fmt.Errorf("worker %d batch %d chunk %d: %w", workerID, job.batchNo, job.chunkNo, err))
			closeClient()
			return
		}
		if !alive {
			logger.Warnf("worker %d session closed after chunk; will reconnect", workerID)
			closeClient()
		}
	}
}
