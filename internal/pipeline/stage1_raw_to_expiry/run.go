package stage1_raw_to_expiry

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

	defer cache.CloseAll()

	for _, file := range files {

		if err := ProcessRawFile(file, cache); err != nil {
			return err
		}

		cache.CloseAll()

		if err := ArchiveRawFile(file, cfg); err != nil {
			return err
		}
	}

	return nil
}
