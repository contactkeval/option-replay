package parquetbuilder

import (
	"fmt"
	"os"
	"path/filepath"
)

const MaxEstimatedBytes = 125 * 1000 * 1000 // ~125MB

// BuildParquetForTicker processes option data files for a given ticker and writes them to Parquet format.
// It batches files based on estimated byte size, writing each batch to a separate Parquet file named
// with the ticker and the first expiry date from that batch.
//
// Parameters:
//   - ticker: The stock ticker symbol used for naming output files and parsing input file paths.
//   - files: A slice of file paths to staging files containing option data for the ticker.
//   - outputRoot: The root directory where the ticker subdirectory and Parquet files will be created.
//
// Returns:
//   - error: An error if directory creation, file reading, parsing, sorting, or Parquet writing fails.
//
// The function:
//  1. Creates an output directory at outputRoot/ticker.
//  2. Iterates through the input files, accumulating rows until the estimated byte size exceeds MaxEstimatedBytes.
//  3. When the threshold is exceeded, sorts the accumulated rows and writes them to a Parquet file.
//  4. After processing all files, writes any remaining rows to a final Parquet file.
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
	var currentEstimatedBytes int64
	var firstExpiry string

	for _, file := range files {

		info, err := os.Stat(file)
		if err != nil {
			return err
		}

		incomingBytes := info.Size()

		expiry := filepath.Base(file)
		expiry = expiry[len(ticker)+1:]
		expiry = expiry[:6]

		if currentEstimatedBytes > 0 &&
			currentEstimatedBytes+incomingBytes > MaxEstimatedBytes {

			parquetPath := filepath.Join(
				outputDir,
				fmt.Sprintf("%s_%s.parquet", ticker, firstExpiry),
			)

			SortRows(currentRows)

			if err := WriteParquet(parquetPath, currentRows); err != nil {
				return err
			}

			currentRows = nil
			currentEstimatedBytes = 0
			firstExpiry = ""
		}

		rows, err := ParseStagingFile(file)
		if err != nil {
			return err
		}

		if firstExpiry == "" {
			firstExpiry = expiry
		}

		currentRows = append(currentRows, rows...)
		currentEstimatedBytes += incomingBytes
	}

	if len(currentRows) > 0 {

		parquetPath := filepath.Join(
			outputDir,
			fmt.Sprintf("%s_%s.parquet", ticker, firstExpiry),
		)

		SortRows(currentRows)

		if err := WriteParquet(parquetPath, currentRows); err != nil {
			return err
		}
	}

	return nil
}
