package stage2_finalize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/transientdb"
)

func Run(cfg config.Config) error {

	logger.Infof("Stage 2 processing started")

	transientDBPath := filepath.Join(
		cfg.SQLiteRoot,
		"transient.db",
	)

	db, err := transientdb.Open(
		transientDBPath,
	)

	if err != nil {
		return fmt.Errorf("open transient database: %w", err)
	}

	defer db.Close()

	// TODO: Log rejected rows details to a separate table for later analysis
	if err = transientdb.EnsureRejectedRowsTable(db); err != nil {
		return fmt.Errorf("ensure rejected rows table: %w", err)
	}

	return filepath.Walk(
		cfg.RawRoot,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {

			if err != nil {
				return fmt.Errorf("walk directory %s: %w", cfg.RawRoot, err)
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(
				path,
				".csv.gz",
			) {
				return nil
			}

			logger.Infof(
				"processing: %s",
				path,
			)

			expiryRows, err := ProcessRawFile(
				path,
			)

			if err != nil {
				return fmt.Errorf("process raw file %s: %w", path, err)
			}

			// --------------------------------
			// BEGIN TRANSACTION
			// --------------------------------

			tx, err := db.Begin()

			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}

			for expiry, rows := range expiryRows {

				err := transientdb.EnsureExpiryTable(
					tx,
					expiry,
				)

				if err != nil {
					tx.Rollback()
					return fmt.Errorf("ensure expiry table for %s: %w", expiry, err)
				}

				err = transientdb.InsertBars(
					tx,
					expiry,
					rows,
				)

				if err != nil {
					tx.Rollback()
					return fmt.Errorf("insert bars for %s: %w", expiry, err)
				}

				// logger.Infof(
				// 	"expiry=%s rows=%d",
				// 	expiry,
				// 	len(rows),
				// )
			}

			// --------------------------------
			// COMMIT TRANSACTION
			// --------------------------------

			err = tx.Commit()

			if err != nil {
				return fmt.Errorf("commit transaction: %w", err)
			}

			err = ArchiveRawFile(
				path,
				cfg.RawRoot,
				cfg.ArchiveRawRoot,
			)

			if err != nil {
				return fmt.Errorf("archive raw file %s: %w", path, err)
			}

			// logger.Infof(
			// 	"completed raw file: %s",
			// 	path,
			// )

			return nil
		},
	)
}

func ArchiveRawFile(
	sourcePath string,
	rawRoot string,
	archivedRoot string,
) error {

	rel, err := filepath.Rel(
		rawRoot,
		sourcePath,
	)

	if err != nil {
		return fmt.Errorf(
			"calculate relative path: %w",
			err,
		)
	}

	targetPath := filepath.Join(
		archivedRoot,
		rel,
	)

	err = os.MkdirAll(
		filepath.Dir(targetPath),
		0755,
	)

	if err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	err = os.Rename(
		sourcePath,
		targetPath,
	)

	if err != nil {
		return fmt.Errorf(
			"move raw file to archive: %w",
			err,
		)
	}

	// logger.Infof(
	// 	"archived raw file: %s",
	// 	targetPath,
	// )

	return nil
}
