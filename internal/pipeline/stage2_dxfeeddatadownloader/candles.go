package stage2_dxfeeddatadownloader

import (
	"fmt"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
)

func DownloadRun(
	metadataDB *db.DB,
	runNo int64,
) error {
	batchCount, err := metadataDB.GetRunBatchCount(runNo)
	if err != nil {
		return fmt.Errorf("failed to get batch count: %w", err)
	}

	for batchNo := 1; batchNo <= batchCount; batchNo++ {
		err := DownloadBatch(metadataDB, runNo, batchNo)
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
