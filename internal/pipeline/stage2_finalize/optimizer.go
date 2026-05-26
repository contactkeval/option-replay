package stage2_finalize

import (
	"strconv"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func ConvertRows(rows []model.CsvRow) []model.ParquetRow {

	result := make([]model.ParquetRow, 0, len(rows))

	for _, r := range rows {

		strike, _ := strconv.ParseUint(
			r.Strike,
			10,
			32,
		)

		result = append(result, model.ParquetRow{
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
