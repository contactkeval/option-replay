package parquetbuilder

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func DiscoverTickerFiles(root string) (map[string][]string, error) {

	result := make(map[string][]string)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

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

		parts := strings.Split(base, "_")

		ticker := parts[0]

		result[ticker] = append(result[ticker], path)

		return nil
	})

	if err != nil {
		return nil, err
	}

	for _, files := range result {
		sort.Strings(files)
	}

	return result, nil
}
