package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run inspect_parquet.go <parquet-root-dir>")
		return
	}

	root := os.Args[1]

	files, err := filepath.Glob(filepath.Join(root, "*.parquet"))
	if err != nil {
		fmt.Printf("error discovering parquet files: %v\n", err)
		return
	}

	if len(files) == 0 {
		fmt.Println("No parquet files found")
		return
	}

	for _, path := range files {
		if err := PrintRowGroupInfo(path); err != nil {
			fmt.Printf("error processing %s: %v\n", path, err)
		}
	}
}

func PrintRowGroupInfo(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open parquet %s: %w", path, err)
	}
	defer file.Close()

	reader := parquet.NewGenericReader[any](file)
	rowGroups := reader.File().RowGroups()

	for i, rg := range rowGroups {
		fmt.Printf("%s, %d, %d\n",
			filepath.Base(path), // FileName
			i,                   // RowGroupNo
			rg.NumRows(),        // RowCount
		)
	}

	return nil
}
