package stage2_dxfeeddatadownloader

import (
	"fmt"

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
	if err := downloadWithPool(metadataDB, runNo, batchNos); err != nil {
		return err
	}

	logger.Infof("dxfeed data download complete")
	return nil
}
