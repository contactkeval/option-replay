package parquetbuilder

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func DiscoverTickerFiles(
	stagingRoot string,
	parquetRoot string,
) (map[string][]string, error) {

	result := make(map[string][]string)

	err := filepath.Walk(
		stagingRoot,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".csv") {
				return nil
			}

			base := filepath.Base(path)

			// Example:
			// AAPL_260515.csv

			parts := strings.Split(base, "_")

			if len(parts) < 2 {
				return nil
			}

			ticker := parts[0]

			// Skip ticker if already processed
			if TickerAlreadyProcessed(parquetRoot, ticker) {
				return nil
			}

			result[ticker] = append(result[ticker], path)

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	// Ensure expiries processed in ascending order
	for _, files := range result {
		sort.Strings(files)
	}

	return result, nil
}

func TickerAlreadyProcessed(
	parquetRoot string,
	ticker string,
) bool {

	tickerDir := filepath.Join(parquetRoot, ticker)

	entries, err := os.ReadDir(tickerDir)

	if err != nil {
		return false
	}

	// Success marker file exists
	for _, e := range entries {

		if e.Name() == "_SUCCESS" {
			return true
		}
	}

	return false
}
