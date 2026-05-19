package main

import (
	"log"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage1 "github.com/contactkeval/option-replay/internal/pipeline/stage1_raw_to_expiry"
	stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_expiry_rollover"
	stage3 "github.com/contactkeval/option-replay/internal/pipeline/stage3_sort_dedupe"
	stage4 "github.com/contactkeval/option-replay/internal/pipeline/stage4_parquet"
)

func main() {

	cfg := config.Load()

	if err := stage1.Run(cfg); err != nil {
		log.Fatal(err)
	}

	if err := stage2.Run(cfg); err != nil {
		log.Fatal(err)
	}

	if err := stage3.Run(cfg); err != nil {
		log.Fatal(err)
	}

	if err := stage4.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
