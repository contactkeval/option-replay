package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/parquet-go/parquet-go"
)

func stats() {

	// CHANGE THIS
	root := `F:\data\parquet\SPY`

	entries, err := os.ReadDir(root)
	if err != nil {
		panic(err)
	}

	fmt.Printf(
		"%-40s %-12s %-15s %-12s\n",
		"FILENAME",
		"SIZE(MB)",
		"ROWS",
		"ROW_GROUPS",
	)

	fmt.Println(strings.Repeat("-", 85))

	for _, entry := range entries {

		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".parquet") {
			continue
		}

		path := filepath.Join(root, entry.Name())

		file, err := os.Open(path)
		if err != nil {
			fmt.Printf("ERROR opening %s: %v\n", path, err)
			continue
		}

		info, err := file.Stat()
		if err != nil {
			file.Close()
			fmt.Printf("ERROR stat %s: %v\n", path, err)
			continue
		}

		pf, err := parquet.OpenFile(file, info.Size())
		if err != nil {
			file.Close()
			fmt.Printf("ERROR parquet %s: %v\n", path, err)
			continue
		}

		sizeMB := float64(info.Size()) / 1024.0 / 1024.0

		fmt.Printf(
			"%-40s %-12.2f %-15d %-12d\n",
			entry.Name(),
			sizeMB,
			pf.NumRows(),
			len(pf.RowGroups()),
		)

		file.Close()
	}
}
