package staging

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"os"
)

func ProcessFile(path string, cache *WriterCache) error {

	fmt.Printf("PROCESSING %s\n", path)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	csvReader := csv.NewReader(gzReader)

	csvReader.FieldsPerRecord = -1

	// Skip header
	_, err = csvReader.Read()
	if err != nil {
		return err
	}

	for {

		record, err := csvReader.Read()

		if err != nil {
			break
		}

		ticker := record[0]

		parsed, err := ParseTicker(ticker)
		if err != nil {
			continue
		}

		line := fmt.Sprintf(
			"%s,%s,%s,%s,%s,%s,%s,%s,%s",
			parsed.Strike,
			parsed.OptionType,
			record[6], // window_start
			record[2], // open
			record[4], // high
			record[5], // low
			record[3], // close
			record[1], // volume
			record[7], // transactions
		)

		if err := cache.Write(parsed, line); err != nil {
			return err
		}
	}

	return nil
}
