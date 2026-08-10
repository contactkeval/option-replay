package stage2_dxfeeddatadownloader

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
)

// CreateBatches assigns already-sorted contracts to download groups.
// Group count is (len/MaxGroupSize)+1. Each contract at sorted index i goes to
// group (i % groupCount) + 1 so groups stay balanced and under MaxGroupSize.
func CreateBatches(contracts []db.Contract) []db.Batch {
	if len(contracts) == 0 {
		return nil
	}

	groupCount := GroupCount(len(contracts))
	batches := make([]db.Batch, groupCount)

	for i := 0; i < groupCount; i++ {
		batches[i] = db.Batch{
			BatchNo: i + 1,
		}
	}

	for idx, contract := range contracts {
		batchIdx := idx % groupCount
		batches[batchIdx].Contracts = append(
			batches[batchIdx].Contracts,
			contract,
		)
	}

	return batches
}

// BuildRunPlan selects contracts for runDate, sorts them for grouping, creates
// batches, and persists the run + batch_contracts plan in the metadata DB.
// Returns the next run number that was reserved before creation (for logging);
// the persisted run id is printed and used by DownloadRun via GetNextRunNo flow.
func BuildRunPlan(database *db.DB, runDate time.Time) (int64, error) {
	nextRunNo, err := database.GetNextRunNo()
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf("Next run=%d\n", nextRunNo)

	contracts, err := GetContractsForRun(database, runDate)
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf("Selected contracts: %d\n", len(contracts))

	SortContractsForGrouping(contracts)

	batches := CreateBatches(contracts)

	fmt.Printf("Created batches: %d\n", len(batches))

	runNo, err := database.CreateRun(len(contracts), len(batches))
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf("Created run: %d\n", runNo)

	if err := database.PersistBatchPlan(runNo, batches); err != nil {
		return nextRunNo, err
	}

	fmt.Println("Batch plan persisted")

	return nextRunNo, nil
}
