package stage2_finalize

import (
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func ProcessRawFile(
	path string,
) (map[string][]config.TransientRow, error) {

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
		map[string][]config.TransientRow,
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

		row := config.TransientRow{
			Ticker: parsed.Underlying,

			ParquetRow: config.ParquetRow{
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

func ParseTicker(raw string) (config.ParsedTicker, error) {

	// Example:
	// O:SPY230327P00390000
	if !strings.HasPrefix(raw, "O:") {
		return config.ParsedTicker{}, errors.New("invalid option ticker")
	}

	s := raw[2:]
	if len(s) < 15 {
		return config.ParsedTicker{}, errors.New("invalid ticker length")
	}

	expiryStart := len(s) - 15
	underlying := s[:expiryStart]
	expiryStr := s[expiryStart : expiryStart+6]
	optionTypeChar := s[expiryStart+6]
	strikeStr := s[expiryStart+7:]
	strike, err := strconv.ParseUint(strikeStr, 10, 32)
	if err != nil {
		return config.ParsedTicker{}, fmt.Errorf("parse strike price: %w", err)
	}
	expiry, err := time.Parse("060102", expiryStr)
	if err != nil {
		return config.ParsedTicker{}, fmt.Errorf("parse expiry date: %w", err)
	}

	return config.ParsedTicker{
		Underlying: strings.ToUpper(underlying),
		ExpiryDate: expiry,
		OptionType: optionTypeChar == 'C',
		Strike:     uint32(strike),
	}, nil
}

func ConvertRows(rows []config.CsvRow) []config.ParquetRow {
	result := make([]config.ParquetRow, 0, len(rows))

	for _, r := range rows {

		strike, _ := strconv.ParseUint(
			r.Strike,
			10,
			32,
		)

		result = append(result, config.ParquetRow{
			Strike:      uint32(strike), // strike is already in fixed-point format in the CSV, so we can directly convert it to uint32
			OptionType:  util.ParseOptionType(r.OptionType),
			WindowStart: util.NanosecondsToSeconds(r.WindowStart),

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
