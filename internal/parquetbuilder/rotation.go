package parquetbuilder

import (
	"fmt"
	"os"
	"path/filepath"
)

const MaxRowsPerFile = 13_000_000

func BuildParquetForTicker(
	ticker string,
	files []string,
	outputRoot string,
) error {

	outputDir := filepath.Join(outputRoot, ticker)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	var currentRows []OptionRow
	var firstExpiry string

	flush := func() error {

		if len(currentRows) == 0 {
			return nil
		}

		parquetPath := filepath.Join(
			outputDir,
			fmt.Sprintf(
				"%s_%s.parquet",
				ticker,
				firstExpiry,
			),
		)

		SortRows(currentRows)

		if err := WriteParquet(parquetPath, currentRows); err != nil {
			return err
		}

		currentRows = nil
		firstExpiry = ""

		return nil
	}

	for _, file := range files {

		rows, err := ParseStagingFile(file)
		if err != nil {
			return err
		}

		// Rotate BEFORE appending new rows
		if len(currentRows) > 0 &&
			len(currentRows)+len(rows) > MaxRowsPerFile {

			if err := flush(); err != nil {
				return err
			}
		}

		expiry := filepath.Base(file)
		expiry = expiry[len(ticker)+1:]
		expiry = expiry[:6]

		if firstExpiry == "" {
			firstExpiry = expiry
		}

		currentRows = append(currentRows, rows...)
	}

	// Final flush
	if err := flush(); err != nil {
		return err
	}

	return nil
}
