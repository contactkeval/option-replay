package stage3_parquet

import (
	"encoding/json"
	"os"
)

func LoadMetadata(path string) (TickerMetadata, error) {

	var meta TickerMetadata

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
	meta TickerMetadata,
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
