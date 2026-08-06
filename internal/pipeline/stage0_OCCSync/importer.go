package stage0_occ

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type Importer struct {
	database *db.DB
}

func NewImporter(database *db.DB) *Importer {
	return &Importer{database: database}
}

func (i *Importer) ImportFile(
	fileName string,
	fileDate time.Time,
	downloadType string,
) (db.ImportStatistics, error) {
	stats := db.ImportStatistics{
		FileName:     fileName,
		FileDate:     fileDate,
		DownloadType: downloadType,
		StartedAt:    time.Now(),
	}

	importID, err := i.database.StartImport(fileName, fileDate, downloadType)
	if err != nil {
		return stats, err
	}

	tx, err := i.database.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	err = ReadFile(
		fileName,
		func(record db.OCCRecord) error {
			stats.RecordsRead++

			if !config.IsAllowedUnderlying(record.Underlying) {
				stats.Ignored++
				return nil
			}

			stats.Processed++

			groupNo := GroupNoForExpiry(record.ExpiryDate)

			var (
				applied   bool
				actionErr error
			)

			switch record.Action {
			case ActionAdd:
				applied, actionErr = i.database.HandleOCCAdd(tx, record, groupNo)
				if actionErr == nil {
					if applied {
						stats.Inserted++
					} else {
						stats.Existing++
					}
				}
			case ActionDelete:
				applied, actionErr = i.database.HandleOCCDelete(tx, record)
				if actionErr == nil && applied {
					stats.Deleted++
				}
			case ActionModify:
				applied, actionErr = i.database.HandleOCCModify(tx, record, groupNo)
				if actionErr == nil && applied {
					stats.Updated++
				}
			default:
				stats.Ignored++
				stats.Processed--
				return nil
			}

			if actionErr != nil {
				stats.Errors++
				logger.Warnf(
					"OCC %s %s %s %.3f %s: %v",
					record.Action,
					record.Underlying,
					record.ExpiryDate.Format("2006-01-02"),
					record.Strike,
					record.Type,
					actionErr,
				)
			}

			return nil
		},
		func(lineNo int, line string, parseErr error) error {
			stats.RecordsRead++
			stats.Errors++
			logger.Warnf("line %d: %v", lineNo, parseErr)
			return nil
		},
	)
	if err != nil {
		stats.EndedAt = time.Now()
		_ = i.database.FailImport(importID, stats)
		return stats, err
	}

	if err := tx.Commit(); err != nil {
		stats.EndedAt = time.Now()
		_ = i.database.FailImport(importID, stats)
		return stats, fmt.Errorf("commit import: %w", err)
	}

	stats.EndedAt = time.Now()

	if err := i.database.CompleteImport(importID, stats); err != nil {
		return stats, err
	}

	return stats, nil
}

func FormatStats(s db.ImportStatistics) string {
	return fmt.Sprintf(
		"%s type=%s fileDate=%s read=%d processed=%d ignored=%d inserted=%d existing=%d deleted=%d updated=%d errors=%d duration=%s",
		filepath.Base(s.FileName),
		s.DownloadType,
		s.FileDate.Format("2006-01-02"),
		s.RecordsRead,
		s.Processed,
		s.Ignored,
		s.Inserted,
		s.Existing,
		s.Deleted,
		s.Updated,
		s.Errors,
		s.EndedAt.Sub(s.StartedAt).Round(time.Millisecond),
	)
}
