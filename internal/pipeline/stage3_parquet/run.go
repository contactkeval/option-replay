package stage3_parquet

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func Run(cfg config.Config) error {
	stage3Root := cfg.Stage3Root
	entries, err := os.ReadDir(stage3Root)
	if err != nil {
		return fmt.Errorf("read stage3 root %s: %w", stage3Root, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip metadata folder
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
			"glob stage3 files ticker=%s: %w",
			ticker,
			err,
		)
	}

	if len(currentFiles) == 0 {
		logger.Infof(
			"ticker=%s no pending csv files",
			ticker,
		)
		return nil
	}

	sort.Strings(currentFiles)

	parquetDir := filepath.Join(
		cfg.ParquetRoot,
		ticker,
	)

	if err := os.MkdirAll(
		parquetDir,
		0755,
	); err != nil {

		return fmt.Errorf(
			"create parquet directory %s: %w",
			parquetDir,
			err,
		)
	}

	metaPath := filepath.Join(
		parquetDir,
		"_metadata.json",
	)

	meta, err := LoadMetadata(metaPath)

	if err != nil {
		return fmt.Errorf(
			"load metadata %s: %w",
			metaPath,
			err,
		)
	}

	// Build pending file set
	pendingSet := make(map[string]bool)

	for _, f := range meta.PendingFiles {
		pendingSet[f] = true
	}

	// Add newly discovered files
	for _, f := range currentFiles {
		if pendingSet[f] {
			continue
		}

		rows, err := CountRows(f)
		if err != nil {
			return fmt.Errorf(
				"count rows file=%s: %w",
				f,
				err,
			)
		}

		meta.PendingFiles = append(
			meta.PendingFiles,
			f,
		)

		meta.PendingRows += rows

		logger.Infof(
			"ticker=%s added pending file=%s rows=%d total_pending=%d",
			ticker,
			filepath.Base(f),
			rows,
			meta.PendingRows,
		)
	}

	// Sanity check
	if len(meta.PendingFiles) == 0 &&
		meta.PendingRows != 0 {

		return fmt.Errorf(
			"inconsistent metadata ticker=%s pending_rows=%d pending_files=0",
			ticker,
			meta.PendingRows,
		)
	}

	// Still below threshold
	if meta.PendingRows < constants.RowGroupTargetRows {
		logger.Infof(
			"ticker=%s pending_rows=%d below threshold=%d",
			ticker,
			meta.PendingRows,
			constants.RowGroupTargetRows,
		)

		if err := SaveMetadata(
			metaPath,
			meta,
		); err != nil {

			return fmt.Errorf(
				"save metadata %s: %w",
				metaPath,
				err,
			)
		}

		return nil
	}

	logger.Infof(
		"ticker=%s flushing parquet pending_files=%d pending_rows=%d",
		ticker,
		len(meta.PendingFiles),
		meta.PendingRows,
	)

	// Load ALL pending rows
	var batchRows []model.ParquetRow

	for _, f := range meta.PendingFiles {
		rows, err := LoadRows(f)
		if err != nil {
			return fmt.Errorf(
				"load rows file=%s: %w",
				f,
				err,
			)
		}

		batchRows = append(
			batchRows,
			rows...,
		)
	}

	// Flush parquet row group
	if err := FlushRowGroup(
		ticker,
		&meta,
		batchRows,
		cfg,
	); err != nil {

		return fmt.Errorf(
			"flush row group ticker=%s: %w",
			ticker,
			err,
		)
	}

	// Archive ONLY after successful parquet write
	for _, f := range meta.PendingFiles {

		expiry := ExtractExpiry(f)
		archivePath := filepath.Join(
			cfg.ArchiveSortedRoot,
			ticker,
			"20"+expiry[:2],
			filepath.Base(f)+".gz",
		)

		if err := ArchiveFile(
			f,
			archivePath,
		); err != nil {

			return fmt.Errorf(
				"archive file=%s: %w",
				f,
				err,
			)
		}
	}

	// ONLY NOW commit metadata changes

	meta.RowGroups++

	meta.PendingFiles = nil
	meta.PendingRows = 0

	// Rotate parquet file
	if meta.RowGroups >= constants.MaxRowGroupsPerFile {

		logger.Infof(
			"ticker=%s rotating parquet file row_groups=%d",
			ticker,
			meta.RowGroups,
		)

		meta.CurrentFile = ""
		meta.RowGroups = 0
	}

	if err := SaveMetadata(
		metaPath,
		meta,
	); err != nil {

		return fmt.Errorf(
			"save metadata %s: %w",
			metaPath,
			err,
		)
	}

	logger.Infof(
		"ticker=%s parquet flush complete row_groups=%d",
		ticker,
		meta.RowGroups,
	)

	return nil
}

func FlushRowGroup(
	ticker string,
	meta *model.TickerMetadata,
	rows []model.ParquetRow,
	cfg config.Config,
) error {

	if len(rows) == 0 {

		return fmt.Errorf(
			"empty row group ticker=%s",
			ticker,
		)
	}

	if meta.CurrentFile == "" {

		firstExpiry := FirstExpiryFromFiles(
			meta.PendingFiles,
		)

		meta.CurrentFile = fmt.Sprintf(
			"%s_%s.parquet",
			ticker,
			firstExpiry,
		)
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

	parquetPath := filepath.Join(
		parquetDir,
		meta.CurrentFile,
	)

	logger.Infof(
		"writing parquet=%s rows=%d",
		parquetPath,
		len(rows),
	)

	if err := WriteRowGroup(
		parquetPath,
		rows,
	); err != nil {

		return fmt.Errorf(
			"write row group parquet=%s: %w",
			parquetPath,
			err,
		)
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

func FirstExpiryFromFiles(
	files []string,
) string {

	if len(files) == 0 {
		return "unknown"
	}
	return ExtractExpiry(files[0])
}
