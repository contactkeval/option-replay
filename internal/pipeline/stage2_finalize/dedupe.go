package stage2_finalize

import "github.com/contactkeval/option-replay/internal/pipeline/model"

func DedupeRows(rows []model.ParquetRow) ([]model.ParquetRow, int) {

	if len(rows) == 0 {
		return rows, 0
	}

	result := make([]model.ParquetRow, 0, len(rows))

	result = append(result, rows[0])

	duplicates := 0

	for i := 1; i < len(rows); i++ {

		prev := rows[i-1]
		curr := rows[i]

		if prev.Strike == curr.Strike &&
			prev.OptionType == curr.OptionType &&
			prev.WindowStart == curr.WindowStart {

			duplicates++
			continue
		}

		result = append(result, curr)
	}

	return result, duplicates
}
