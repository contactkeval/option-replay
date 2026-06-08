package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func BuildRowGroups(
	rows []model.ParquetRow,
	targetRows int,
	maxTrailingRows int,
) [][]model.ParquetRow {

	accumulator := RowGroupAccumulator{}

	rowGroups := accumulator.AppendExpiry(
		rows,
	)

	// trailing rows rule
	if accumulator.PendingCount() >
		maxTrailingRows {

		return nil
	}

	return rowGroups
}
