package stage2_finalize

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/contactkeval/option-replay/internal/pipeline/transientdb"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func ProcessRawFile(
	path string,
) (map[string][]transientdb.TransientRow, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open raw file %s: %w", path, err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader for %s: %w", path, err)
	}
	defer gzReader.Close()

	csvReader := csv.NewReader(gzReader)

	csvReader.FieldsPerRecord = -1

	_, err = csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header from %s: %w", path, err)
	}

	result := make(
		map[string][]transientdb.TransientRow,
	)

	for {

		record, err := csvReader.Read()
		if err != nil {
			break
		}

		parsed, err := ParseTicker(record[0])
		if err != nil {
			continue
		}

		if !config.IsAllowedUnderlying(
			parsed.Underlying,
		) {
			continue
		}

		expiryString := parsed.ExpiryDate.Format(
			"2006-01-02",
		)

		row := transientdb.TransientRow{
			Ticker: parsed.Underlying,

			ParquetRow: model.ParquetRow{
				Strike: uint32(parsed.Strike),

				OptionType: parsed.OptionType,

				WindowStart: util.NanosecondsToSecondsMust(
					record[6],
				),

				Open: util.PriceStringToUint32(
					record[2],
				),

				High: util.PriceStringToUint32(
					record[4],
				),

				Low: util.PriceStringToUint32(
					record[5],
				),

				Close: util.PriceStringToUint32(
					record[3],
				),

				Volume: util.Uint32Must(
					record[1],
				),

				Transactions: util.Uint32Must(
					record[7],
				),
			},
		}

		result[expiryString] = append(
			result[expiryString],
			row,
		)
	}

	return result, nil
}
