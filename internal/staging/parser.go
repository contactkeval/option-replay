package staging

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"os"
	"time"
)

func ProcessFile(path string, cache *WriterCache) error {

	fmt.Printf("%s PROCESSING %s\n", time.Now().Format("2006-01-02 15:04:05"), path)

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
			fmt.Printf(
				"WRITE ERROR ticker=%s err=%v\n",
				ticker,
				err,
			)

			continue
		}
	}

	return nil
}
