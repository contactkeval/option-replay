package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
)

func main() {

	parquetRoot := `G:\data\parquet`

	err := filepath.Walk(
		parquetRoot,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if filepath.Ext(path) != ".parquet" {
				return nil
			}

			if err := PrintRowGroupCount(path); err != nil {
				fmt.Printf(
					"ERROR %s: %v\n",
					path,
					err,
				)
			}

			return nil
		},
	)

	if err != nil {
		panic(err)
	}
}

func PrintRowGroupCount(
	path string,
) error {

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf(
			"open parquet file %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf(
			"stat parquet file %s: %w",
			path,
			err,
		)
	}

	reader, err := parquet.OpenFile(
		file,
		stat.Size(),
	)
	if err != nil {
		return fmt.Errorf(
			"open parquet reader %s: %w",
			path,
			err,
		)
	}

	fmt.Printf(
		"%s -> row_groups=%d rows=%d\n",
		path,
		len(reader.RowGroups()),
		reader.NumRows(),
	)

	return nil
}
