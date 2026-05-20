package stage3_sort_dedupe

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	return filepath.Walk(
		cfg.Stage3Root,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".csv") {
				return nil
			}

			logger.Infof(
				"stage3 processing path=%s",
				path,
			)

			rows, err := LoadRows(path)

			if err != nil {
				return err
			}

			SortRows(rows)

			rows, duplicates := DedupeRows(rows)

			if err := RewriteFile(
				cfg,
				path,
				rows,
				duplicates,
			); err != nil {

				return err
			}

			logger.Infof(
				"stage3 completed path=%s rows=%d duplicates_removed=%d",
				path,
				len(rows),
				duplicates,
			)
			return nil
		},
	)
}
