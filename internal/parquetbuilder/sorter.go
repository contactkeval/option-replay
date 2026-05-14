package parquetbuilder

import "sort"

func SortRows(rows []OptionRow) {

    sort.Slice(rows, func(i, j int) bool {

        a := rows[i]
        b := rows[j]

        if a.Expiry != b.Expiry {
            return a.Expiry < b.Expiry
        }

        if a.Strike != b.Strike {
            return a.Strike < b.Strike
        }

        if a.OptionType != b.OptionType {
            return a.OptionType < b.OptionType
        }

        return a.WindowStart < b.WindowStart
    })
}