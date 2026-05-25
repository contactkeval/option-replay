package stage1_ingest

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func ProcessRawFile(path string, cache *WriterCache) error {

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

	_, err = csvReader.Read()
	if err != nil {
		return err
	}

	for {

		record, err := csvReader.Read()
		if err != nil {
			break
		}

		parsed, err := ParseTicker(record[0])
		if err != nil {
			continue
		}

		if !config.IsAllowedUnderlying(parsed.Underlying) {
			continue
		}

		optionType := "P"
		if parsed.OptionType {
			optionType = "C"
		}

		line := fmt.Sprintf(
			"%08d,%s,%s,%s,%s,%s,%s,%s,%s",
			parsed.Strike,
			optionType,
			record[6],
			record[2],
			record[4],
			record[5],
			record[3],
			record[1],
			record[7],
		)

		if err := cache.Write(parsed, line); err != nil {
			return err
		}
	}

	return nil
}
