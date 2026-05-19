package stage3_sort_dedupe

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	return nil
}

func SortAndDedupe(path string) error {

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var rows []string

	for scanner.Scan() {
		rows = append(rows, scanner.Text())
	}

	sort.Strings(rows)

	tempPath := path + ".tmp"

	out, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	writer := bufio.NewWriterSize(out, 64*1024)

	var last string

	for _, row := range rows {

		if row == last {
			fmt.Printf("DUPLICATE %s\n", row)
			continue
		}

		_, err := writer.WriteString(row + "\n")
		if err != nil {
			return err
		}

		last = row
	}

	writer.Flush()
	out.Close()

	os.Remove(path)

	return os.Rename(tempPath, path)
}
