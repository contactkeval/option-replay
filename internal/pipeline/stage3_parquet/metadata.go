package stage3_parquet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadMetadata(path string) (model.TickerMetadata, error) {
	var meta model.TickerMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return meta, nil
		}

		return meta, fmt.Errorf(
			"read metadata file %s: %w",
			path,
			err,
		)
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf(
			"unmarshal metadata file %s: %w",
			path,
			err,
		)
	}

	return meta, nil
}

func SaveMetadata(
	path string,
	meta model.TickerMetadata,
) error {

	if err := os.MkdirAll(
		filepath.Dir(path),
		0755,
	); err != nil {
		return fmt.Errorf(
			"create metadata directory %s: %w",
			filepath.Dir(path),
			err,
		)
	}

	data, err := json.MarshalIndent(
		meta,
		"",
		"  ",
	)

	if err != nil {
		return fmt.Errorf(
			"marshal metadata file %s: %w",
			path,
			err,
		)
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(
		tempPath,
		data,
		0644,
	); err != nil {

		return fmt.Errorf(
			"write temp metadata file %s: %w",
			tempPath,
			err,
		)
	}

	if err := os.Rename(
		tempPath,
		path,
	); err != nil {

		return fmt.Errorf(
			"rename metadata temp file %s -> %s: %w",
			tempPath,
			path,
			err,
		)
	}

	return nil
}
