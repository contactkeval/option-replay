package stage2_dxfeeddatadownloader

import (
	"fmt"
	"sort"
	"time"
)

const (
	BatchSize = 100
)

type Contract struct {
	SerialNo   int64
	Underlying string
	Expiry     time.Time
	Type       string
	Strike     float64
	GroupNo    int
}

type Batch struct {
	BatchNo   int
	Contracts []Contract
}

func (m *MetadataDB) GetContractsForRun() (
	[]Contract,
	error,
) {

	now := time.Now()

	switch now.Weekday() {

	case time.Saturday,
		time.Sunday:

		return m.GetWeekendContracts()

	default:

		return m.GetWeekdayContracts()
	}
}

func CreateBatches(
	contracts []Contract,
) []Batch {

	if len(contracts) == 0 {
		return nil
	}

	batchCount :=
		(len(contracts) + BatchSize - 1) /
			BatchSize

	batches :=
		make(
			[]Batch,
			batchCount,
		)

	for i := 0; i < batchCount; i++ {

		batches[i] = Batch{
			BatchNo: i + 1,
		}
	}

	for idx, contract := range contracts {

		batchIdx :=
			idx % batchCount

		batches[batchIdx].Contracts =
			append(
				batches[batchIdx].Contracts,
				contract,
			)
	}

	return batches
}
func (m *MetadataDB) CreateRun(
	contractCount int,
	batchCount int,
) (int64, error) {

	res, err := m.db.Exec(`
		INSERT INTO runs (
			groupNo,
			runDateTime,
			contractCount,
			batchCount
		)
		VALUES (?, ?, ?, ?)
	`,
		-1,
		time.Now().Format(
			time.RFC3339,
		),
		contractCount,
		batchCount,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (m *MetadataDB) PersistBatchPlan(
	runNo int64,
	batches []Batch,
) error {

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, batch := range batches {

		_, err := tx.Exec(`
			INSERT INTO batches (
				runNo,
				batchNo,
				contractCount
			)
			VALUES (?, ?, ?)
		`,
			runNo,
			batch.BatchNo,
			len(batch.Contracts),
		)
		if err != nil {
			return err
		}

		for idx, contract := range batch.Contracts {

			_, err :=
				tx.Exec(`
				INSERT INTO batch_contracts (
					runNo,
					batchNo,
					serialNo,
					listNo
				)
				VALUES (?, ?, ?, ?)
			`,
					runNo,
					batch.BatchNo,
					contract.SerialNo,
					idx+1,
				)

			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func BuildRunPlan(
	db *MetadataDB,
) (int64, error) {

	nextRunNo,
		err := db.GetNextRunNo()
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf(
		"Next run=%d\n",
		nextRunNo,
	)

	contracts, err := db.GetContractsForRun()
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf(
		"Selected contracts: %d\n",
		len(contracts),
	)

	sort.Slice(
		contracts,
		func(i, j int) bool {

			a := contracts[i]
			b := contracts[j]

			if !a.Expiry.Equal(b.Expiry) {
				return a.Expiry.Before(
					b.Expiry,
				)
			}

			if a.Underlying != b.Underlying {
				return a.Underlying <
					b.Underlying
			}

			if a.Type != b.Type {
				return a.Type <
					b.Type
			}

			return a.Strike <
				b.Strike
		},
	)

	batches :=
		CreateBatches(
			contracts,
		)

	fmt.Printf(
		"Created batches: %d\n",
		len(batches),
	)

	runNo,
		err := db.CreateRun(
		len(contracts),
		len(batches),
	)
	if err != nil {
		return nextRunNo, err
	}

	fmt.Printf(
		"Created run: %d\n",
		runNo,
	)

	err =
		db.PersistBatchPlan(
			runNo,
			batches,
		)
	if err != nil {
		return nextRunNo, err
	}

	fmt.Println(
		"Batch plan persisted",
	)

	return nextRunNo, nil
}

func (m *MetadataDB) GetNextRunNo() (
	int64,
	error,
) {

	var nextRunNo int64

	err := m.db.QueryRow(`
		SELECT
			COALESCE(MAX(runNo), 0) + 1
		FROM runs
	`).Scan(&nextRunNo)

	return nextRunNo, err
}
