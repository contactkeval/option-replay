package stage2_finalize

import stage3 "github.com/contactkeval/option-replay/internal/pipeline/stage3_sort_dedupe"

func DedupeRows(rows []stage3.Stage3Row) ([]stage3.Stage3Row, int) {

	if len(rows) == 0 {
		return rows, 0
	}

	result := make([]stage3.Stage3Row, 0, len(rows))

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
