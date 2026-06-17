package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type RowGroup struct {
	Rows []config.ParquetRow
}

func BuildPhysicalRowGroups(
	rows []config.ParquetRow,
) []RowGroup {

	if len(rows) == 0 {
		return nil
	}

	strikeBlocks := SplitStrikeBlocks(
		rows,
	)

	result := make(
		[]RowGroup,
		0,
	)

	current := make(
		[]config.ParquetRow,
		0,
		config.MaxRowsPerRowGroup,
	)

	currentRows := 0

	for _, block := range strikeBlocks {

		blockRows := len(block)

		// first block always fits

		if currentRows == 0 {

			current = append(
				current,
				block...,
			)

			currentRows += blockRows

			continue
		}

		// block still fits

		if currentRows+blockRows <= config.MaxRowsPerRowGroup {

			current = append(
				current,
				block...,
			)

			currentRows += blockRows

			continue
		}

		// ----------------------------------
		// strike would overflow rowgroup
		// ----------------------------------

		result = append(
			result,
			RowGroup{
				Rows: current,
			},
		)

		current = make(
			[]config.ParquetRow,
			0,
			config.MaxRowsPerRowGroup,
		)

		current = append(
			current,
			block...,
		)

		currentRows = blockRows
	}

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
	rows []config.ParquetRow,
) [][]config.ParquetRow {

	if len(rows) == 0 {
		return nil
	}

	result := make(
		[][]config.ParquetRow,
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
