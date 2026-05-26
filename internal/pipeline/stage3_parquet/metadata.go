package stage3_parquet

import (
	"encoding/json"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadMetadata(path string) (model.TickerMetadata, error) {

	var meta model.TickerMetadata

	data, err := os.ReadFile(path)
	if err != nil {

		if os.IsNotExist(err) {
			return meta, nil
		}

		return meta, err
	}

	err = json.Unmarshal(data, &meta)

	return meta, err
}

func SaveMetadata(
	path string,
	meta model.TickerMetadata,
) error {

	data, err := json.MarshalIndent(
		meta,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	return os.WriteFile(
		path,
		data,
		0644,
	)
}
