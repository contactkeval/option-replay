package stage0_occ

import (
	"fmt"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config, recordsByFile map[string][]db.OCCRecord, groupNo int) error {
	metadataDBPath := filepath.Join(cfg.MetadataRoot, "metadata.db")

	database, err := db.Open(db.Options{
		Path:    metadataDBPath,
		Schemas: db.SchemaMetadata,
	})
	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}
	defer database.Close()

	importer := NewImporter(database, groupNo)

	for fileName, records := range recordsByFile {
		stats, err := importer.ImportFile(fileName, records)
		if err != nil {
			return fmt.Errorf("import %s: %w", fileName, err)
		}

		fmt.Printf(
			"%s: read=%d inserted=%d deleted=%d updated=%d skipped=%d errors=%d\n",
			fileName,
			stats.RecordsRead,
			stats.Inserted,
			stats.Deleted,
			stats.Updated,
			stats.Skipped,
			stats.Errors,
		)
	}

	return nil
}
