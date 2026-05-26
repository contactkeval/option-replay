package stage2_finalize

import (
	"sort"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func SortRows(rows []model.ParquetRow) {

	sort.Slice(rows, func(i, j int) bool {

		a := rows[i]
		b := rows[j]

		if a.Strike != b.Strike {
			return a.Strike < b.Strike
		}

		if a.OptionType != b.OptionType {
			// Call before Put
			return a.OptionType && !b.OptionType
		}

		return a.WindowStart < b.WindowStart
	})
}
