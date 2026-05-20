package stage3_sort_dedupe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func RewriteFile(
	cfg config.Config,
	originalPath string,
	rows []Stage3Row,
	duplicates uint32,
) error {

	rel, err := filepath.Rel(
		cfg.Stage3Root,
		originalPath,
	)

	if err != nil {
		return err
	}

	tempPath := filepath.Join(
		cfg.TempRoot,
		"stage3",
		rel,
	)

	if err := os.MkdirAll(
		filepath.Dir(tempPath),
		0755,
	); err != nil {
		return err
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	writer := bufio.NewWriterSize(tempFile, 64*1024)

	for _, row := range rows {
		optionType := "P"
		if row.OptionType {
			optionType = "C"
		}

		line := fmt.Sprintf(
			"%08d,%s,%d,%d,%d,%d,%d,%d,%d\n",
			row.Strike,
			optionType,
			row.WindowStart,
			row.Open,
			row.High,
			row.Low,
			row.Close,
			row.Volume,
			row.Transactions,
		)

		_, err := writer.WriteString(line)
		if err != nil {
			return err
		}
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, originalPath); err != nil {
		return err
	}

	meta := FileMetadata{
		Rows:              uint32(len(rows)),
		DuplicatesRemoved: duplicates,
		ProcessedAtUnix:   time.Now().Unix(),
	}

	metaPath := originalPath + ".meta.json"
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, metaBytes, 0644)
}
