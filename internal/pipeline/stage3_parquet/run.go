package stage3_parquet

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/constants"
)

func Run(cfg config.Config) error {

	stage3Root := cfg.Stage3Root
	parquetRoot := cfg.ParquetRoot

	logger.Infof("Stage 3 processing started")
	tickers, err := os.ReadDir(stage3Root)
	if err != nil {
		return err
	}

	for _, tickerEntry := range tickers {

		if !tickerEntry.IsDir() {
			continue
		}

		ticker := tickerEntry.Name()

		if ticker == "_metadata" {
			continue
		}

		if err := ProcessTicker(
			ticker,
			cfg,
			parquetRoot,
		); err != nil {
			return err
		}
	}

	return nil
}

func ProcessTicker(
	ticker string,
	cfg config.Config,
	parquetRoot string,
) error {

	stage3TickerDir := filepath.Join(
		cfg.Stage3Root,
		ticker,
	)

	files, err := filepath.Glob(
		filepath.Join(stage3TickerDir, "*.csv"),
	)

	if err != nil {
		return err
	}

	sort.Strings(files)

	metaDir := filepath.Join(
		parquetRoot,
		"_metadata",
	)

	os.MkdirAll(metaDir, 0755)

	metaPath := filepath.Join(
		metaDir,
		ticker+".json",
	)

	meta, err := LoadMetadata(metaPath)
	if err != nil {
		return err
	}

	var pendingRows []ParquetRow
	pendingCount := meta.PendingRows

	for _, file := range files {

		rows, err := LoadRows(file)
		if err != nil {
			return err
		}

		if pendingCount > 0 &&
			pendingCount+len(rows) > constants.RowGroupTargetRows {

			parquetPath := filepath.Join(
				parquetRoot,
				ticker,
				meta.CurrentFile,
			)

			os.MkdirAll(
				filepath.Dir(parquetPath),
				0755,
			)

			logger.Infof(
				"writing row group: %s rows=%d",
				parquetPath,
				len(pendingRows),
			)

			if err := WriteRowGroup(
				parquetPath,
				pendingRows,
			); err != nil {
				return err
			}

			meta.RowGroups++

			pendingRows = nil
			pendingCount = 0

			if meta.RowGroups >= constants.MaxRowGroupsPerFile {

				meta.RowGroups = 0
				meta.CurrentFile = ""
			}
		}

		if meta.CurrentFile == "" {

			base := filepath.Base(file)
			expiry := strings.TrimSuffix(
				strings.Split(base, "_")[1],
				".csv",
			)

			meta.CurrentFile =
				ticker + "_" + expiry + ".parquet"
		}

		pendingRows = append(
			pendingRows,
			rows...,
		)

		pendingCount += len(rows)

		meta.PendingRows = pendingCount

		if err := SaveMetadata(
			metaPath,
			meta,
		); err != nil {
			return err
		}

		archivePath := filepath.Join(
			cfg.ArchiveSortedRoot,
			ticker,
			filepath.Base(file)+".gz",
		)

		if err := ArchiveStage3File(
			file,
			archivePath,
		); err != nil {
			return err
		}
	}

	if len(pendingRows) > 0 {

		parquetPath := filepath.Join(
			parquetRoot,
			ticker,
			meta.CurrentFile,
		)

		os.MkdirAll(
			filepath.Dir(parquetPath),
			0755,
		)

		if err := WriteRowGroup(
			parquetPath,
			pendingRows,
		); err != nil {
			return err
		}

		meta.RowGroups++
		meta.PendingRows = 0

		if err := SaveMetadata(
			metaPath,
			meta,
		); err != nil {
			return err
		}
	}

	return nil
}
