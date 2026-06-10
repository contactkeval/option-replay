package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

type RowGroup struct {
	Rows []model.ParquetRow
}

func BuildPhysicalRowGroups(
	rows []model.ParquetRow,
) []RowGroup {

	if len(rows) == 0 {
		return nil
	}

	strikeBlocks := SplitStrikeBlocks(rows)

	target := constants.TargetRowsPerRowGroup
	maxTrailing := constants.MaxTrailingRows

	result := make([]RowGroup, 0)

	current := make(
		[]model.ParquetRow,
		0,
		target+maxTrailing,
	)

	remainingRows := len(rows)

	for _, block := range strikeBlocks {

		blockSize := len(block)

		// ---------------------------------
		// If current rowgroup is empty
		// just add block.
		// ---------------------------------

		if len(current) == 0 {

			current = append(current, block...)
			remainingRows -= blockSize

			continue
		}

		// ---------------------------------
		// Always add strike block first.
		// Strike-level overshoot is allowed.
		// ---------------------------------

		current = append(current, block...)
		remainingRows -= blockSize

		// ---------------------------------
		// Still below target.
		// keep accumulating.
		// ---------------------------------

		if len(current) < target {
			continue
		}

		// ---------------------------------
		// We crossed target.
		// ---------------------------------

		// CASE 1:
		// tiny trailing rows remain.
		// absorb them into current rowgroup.

		if remainingRows > 0 &&
			remainingRows <= maxTrailing {

			continue
		}

		// ---------------------------------
		// finalize current rowgroup
		// ---------------------------------

		result = append(
			result,
			RowGroup{
				Rows: current,
			},
		)

		// ---------------------------------
		// start new rowgroup
		// ---------------------------------

		current = make(
			[]model.ParquetRow,
			0,
			target+maxTrailing,
		)
	}

	// ---------------------------------
	// final partial rowgroup
	// ---------------------------------

	if len(current) > 0 {

		result = append(
			result,
			RowGroup{
				Rows: current,
			},
		)
	}

	return result
}

func SplitStrikeBlocks(
	rows []model.ParquetRow,
) [][]model.ParquetRow {

	if len(rows) == 0 {
		return nil
	}

	result := make(
		[][]model.ParquetRow,
		0,
	)

	start := 0

	for i := 1; i < len(rows); i++ {

		prev := rows[i-1]
		curr := rows[i]

		// strike boundary
		if prev.Strike != curr.Strike ||
			prev.OptionType != curr.OptionType {

			result = append(
				result,
				rows[start:i],
			)

			start = i
		}
	}

	// final block
	result = append(
		result,
		rows[start:],
	)

	return result
}
