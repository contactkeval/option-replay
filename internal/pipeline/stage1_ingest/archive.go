package stage1_ingest

import (
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func ArchiveRawFile(
	sourcePath string,
	cfg config.Config,
) error {

	rel, err := filepath.Rel(
		cfg.RawRoot,
		sourcePath,
	)

	if err != nil {
		return err
	}

	destination := filepath.Join(
		cfg.ArchiveRawRoot,
		rel,
	)

	if err := os.MkdirAll(
		filepath.Dir(destination),
		0755,
	); err != nil {
		return err
	}

	return os.Rename(sourcePath, destination)
}
