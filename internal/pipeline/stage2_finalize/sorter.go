package stage2_finalize

import (
	"sort"

	stage3 "github.com/contactkeval/option-replay/internal/pipeline/stage3_sort_dedupe"
)

func SortRows(rows []stage3.Stage3Row) {

	sort.Slice(rows, func(i, j int) bool {

		a := rows[i]
		b := rows[j]

		if a.Strike != b.Strike {
			return a.Strike < b.Strike
		}

		if a.OptionType != b.OptionType {
			// false (Put) before true (Call)
			return !a.OptionType && b.OptionType
		}

		return a.WindowStart < b.WindowStart
	})
}
