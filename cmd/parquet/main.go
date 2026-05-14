package main

import (
	"fmt"
	"log"

	"option-replay/internal/parquetbuilder"
)

func main() {

	stagingRoot := `F:\data\staging`

	parquetRoot := `F:\data\parquet`

	tickerFiles, err := parquetbuilder.DiscoverTickerFiles(stagingRoot)
	if err != nil {
		log.Fatal(err)
	}

	for ticker, files := range tickerFiles {

		fmt.Printf(
			"PROCESSING %s (%d files)\n",
			ticker,
			len(files),
		)

		if err := parquetbuilder.BuildParquetForTicker(
			ticker,
			files,
			parquetRoot,
		); err != nil {
			log.Fatal(err)
		}
	}
}
