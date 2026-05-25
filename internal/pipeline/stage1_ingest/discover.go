package stage1_ingest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func DiscoverRawFiles(root string) ([]string, error) {

	var result []string

	today := time.Now().Format("2006-01-02")

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".csv.gz") {
			return nil
		}

		base := filepath.Base(path)

		if strings.Contains(base, today) {
			return nil
		}

		result = append(result, path)

		return nil
	})

	if err != nil {
		return nil, err
	}
	sort.Strings(result)

	return result, nil
}
