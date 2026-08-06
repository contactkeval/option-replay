package stage0_occ

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

// DefaultDownloadTypes is the usual OCC sync set: adds, deletes, modifications.
var DefaultDownloadTypes = []string{ActionAdd, ActionDelete, ActionModify}

func Run(
	ctx context.Context,
	cfg config.Config,
	fileDate time.Time,
	downloadTypes []string,
) error {
	if len(downloadTypes) == 0 {
		downloadTypes = DefaultDownloadTypes
	}

	logger.Infof("----------------------------------------")
	logger.Infof("Stage 0 - OCC contract metadata sync")
	logger.Infof("File date : %s", fileDate.Format("2006-01-02"))
	logger.Infof("Types     : %s", strings.Join(downloadTypes, ","))
	logger.Infof("----------------------------------------")

	start := time.Now()

	metadataDBPath := filepath.Join(cfg.MetadataRoot, "metadata.db")
	database, err := db.Open(db.Options{
		Path:    metadataDBPath,
		Schemas: db.SchemaContracts | db.SchemaOCC,
	})
	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}
	defer database.Close()

	downloader := NewDownloader(cfg.OCCDataRoot)
	importer := NewImporter(database)

	var total db.ImportStatistics

	for _, downloadType := range downloadTypes {
		if err := ctx.Err(); err != nil {
			return err
		}

		filename, err := downloader.Download(ctx, fileDate, downloadType)
		if err != nil {
			return fmt.Errorf("download type %s: %w", downloadType, err)
		}
		if filename == "" {
			continue
		}

		stats, err := importer.ImportFile(filename, fileDate, downloadType)
		if err != nil {
			return fmt.Errorf("import %s: %w", filename, err)
		}

		logger.Infof("%s", FormatStats(stats))

		total.RecordsRead += stats.RecordsRead
		total.Processed += stats.Processed
		total.Ignored += stats.Ignored
		total.Inserted += stats.Inserted
		total.Existing += stats.Existing
		total.Deleted += stats.Deleted
		total.Updated += stats.Updated
		total.Errors += stats.Errors
	}

	logger.Infof(
		"OCC sync done in %s: read=%d processed=%d ignored=%d inserted=%d existing=%d deleted=%d updated=%d errors=%d",
		time.Since(start).Round(time.Millisecond),
		total.RecordsRead,
		total.Processed,
		total.Ignored,
		total.Inserted,
		total.Existing,
		total.Deleted,
		total.Updated,
		total.Errors,
	)

	return nil
}
