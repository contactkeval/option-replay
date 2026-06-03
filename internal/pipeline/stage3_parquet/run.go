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

	currentFiles, err := filepath.Glob(
		filepath.Join(stage3Dir, "*.csv"),
	)

	if err != nil {

		return fmt.Errorf(
			"discover stage3 files for %s: %w",
			ticker,
			err,
		)
	}

	sort.Strings(currentFiles)

	if len(currentFiles) == 0 {
		return nil
	}

	parquetDir := filepath.Join(
		cfg.ParquetRoot,
		ticker,
	)

	if err := os.MkdirAll(
		parquetDir,
		0755,
	); err != nil {

		return fmt.Errorf(
			"create parquet dir %s: %w",
			parquetDir,
			err,
		)
	}

	activeMetaPath := filepath.Join(
		cfg.MetadataRoot,
		"active",
		ticker+".csv",
	)

	if err := os.MkdirAll(
		filepath.Dir(activeMetaPath),
		0755,
	); err != nil {

		return fmt.Errorf(
			"create active metadata dir %s: %w",
			filepath.Dir(activeMetaPath),
			err,
		)
	}

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

	// Discover new expiries
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

	if err := SaveActiveMetadata(
		activeMetaPath,
		activeRows,
	); err != nil {

		return err
	}

	if len(activeRows) == 0 {
		return nil
	}

	accumulator := RowGroupAccumulator{}

	activeParquetMetaPath := filepath.Join(
		cfg.MetadataRoot,
		"parquet_active",
		ticker+".csv",
	)

	if err := os.MkdirAll(
		filepath.Dir(activeParquetMetaPath),
		0755,
	); err != nil {

		return fmt.Errorf(
			"create parquet metadata dir %s: %w",
			filepath.Dir(activeParquetMetaPath),
			err,
		)
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

	var candidateExpiries []model.ActiveMetadataRow
	var finalizedExpiries []model.ActiveMetadataRow

	var pw *ParquetFileWriter

	rowGroupNumber := 0

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

		candidateExpiries = append(
			candidateExpiries,
			r,
		)

		flushedGroups := accumulator.AppendExpiry(
			rows,
		)

		for _, group := range flushedGroups {

			if pw == nil {

				pw, err = NewParquetFileWriter(
					parquetPath,
				)

				if err != nil {
					return err
				}
			}

			logger.Infof(
				"writing parquet rows=%d file=%s row_group=%d",
				len(group),
				parquetPath,
				rowGroupNumber,
			)

			if err := pw.WriteRowGroup(
				group,
			); err != nil {
				return err
			}

			if err := pw.FlushRowGroup(); err != nil {
				return err
			}

			rowGroupNumber++
		}

		trailingRows :=
			accumulator.PendingRows()

		logger.Infof(
			"ticker=%s row_groups=%d trailing_rows=%d",
			ticker,
			rowGroupNumber,
			trailingRows,
		)

		// Need at least one RG before finalization
		if rowGroupNumber == 0 {
			continue
		}

		// Finalize ONLY when trailing rows are acceptable
		if trailingRows <= constants.MaxTrailingRows {

			logger.Infof(
				"finalizing parquet file=%s row_groups=%d trailing_rows=%d",
				parquetPath,
				rowGroupNumber,
				trailingRows,
			)

			finalizedExpiries =
				append(
					finalizedExpiries,
					candidateExpiries...,
				)

			break
		}
	}

	// Nothing eligible yet
	if len(finalizedExpiries) == 0 {

		logger.Infof(
			"ticker=%s nothing eligible for parquet finalization",
			ticker,
		)

		if pw != nil {
			_ = pw.Close()
			_ = os.Remove(parquetPath)
		}

		return nil
	}

	// Flush trailing rows into final RG
	remaining := accumulator.FlushRemaining()

	if len(remaining) > 0 {

		if pw == nil {

			pw, err = NewParquetFileWriter(
				parquetPath,
			)

			if err != nil {
				return err
			}
		}

		logger.Infof(
			"writing final trailing row group rows=%d file=%s row_group=%d",
			len(remaining),
			parquetPath,
			rowGroupNumber,
		)

		if err := pw.WriteRowGroup(
			remaining,
		); err != nil {
			return err
		}

		if err := pw.FlushRowGroup(); err != nil {
			return err
		}

		rowGroupNumber++
	}

	if pw != nil {

		if err := pw.Close(); err != nil {
			return err
		}
	}

	processedExpiries := make(map[string]struct{})

	for _, r := range finalizedExpiries {

		metadataRow := model.ActiveParquetMetadataRow{
			Ticker: ticker,
			Expiry: r.Expiry,
			Rows:   r.Rows,

			ParquetFile: parquetFile,

			StartRowGroup: -1,
			RowGroupCount: 0,

			Status: "active",

			CreatedAt: time.Now().
				UTC().
				Format(time.RFC3339),
		}

		if err := AppendActiveParquetMetadata(
			activeParquetMetaPath,
			metadataRow,
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

		processedExpiries[r.Expiry] = struct{}{}
	}

	var remainingActive []model.ActiveMetadataRow

	for _, r := range activeRows {

		if _, found :=
			processedExpiries[r.Expiry]; !found {

			remainingActive = append(
				remainingActive,
				r,
			)
		}
	}

	return SaveActiveMetadata(
		activeMetaPath,
		remainingActive,
	)
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
