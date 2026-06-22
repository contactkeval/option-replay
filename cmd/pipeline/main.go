package main

import (
	"log"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	stage1 "github.com/contactkeval/option-replay/internal/pipeline/stage1_dxfeed"
	// stage2 "github.com/contactkeval/option-replay/internal/pipeline/stage2_ingest"
	// stage3 "github.com/contactkeval/option-replay/internal/pipeline/stage3_parquet"
	// stage4 "github.com/contactkeval/option-replay/internal/pipeline/stage4_compact"
)

func main() {

	// cfg := config.Load()

	if err := config.LoadAllowedUnderlyings(
		"../../internal/pipeline/config/allowed_underlyings.txt",
	); err != nil {
		log.Fatal(err)
	}

	if err := stage1.Run(); err != nil {
		log.Fatal(err)
	}

	// stage1.Run()

	// if err := stage2.Run(cfg); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := stage3.Run(cfg); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := stage4.Run(cfg); err != nil {
	// 	log.Fatal(err)
	// }
}
