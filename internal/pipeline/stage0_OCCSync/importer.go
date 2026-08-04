package stage0_occ

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
)

type Importer struct {
	database *db.DB
	groupNo  int
}

func NewImporter(database *db.DB, groupNo int) *Importer {
	return &Importer{
		database: database,
		groupNo:  groupNo,
	}
}

func (i *Importer) ImportFile(
	fileName string,
	records []db.OCCRecord,
) (db.ImportStatistics, error) {
	stats := db.ImportStatistics{
		FileName:  fileName,
		StartedAt: time.Now(),
	}

	importID, err := i.database.StartImport(fileName)
	if err != nil {
		return stats, err
	}

	tx, err := i.database.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	for _, record := range records {
		stats.RecordsRead++

		var actionErr error

		switch record.Action {
		case ActionAdd:
			actionErr = i.database.HandleOCCAdd(tx, record, i.groupNo)
			if actionErr == nil {
				stats.Inserted++
			}
		case ActionDelete:
			actionErr = i.database.HandleOCCDelete(tx, record)
			if actionErr == nil {
				stats.Deleted++
			}
		case ActionModify:
			actionErr = i.database.HandleOCCModify(tx, record, i.groupNo)
			if actionErr == nil {
				stats.Updated++
			}
		default:
			stats.Skipped++
			continue
		}

		if actionErr != nil {
			stats.Errors++
		}
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
