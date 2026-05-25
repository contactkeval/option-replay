package stage1_ingest

import (
	"fmt"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	files, err := DiscoverRawFiles(cfg.RawRoot)
	if err != nil {
		return err
	}

	fmt.Printf("FOUND %d RAW FILES\n", len(files))

	cache := NewWriterCache(
		cfg.Stage2Root,
		cfg.MaxOpenFiles,
	)

	for _, file := range files {

		if err := ProcessRawFile(file, cache); err != nil {
			return err
		}

		if err := cache.CloseAll(); err != nil {
			return err
		}

		if err := ArchiveRawFile(file, cfg); err != nil {
			return err
		}
	}

	return nil
}
