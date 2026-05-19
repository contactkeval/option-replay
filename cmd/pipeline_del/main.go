package main

import (
	"log"

	"github.com/contactkeval/option-replay/internal/s3sync"
	"github.com/contactkeval/option-replay/internal/staging"
)

func main_old() {
	cfg := s3sync.Config{
		Bucket:              "your-bucket-name",
		Prefix:              "", // optional
		LocalDir:            "./data",
		Region:              "us-east-1",
		ConcurrentDownloads: 5,
	}

	if err := s3sync.Sync(cfg); err != nil {
		log.Fatal(err)
	}
}

func main() {

	inputRoot := `F:\data\minute_aggs_v1`

	outputRoot := `F:\data\staging`

	if err := staging.Run(inputRoot, outputRoot); err != nil {
		log.Fatal(err)
	}
}
