package stage3_sort_dedupe

import "sort"

func SortRows(rows []Stage3Row) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i]
		b := rows[j]

		if a.Strike != b.Strike {
			return a.Strike < b.Strike
		}

		if a.OptionType != b.OptionType {
			if !a.OptionType && b.OptionType {
				return true
			}
			return false
		}

		return a.WindowStart < b.WindowStart
	})
}
