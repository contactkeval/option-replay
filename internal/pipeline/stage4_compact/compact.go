package stage4_compact

import (
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/parquet-go/parquet-go"
)

func RenamePending(
	path string,
) (string, error) {

	pending := path + ".pending"

	err := os.Rename(
		path,
		pending,
	)

	if err != nil {
		return "", fmt.Errorf(
			"rename pending file: %w",
			err,
		)
	}

	return pending, nil
}

func SelectCompactCandidates(
	rows []CompactCandidate,
	targetRowGroups int,
) [][]CompactCandidate {

	grouped := make(
		map[string][]CompactCandidate,
	)

	for _, row := range rows {

		grouped[row.Ticker] = append(
			grouped[row.Ticker],
			row,
		)
	}

	result := make(
		[][]CompactCandidate,
		0,
	)

	for _, tickerRows := range grouped {

		current := make(
			[]CompactCandidate,
			0,
		)

		currentGroups := 0

		for _, row := range tickerRows {

			current = append(
				current,
				row,
			)

			currentGroups += row.RowGroupCount

			if currentGroups >= targetRowGroups {

				result = append(
					result,
					current,
				)

				current = nil
				currentGroups = 0
			}
		}
	}

	return result
}

func CompactParquetFiles(
	outputPath string,
	inputPaths []string,
) error {

	out, err := os.Create(
		outputPath,
	)

	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	writer := parquet.NewGenericWriter[config.ParquetRow](
		out,
	)

	defer func() {
		_ = writer.Close()
		_ = out.Close()
	}()

	for _, path := range inputPaths {

		err := func() error {

			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer f.Close()

			stat, err := f.Stat()
			if err != nil {
				return fmt.Errorf("get input file stats: %w", err)
			}

			pf, err := parquet.OpenFile(f, stat.Size())
			if err != nil {
				return fmt.Errorf("open parquet file: %w", err)
			}

			for _, rg := range pf.RowGroups() {
				if _, err := writer.WriteRowGroup(rg); err != nil {
					return fmt.Errorf("write row group: %w", err)
				}
			}

			return nil
		}()

		if err != nil {
			return fmt.Errorf("process input file %s: %w", path, err)
		}
	}

	return nil
}
