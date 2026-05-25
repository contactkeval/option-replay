package stage2_finalize

import (
	"strconv"

	stage3 "github.com/contactkeval/option-replay/internal/pipeline/stage3_parquet"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func ConvertRows(rows []CsvRow) []stage3.ParquetRow {

	result := make([]stage3.ParquetRow, 0, len(rows))

	for _, r := range rows {

		strike, _ := strconv.ParseUint(
			r.Strike,
			10,
			32,
		)

		result = append(result, stage3.ParquetRow{
			Strike:      uint32(strike),
			OptionType:  r.OptionType == "C",
			WindowStart: uint32(r.WindowStart / 1_000_000_000),

			Open:  util.PriceToUint32(r.Open),
			High:  util.PriceToUint32(r.High),
			Low:   util.PriceToUint32(r.Low),
			Close: util.PriceToUint32(r.Close),

			Volume:       r.Volume,
			Transactions: r.Transactions,
		})
	}

	return result
}
