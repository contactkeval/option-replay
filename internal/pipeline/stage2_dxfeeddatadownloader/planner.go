package stage2_dxfeeddatadownloader

import (
	"fmt"
	"sort"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
)

const BatchSize = 100

func GetContractsForRun(database *db.DB) ([]db.Contract, error) {
	now := time.Now()

	switch now.Weekday() {
	case time.Saturday, time.Sunday:
		return GetWeekendContracts(database)
	default:
		return GetWeekdayContracts(database)
	}
}

func CreateBatches(contracts []db.Contract) []db.Batch {
	if len(contracts) == 0 {
		return nil
	}

	batchCount := (len(contracts) + BatchSize - 1) / BatchSize
	batches := make([]db.Batch, batchCount)

	for i := 0; i < batchCount; i++ {
		batches[i] = db.Batch{
			BatchNo: i + 1,
		}
	}

	for idx, contract := range contracts {
		batchIdx := idx % batchCount
		batches[batchIdx].Contracts = append(
			batches[batchIdx].Contracts,
			contract,
		)
	}

	return batches
}

func BuildRunPlan(database *db.DB) (int64, error) {
	nextRunNo, err := database.GetNextRunNo()
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf("Next run=%d\n", nextRunNo)

	contracts, err := GetContractsForRun(database)
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf("Selected contracts: %d\n", len(contracts))

	sort.Slice(contracts, func(i, j int) bool {
		a := contracts[i]
		b := contracts[j]

		if !a.Expiry.Equal(b.Expiry) {
			return a.Expiry.Before(b.Expiry)
		}

		if a.Underlying != b.Underlying {
			return a.Underlying < b.Underlying
		}

		if a.Type != b.Type {
			return a.Type < b.Type
		}

		return a.Strike < b.Strike
	})

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
