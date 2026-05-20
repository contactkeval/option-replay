package stage3_sort_dedupe

import (
	"github.com/contactkeval/option-replay/internal/logger"
)

func DedupeRows(rows []Stage3Row) ([]Stage3Row, uint32) {
	if len(rows) == 0 {
		return rows, 0
	}

	result := make([]Stage3Row, 0, len(rows))
	result = append(result, rows[0])

	var duplicates uint32
	last := rows[0]

	for i := 1; i < len(rows); i++ {
		current := rows[i]
		if current.Strike == last.Strike &&
			current.OptionType == last.OptionType &&
			current.WindowStart == last.WindowStart {
			duplicates++

			logger.Debugf(
				"duplicate row removed strike=%d optionType=%v windowStart=%d",
				current.Strike,
				current.OptionType,
				current.WindowStart,
			)
			continue
		}

		result = append(result, current)
		last = current
	}

	return result, duplicates
}
