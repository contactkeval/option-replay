package main

import (
	"fmt"
	"log"
	"time"

	"github.com/contactkeval/option-replay/internal/parquetbuilder"
)

func main() {

	stagingRoot := `F:\data\staging`

	parquetRoot := `F:\data\parquet`

	tickerFiles, err := parquetbuilder.DiscoverTickerFiles(stagingRoot, parquetRoot)
	if err != nil {
		log.Fatal(err)
	}

	for ticker, files := range tickerFiles {

		fmt.Printf(
			"%s PROCESSING %s (%d files)\n",
			time.Now().Format("2006-01-02 15:04:05"),
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
