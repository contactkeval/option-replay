package stage2_dxfeeddatadownloader

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
)

// ResolveDownloadTarget picks the run and batch numbers to download.
// runNo 0 means the latest run. batchNo 0 means every batch of that run.
func ResolveDownloadTarget(
	database *db.DB,
	runNo int64,
	batchNo int,
) (int64, []int, error) {
	if runNo < 0 {
		return 0, nil, fmt.Errorf("run number must be >= 0")
	}
	if batchNo < 0 {
		return 0, nil, fmt.Errorf("batch number must be >= 0")
	}

	if runNo == 0 {
		latest, err := database.GetLatestRunNo()
		if err != nil {
			return 0, nil, err
		}
		runNo = latest
	}

	batchCount, err := database.GetRunBatchCount(runNo)
	if err != nil {
		return 0, nil, err
	}
	if batchCount <= 0 {
		return 0, nil, fmt.Errorf("run %d has no batches", runNo)
	}

	if batchNo == 0 {
		batches := make([]int, batchCount)
		for i := 0; i < batchCount; i++ {
			batches[i] = i + 1
		}
		return runNo, batches, nil
	}

	if batchNo > batchCount {
		return 0, nil, fmt.Errorf(
			"batch %d not in run %d (batchCount=%d)",
			batchNo,
			runNo,
			batchCount,
		)
	}

	return runNo, []int{batchNo}, nil
}

func DownloadRun(
	metadataDB *db.DB,
	runNo int64,
	batchNos []int,
) error {
	waves := splitIntoWaves(batchNos, DownloadWaves)
	for i, wave := range waves {
		logger.Infof(
			"Download wave %d/%d: batches=%v (disconnect after wave; cooldown=%s between waves)",
			i+1,
			len(waves),
			wave,
			WaveCooldown,
		)
		if err := downloadWithPool(metadataDB, runNo, wave); err != nil {
			return err
		}
		// Pool exit closes all worker DXLink sessions. Pause before the next
		// wave so the next connect cycle starts fresh.
		if i+1 < len(waves) {
			logger.Infof(
				"Wave %d/%d complete; pausing %s before reconnecting for next %d batches",
				i+1,
				len(waves),
				WaveCooldown,
				len(waves[i+1]),
			)
			time.Sleep(WaveCooldown)
		}
	}

	logger.Infof("dxfeed data download complete for run %d, batches=%v", runNo, batchNos)
	return nil
}

// splitIntoWaves divides batchNos into n nearly equal contiguous waves.
func splitIntoWaves(batchNos []int, n int) [][]int {
	if len(batchNos) == 0 {
		return nil
	}
	if n < 1 {
		n = 1
	}
	if n > len(batchNos) {
		n = len(batchNos)
	}

	waves := make([][]int, 0, n)
	base := len(batchNos) / n
	rem := len(batchNos) % n
	idx := 0
	for i := 0; i < n; i++ {
		size := base
		if i < rem {
			size++
		}
		waves = append(waves, batchNos[idx:idx+size])
		idx += size
	}
	return waves
}
