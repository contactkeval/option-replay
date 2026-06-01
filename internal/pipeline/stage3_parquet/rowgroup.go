package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

type RowGroupAccumulator struct {
	Rows []model.ParquetRow
}

func (a *RowGroupAccumulator) Count() int {
	return len(a.Rows)
}

func (a *RowGroupAccumulator) Empty() bool {
	return len(a.Rows) == 0
}

func (a *RowGroupAccumulator) Reset() {
	a.Rows = nil
}

// AppendExpiry:
// - never splits strike
// - targets RG size
// - expiry MAY span RGs
func (a *RowGroupAccumulator) AppendExpiry(
	expiryRows []model.ParquetRow,
) [][]model.ParquetRow {

	var flushed [][]model.ParquetRow

	if len(expiryRows) == 0 {
		return flushed
	}

	start := 0

	for start < len(expiryRows) {

		target := constants.RowGroupTargetRows

		remainingCapacity := target - len(a.Rows)

		// if current RG already full enough,
		// flush BEFORE appending more
		if remainingCapacity <= 0 {

			flushed = append(
				flushed,
				a.Rows,
			)

			a.Reset()

			remainingCapacity = target
		}

		end := start

		for end < len(expiryRows) &&
			(end-start) < remainingCapacity {

			end++
		}

		// NEVER split strike
		if end < len(expiryRows) {

			lastStrike := expiryRows[end-1].Strike

			for end > start &&
				expiryRows[end].Strike == lastStrike {

				end--
			}

			// pathological case:
			// single strike > RG size
			if end == start {

				end = start

				currentStrike := expiryRows[start].Strike

				for end < len(expiryRows) &&
					expiryRows[end].Strike == currentStrike {

					end++
				}
			}
		}

		a.Rows = append(
			a.Rows,
			expiryRows[start:end]...,
		)

		start = end

		// flush once threshold crossed
		if len(a.Rows) >= target {

			flushed = append(
				flushed,
				a.Rows,
			)

			a.Reset()
		}
	}

	return flushed
}

// FlushRemaining
// used at parquet-file boundary
func (a *RowGroupAccumulator) FlushRemaining() []model.ParquetRow {

	if len(a.Rows) == 0 {
		return nil
	}

	rows := a.Rows

	a.Reset()

	return rows
}
