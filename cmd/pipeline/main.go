package main

import (
	"log"

	"github.com/contactkeval/option-replay/internal/s3sync"
)

func main() {
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
