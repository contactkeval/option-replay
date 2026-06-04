package stage3_parquet

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func Run(cfg config.Config) error {

	stage3Root := cfg.Stage3Root

	entries, err := os.ReadDir(stage3Root)

	if err != nil {

		return fmt.Errorf(
			"read stage3 root %s: %w",
			stage3Root,
			err,
		)
	}

	for _, entry := range entries {

		if !entry.IsDir() {
			continue
		}

		if entry.Name() == "_metadata" {
			continue
		}

		ticker := entry.Name()

		logger.Infof(
			"stage3 processing ticker=%s",
			ticker,
		)

		if err := ProcessTicker(
			ticker,
			cfg,
		); err != nil {

			return fmt.Errorf(
				"process ticker=%s: %w",
				ticker,
				err,
			)
		}
	}

	return nil
}

func ProcessTicker(
	ticker string,
	cfg config.Config,
) error {

	stage3Dir := filepath.Join(
		cfg.Stage3Root,
		ticker,
	)

	parquetDir := filepath.Join(
		cfg.ParquetRoot,
		ticker,
	)

	if err := os.MkdirAll(
		parquetDir,
		0755,
	); err != nil {

		return err
	}

	activeMetaPath := filepath.Join(
		cfg.MetadataRoot,
		"active",
		ticker+".csv",
	)

	activeParquetMetaPath := filepath.Join(
		cfg.MetadataRoot,
		"parquet_active",
		ticker+".csv",
	)

	activeRows, err := LoadActiveMetadata(
		activeMetaPath,
	)

	if err != nil {
		return err
	}

	known := make(map[string]bool)

	for _, r := range activeRows {
		known[r.Expiry] = true
	}

	currentFiles, err := filepath.Glob(
		filepath.Join(stage3Dir, "*.csv"),
	)

	if err != nil {
		return err
	}

	sort.Strings(currentFiles)

	for _, f := range currentFiles {

		expiry := ExtractExpiry(f)

		if known[expiry] {
			continue
		}

		rowCount, err := CountRows(f)

		if err != nil {
			return err
		}

		activeRows = append(
			activeRows,
			model.ActiveMetadataRow{
				Expiry: expiry,
				Rows:   rowCount,
				Status: "pending",
			},
		)
	}

	sort.Slice(
		activeRows,
		func(i, j int) bool {
			return activeRows[i].Expiry <
				activeRows[j].Expiry
		},
	)

	if err := SaveActiveMetadata(
		activeMetaPath,
		activeRows,
	); err != nil {

		return err
	}

	for {

		if len(activeRows) == 0 {
			break
		}

		firstExpiry := activeRows[0].Expiry

		parquetFile := fmt.Sprintf(
			"%s_%s.parquet",
			ticker,
			firstExpiry,
		)

		parquetPath := filepath.Join(
			parquetDir,
			parquetFile,
		)

		accumulator := RowGroupAccumulator{}

		var completedRGs [][]model.ParquetRow

		var candidateExpiries []model.ActiveMetadataRow

		shouldFinalize := false

		for _, r := range activeRows {

			filePath := filepath.Join(
				stage3Dir,
				fmt.Sprintf(
					"%s_%s.csv",
					ticker,
					r.Expiry,
				),
			)

			rows, err := LoadRows(filePath)

			if err != nil {
				return err
			}

			flushed := accumulator.AppendExpiry(
				rows,
			)

			completedRGs = append(
				completedRGs,
				flushed...,
			)

			candidateExpiries = append(
				candidateExpiries,
				r,
			)

			trailingRows :=
				accumulator.PendingCount()

			rowGroups :=
				len(completedRGs)

			logger.Infof(
				"ticker=%s row_groups=%d trailing_rows=%d",
				ticker,
				rowGroups,
				trailingRows,
			)

			if rowGroups >=
				constants.TargetRowGroupsPerFile {

				shouldFinalize = true
				break
			}

			if rowGroups > 0 &&
				trailingRows <=
					constants.MaxTrailingRows {

				shouldFinalize = true
				break
			}
		}

		if !shouldFinalize {

			logger.Infof(
				"ticker=%s nothing eligible for parquet finalization",
				ticker,
			)

			break
		}

		trailing :=
			accumulator.PendingRows()

		if len(trailing) > 0 {

			if len(completedRGs) > 0 {

				last :=
					len(completedRGs) - 1

				completedRGs[last] =
					append(
						completedRGs[last],
						trailing...,
					)

			} else {

				completedRGs =
					append(
						completedRGs,
						trailing,
					)
			}
		}

		logger.Infof(
			"finalizing parquet file=%s row_groups=%d",
			parquetPath,
			len(completedRGs),
		)

		pw, err := NewParquetFileWriter(
			parquetPath,
		)

		if err != nil {
			return err
		}

		for i, rg := range completedRGs {

			logger.Infof(
				"writing parquet rows=%d file=%s row_group=%d",
				len(rg),
				parquetPath,
				i,
			)

			if err := pw.WriteRowGroup(
				rg,
			); err != nil {

				_ = pw.Close()
				return err
			}

			if err := pw.FlushRowGroup(); err != nil {

				_ = pw.Close()
				return err
			}
		}

		if err := pw.Close(); err != nil {
			return err
		}

		processedExpiries :=
			make(map[string]struct{})

		for _, r := range candidateExpiries {

			meta := model.ActiveParquetMetadataRow{
				Ticker: ticker,
				Expiry: r.Expiry,
				Rows:   r.Rows,

				ParquetFile: parquetFile,

				StartRowGroup: 0,
				RowGroupCount: len(completedRGs),

				Status: "active",

				CreatedAt: time.Now().
					UTC().
					Format(time.RFC3339),
			}

			if err := AppendActiveParquetMetadata(
				activeParquetMetaPath,
				meta,
			); err != nil {

				return err
			}

			sourceFile := filepath.Join(
				stage3Dir,
				fmt.Sprintf(
					"%s_%s.csv",
					ticker,
					r.Expiry,
				),
			)

			archivePath := filepath.Join(
				cfg.ArchiveSortedRoot,
				ticker,
				"20"+r.Expiry[:2],
				filepath.Base(sourceFile)+".gz",
			)

			if err := ArchiveFile(
				sourceFile,
				archivePath,
			); err != nil {

				return err
			}

			processedExpiries[r.Expiry] =
				struct{}{}
		}

		var remaining []model.ActiveMetadataRow

		for _, r := range activeRows {

			if _, found :=
				processedExpiries[r.Expiry]; !found {

				remaining = append(
					remaining,
					r,
				)
			}
		}

		activeRows = remaining

		if err := SaveActiveMetadata(
			activeMetaPath,
			activeRows,
		); err != nil {

			return err
		}
	}

	return nil
}

func ExtractExpiry(path string) string {

	base := filepath.Base(path)

	parts := strings.Split(
		base,
		"_",
	)

	if len(parts) < 2 {
		return "unknown"
	}

	return strings.TrimSuffix(
		parts[1],
		".csv",
	)
}
