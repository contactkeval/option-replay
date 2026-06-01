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

	statePath := filepath.Join(
		parquetDir,
		"parquet_state.json",
	)

	state, err := LoadParquetState(
		statePath,
	)

	if err != nil {
		return err
	}

	activeMetaPath := filepath.Join(
		parquetDir,
		"metadata_active.csv",
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

	// discover new expiries
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

	accumulator := RowGroupAccumulator{}

	archiveMetaPath := filepath.Join(
		cfg.ParquetRoot,
		"_metadata",
		"archive.csv",
	)

	if err := os.MkdirAll(
		filepath.Dir(archiveMetaPath),
		0755,
	); err != nil {
		return err
	}

	for i := range activeRows {

		r := &activeRows[i]

		if r.Status != "pending" {
			continue
		}

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

		// rotate parquet ONLY BEFORE new expiry
		// NEVER mid-expiry
		if state.CurrentFile == "" ||
			state.RowGroups >= constants.MaxRowGroupsPerFile {

			state.CurrentFile = fmt.Sprintf(
				"%s_%s.parquet",
				ticker,
				r.Expiry,
			)

			state.RowGroups = 0
		}

		startRowGroup := state.RowGroups

		flushedGroups := accumulator.AppendExpiry(rows)

		parquetPath := filepath.Join(
			parquetDir,
			state.CurrentFile,
		)

		for _, group := range flushedGroups {

			logger.Infof(
				"writing parquet rows=%d file=%s row_group=%d",
				len(group),
				parquetPath,
				state.RowGroups,
			)

			if err := WriteRowGroup(
				parquetPath,
				group,
			); err != nil {
				return err
			}

			state.RowGroups++
		}

		rowGroupCount := state.RowGroups - startRowGroup

		archiveRow := model.ArchiveMetadataRow{
			Ticker: ticker,
			Expiry: r.Expiry,
			Rows:   r.Rows,

			ParquetFile: state.CurrentFile,

			StartRowGroup: startRowGroup,
			RowGroupCount: rowGroupCount,

			ArchivedAt: time.Now().
				UTC().
				Format(time.RFC3339),
		}

		if err := AppendArchiveMetadata(
			archiveMetaPath,
			archiveRow,
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

		r.Status = "processed"
	}

	// flush trailing partial RG
	remaining := accumulator.FlushRemaining()

	if len(remaining) > 0 {

		parquetPath := filepath.Join(
			parquetDir,
			state.CurrentFile,
		)

		logger.Infof(
			"flushing trailing row group rows=%d file=%s row_group=%d",
			len(remaining),
			parquetPath,
			state.RowGroups,
		)

		if err := WriteRowGroup(
			parquetPath,
			remaining,
		); err != nil {
			return err
		}

		state.RowGroups++
	}

	if err := SaveParquetState(
		statePath,
		state,
	); err != nil {
		return err
	}

	var remainingActive []model.ActiveMetadataRow

	for _, r := range activeRows {

		if r.Status == "pending" {

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
