package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

type RowGroupAccumulator struct {
	pending []model.ParquetRow
}

func (a *RowGroupAccumulator) PendingRows() []model.ParquetRow {
	return a.pending
}

func (a *RowGroupAccumulator) PendingCount() int {
	return len(a.pending)
}

func (a *RowGroupAccumulator) Reset() {
	a.pending = nil
}

// AppendExpiry:
// - never splits strike
// - expiry MAY span RGs
// - returns completed RGs
func (a *RowGroupAccumulator) AppendExpiry(
	expiryRows []model.ParquetRow,
) [][]model.ParquetRow {

	var flushed [][]model.ParquetRow

	if len(expiryRows) == 0 {
		return flushed
	}

	start := 0

	for start < len(expiryRows) {

		remainingCapacity :=
			constants.RowGroupTargetRows - len(a.pending)

		if remainingCapacity <= 0 {

			flushed = append(
				flushed,
				a.pending,
			)

			a.Reset()

			remainingCapacity =
				constants.RowGroupTargetRows
		}

		end := start

		for end < len(expiryRows) &&
			(end-start) < remainingCapacity {

			end++
		}

		// NEVER split strike
		if end < len(expiryRows) {

			lastStrike :=
				expiryRows[end-1].Strike

			for end > start &&
				expiryRows[end].Strike == lastStrike {

				end--
			}

			// pathological case
			if end == start {

				end = start

				strike := expiryRows[start].Strike

				for end < len(expiryRows) &&
					expiryRows[end].Strike == strike {

					end++
				}
			}
		}

		a.pending = append(
			a.pending,
			expiryRows[start:end]...,
		)

		start = end

		if len(a.pending) >=
			constants.RowGroupTargetRows {

			flushed = append(
				flushed,
				a.pending,
			)

			a.Reset()
		}
	}

	return flushed
}
