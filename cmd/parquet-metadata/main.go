package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// go run .\cmd\parquet-metadata\main.go G:\data\parquet\SPY\SPY_20240513.parquet
// go run .\cmd\parquet-metadata\main.go G:\data\parquet\SPY\SPY_20240513.parquet allcols
// go run .\cmd\parquet-metadata\main.go G:\data\parquet\SPY\SPY_20240513.parquet nodetails
// go run .\cmd\parquet-metadata\main.go G:\data\parquet\SPY\
// go run .\cmd\parquet-metadata\main.go G:\data\parquet\SPY\ nodetails
func main() {

	if len(os.Args) < 2 {
		fmt.Println(
			"Usage: go run . <file-or-folder> [nodetails] [allcols]",
		)
		return
	}

	path := os.Args[1]

	var (
		noDetails bool
		allCols   bool
	)

	for _, arg := range os.Args[2:] {

		switch strings.ToLower(arg) {

		case "nodetails":
			noDetails = true

		case "allcols":
			allCols = true
		}
	}

	files, err := DiscoverParquetFiles(
		path,
	)

	if err != nil {
		fmt.Printf(
			"error discovering parquet files: %v\n",
			err,
		)
		return
	}

	if len(files) == 0 {
		fmt.Println(
			"No parquet files found",
		)
		return
	}

	for _, file := range files {

		err := PrintRowGroupInfo(
			file,
			noDetails,
			allCols,
		)

		if err != nil {

			fmt.Printf(
				"error processing %s: %v\n",
				file,
				err,
			)
		}
	}
}

func PrintRowGroupInfo(
	path string,
	noDetails bool,
	allCols bool,
) error {

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf(
			"open parquet %s: %w",
			path,
			err,
		)
	}

	defer file.Close()

	reader := parquet.NewGenericReader[any](
		file,
	)

	schema := reader.File().Schema()

	columnNames := schema.Columns()

	rowGroups := reader.File().RowGroups()

	fmt.Printf(
		"\nFile=%s\n",
		filepath.Base(path),
	)

	interesting := map[string]bool{
		"expiry_date":  true,
		"strike":       true,
		"window_start": true,
	}

	for rgNo, rg := range rowGroups {

		fmt.Printf(
			"\nRowGroup=%d rows=%d\n",
			rgNo,
			rg.NumRows(),
		)

		if noDetails {
			continue
		}

		for colNo, chunk := range rg.ColumnChunks() {

			name := columnNames[colNo][0]

			name = strings.TrimPrefix(
				name,
				"name=",
			)

			if !allCols && !interesting[name] {
				// fmt.Printf(
				// 	"  %-12q (skipped)\n",
				// 	name,
				// )
				continue
			}

			fc, ok := chunk.(*parquet.FileColumnChunk)
			if !ok {
				fmt.Printf(
					"column=%s not a file column chunk\n",
					name,
				)
				continue
			}

			idx, err := fc.ColumnIndex()
			if err != nil {
				fmt.Printf(
					"column=%s column-index-error=%v\n",
					name,
					err,
				)
				continue
			}

			if idx.NumPages() == 0 {
				fmt.Printf(
					"column=%s no-pages\n",
					name,
				)
				continue
			}

			overallMin := idx.MinValue(0).Uint32()
			overallMax := idx.MaxValue(0).Uint32()

			for page := 1; page < idx.NumPages(); page++ {

				pageMin := idx.MinValue(page).Uint32()
				pageMax := idx.MaxValue(page).Uint32()

				if pageMin < overallMin {
					overallMin = pageMin
				}

				if pageMax > overallMax {
					overallMax = pageMax
				}
			}

			offsetIdx, err := fc.OffsetIndex()
			if err != nil {
				fmt.Printf(
					"column=%s offset-index-error=%v\n",
					name,
					err,
				)
				continue
			}

			for page := 0; page < idx.NumPages(); page++ {

				firstRow := offsetIdx.FirstRowIndex(
					page,
				)

				var rowCount int64

				if page < idx.NumPages()-1 {

					rowCount =
						offsetIdx.FirstRowIndex(page+1) -
							firstRow

				} else {

					rowCount =
						rg.NumRows() -
							firstRow
				}

				fmt.Printf(
					"      page=%d rows=%d min=%v max=%v\n",
					page,
					rowCount,
					idx.MinValue(page),
					idx.MaxValue(page),
				)
			}

			fmt.Printf(
				"  %-12s min=%v max=%v\n",
				name,
				overallMin,
				overallMax,
			)
		}
	}

	return nil
}

func DiscoverParquetFiles(
	path string,
) ([]string, error) {

	info, err := os.Stat(path)

	if err != nil {
		return nil, err
	}

	//
	// Single parquet file
	//
	if !info.IsDir() {

		if strings.EqualFold(
			filepath.Ext(path),
			".parquet",
		) {
			return []string{path}, nil
		}

		return nil,
			fmt.Errorf(
				"%s is not a parquet file",
				path,
			)
	}

	//
	// Directory -> recursive walk
	//
	var files []string

	err = filepath.Walk(
		path,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if strings.EqualFold(
				filepath.Ext(path),
				".parquet",
			) {

				files = append(
					files,
					path,
				)
			}

			return nil
		},
	)

	sort.Strings(files)

	return files, err
}
