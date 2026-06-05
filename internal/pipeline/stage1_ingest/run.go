package stage1_ingest

import (
	"fmt"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	fmt.Printf("Stage 1 ingest started\n")
	// code to download raw files from source and save to local disk

	// files, err := DiscoverRawFiles(cfg.RawRoot)
	// if err != nil {
	// 	return err
	// }

	// fmt.Printf("FOUND %d RAW FILES\n", len(files))

	// cache := NewWriterCache(
	// 	cfg.Stage2Root,
	// 	constants.MaxOpenFiles,
	// )

	// for _, file := range files {

	// 	if err := ProcessRawFile(file, cache); err != nil {
	// 		return err
	// 	}

	// 	if err := cache.CloseAll(); err != nil {
	// 		return err
	// 	}

	// 	if err := ArchiveRawFile(file, cfg); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}
