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
	for batchNo := 1; batchNo <= 11; batchNo++ {
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
