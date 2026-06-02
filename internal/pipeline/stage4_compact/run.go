package stage4_compact

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/parquet-go/parquet-go"
)

type ParquetFile struct {
	Path      string
	FileName  string
	RowGroups int
}

func Run(cfg config.Config) error {

	entries, err := os.ReadDir(cfg.ParquetRoot)
	if err != nil {
		return fmt.Errorf(
			"read parquet root %s: %w",
			cfg.ParquetRoot,
			err,
		)
	}

	for _, entry := range entries {

		if !entry.IsDir() {
			continue
		}

		ticker := entry.Name()

		if err := ProcessTicker(
			ticker,
			cfg,
		); err != nil {

			return fmt.Errorf(
				"compact ticker=%s: %w",
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

	tickerDir := filepath.Join(
		cfg.ParquetRoot,
		ticker,
	)

	compactingDir := filepath.Join(
		tickerDir,
		".compacting",
	)

	// --------------------------------------------------
	// STEP 0 - recovery
	// --------------------------------------------------

	if err := RecoverCompaction(
		tickerDir,
		compactingDir,
	); err != nil {
		return err
	}

	// --------------------------------------------------
	// STEP 1 - discover parquet files
	// --------------------------------------------------

	files, err := filepath.Glob(
		filepath.Join(
			tickerDir,
			"*.parquet",
		),
	)

	if err != nil {
		return fmt.Errorf(
			"discover parquet files: %w",
			err,
		)
	}

	sort.Strings(files)

	var parquetFiles []ParquetFile

	for _, path := range files {

		rgCount, err := GetRowGroupCount(path)

		if err != nil {
			return err
		}

		parquetFiles = append(
			parquetFiles,
			ParquetFile{
				Path:      path,
				FileName:  filepath.Base(path),
				RowGroups: rgCount,
			},
		)
	}

	// --------------------------------------------------
	// STEP 2 - build batches
	// --------------------------------------------------

	start := 0

	for {

		if start >= len(parquetFiles) {
			break
		}

		totalRG := 0
		end := start

		for end < len(parquetFiles) {

			totalRG += parquetFiles[end].RowGroups
			end++

			if totalRG >= 100 {
				break
			}
		}

		if totalRG < 100 {
			break
		}

		batch := parquetFiles[start:end]

		logger.Infof(
			"ticker=%s compacting files=%d row_groups=%d",
			ticker,
			len(batch),
			totalRG,
		)

		if err := CompactBatch(
			tickerDir,
			compactingDir,
			batch,
		); err != nil {
			return err
		}

		start = end
	}

	return nil
}

func CompactBatch(
	tickerDir string,
	compactingDir string,
	batch []ParquetFile,
) error {

	if len(batch) == 0 {
		return nil
	}

	if err := os.MkdirAll(
		compactingDir,
		0755,
	); err != nil {

		return fmt.Errorf(
			"create compacting dir %s: %w",
			compactingDir,
			err,
		)
	}

	oldest := batch[0]

	targetPath := filepath.Join(
		tickerDir,
		oldest.FileName,
	)

	tempOutput := targetPath + ".tmp"

	// --------------------------------------------------
	// STEP 3 - move parquet into .compacting
	// --------------------------------------------------

	for _, f := range batch {

		src := f.Path

		dst := filepath.Join(
			compactingDir,
			f.FileName,
		)

		if err := os.Rename(
			src,
			dst,
		); err != nil {

			return fmt.Errorf(
				"move parquet into compacting %s -> %s: %w",
				src,
				dst,
				err,
			)
		}
	}

	// --------------------------------------------------
	// STEP 4 - create merged parquet
	// --------------------------------------------------

	outFile, err := os.Create(tempOutput)

	if err != nil {
		return fmt.Errorf(
			"create compacted parquet %s: %w",
			tempOutput,
			err,
		)
	}

	writer := parquet.NewGenericWriter[model.ParquetRow](outFile)

	for _, f := range batch {

		path := filepath.Join(
			compactingDir,
			f.FileName,
		)

		if err := CopyParquetRows(
			path,
			writer,
		); err != nil {

			writer.Close()
			outFile.Close()

			return err
		}
	}

	if err := writer.Close(); err != nil {

		outFile.Close()

		return fmt.Errorf(
			"close compacted parquet writer %s: %w",
			tempOutput,
			err,
		)
	}

	if err := outFile.Close(); err != nil {

		return fmt.Errorf(
			"close compacted parquet file %s: %w",
			tempOutput,
			err,
		)
	}

	// --------------------------------------------------
	// STEP 5 - atomic rename
	// --------------------------------------------------

	if err := os.Rename(
		tempOutput,
		targetPath,
	); err != nil {

		return fmt.Errorf(
			"rename compacted parquet %s -> %s: %w",
			tempOutput,
			targetPath,
			err,
		)
	}

	// --------------------------------------------------
	// STEP 6 - delete compacting files
	// --------------------------------------------------

	if err := os.RemoveAll(
		compactingDir,
	); err != nil {

		return fmt.Errorf(
			"remove compacting dir %s: %w",
			compactingDir,
			err,
		)
	}

	return nil
}

func RecoverCompaction(
	tickerDir string,
	compactingDir string,
) error {

	info, err := os.Stat(compactingDir)

	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf(
			"stat compacting dir %s: %w",
			compactingDir,
			err,
		)
	}

	if !info.IsDir() {
		return fmt.Errorf(
			"compacting path is not directory: %s",
			compactingDir,
		)
	}

	logger.Infof(
		"recovering compacting dir=%s",
		compactingDir,
	)

	files, err := filepath.Glob(
		filepath.Join(
			compactingDir,
			"*.parquet",
		),
	)

	if err != nil {
		return err
	}

	for _, f := range files {

		base := filepath.Base(f)

		mainParquet := filepath.Join(
			tickerDir,
			base,
		)

		_ = os.Remove(mainParquet)
		_ = os.Remove(mainParquet + ".tmp")
	}

	return nil
}

func GetRowGroupCount(
	path string,
) (int, error) {

	file, err := os.Open(path)

	if err != nil {
		return 0, fmt.Errorf(
			"open parquet %s: %w",
			path,
			err,
		)
	}

	defer file.Close()

	reader := parquet.NewGenericReader[any](file)

	return len(reader.File().RowGroups()), nil
}

func CopyParquetRows(
	path string,
	writer *parquet.GenericWriter[model.ParquetRow],
) error {

	file, err := os.Open(path)

	if err != nil {
		return fmt.Errorf(
			"open parquet for merge %s: %w",
			path,
			err,
		)
	}

	defer file.Close()

	reader := parquet.NewGenericReader[model.ParquetRow](file)

	rows := make([]model.ParquetRow, 10000)

	for {

		n, err := reader.Read(rows)

		if n > 0 {

			if _, werr := writer.Write(rows[:n]); werr != nil {

				return fmt.Errorf(
					"write merged parquet rows from %s: %w",
					path,
					werr,
				)
			}
		}

		if err != nil {

			if strings.Contains(
				err.Error(),
				"EOF",
			) {
				break
			}

			return fmt.Errorf(
				"read parquet rows %s: %w",
				path,
				err,
			)
		}
	}

	return nil
}
