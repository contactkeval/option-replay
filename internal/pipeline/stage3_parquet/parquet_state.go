package stage3_parquet

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadParquetState(
	path string,
) (model.ParquetState, error) {

	var state model.ParquetState

	data, err := os.ReadFile(path)

	if err != nil {

		if os.IsNotExist(err) {
			return state, nil
		}

		return state, fmt.Errorf(
			"read parquet state %s: %w",
			path,
			err,
		)
	}

	err = json.Unmarshal(data, &state)

	if err != nil {
		return state, fmt.Errorf(
			"unmarshal parquet state %s: %w",
			path,
			err,
		)
	}

	return state, nil
}

func SaveParquetState(
	path string,
	state model.ParquetState,
) error {

	data, err := json.MarshalIndent(
		state,
		"",
		"  ",
	)

	if err != nil {
		return fmt.Errorf(
			"marshal parquet state %s: %w",
			path,
			err,
		)
	}

	return os.WriteFile(
		path,
		data,
		0644,
	)
}
