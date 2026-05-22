package stage2_finalize

import (
	"encoding/json"
	"os"
	"time"
)

type Metadata struct {
	Ticker string `json:"ticker"`
	Expiry string `json:"expiry"`

	Rows int `json:"rows"`

	DuplicatesRemoved int `json:"duplicates_removed"`

	MinWindowStart uint32 `json:"min_window_start"`
	MaxWindowStart uint32 `json:"max_window_start"`

	ProcessedAt string `json:"processed_at"`
}

func WriteMetadata(
	path string,
	meta Metadata,
) error {

	meta.ProcessedAt = time.Now().UTC().Format(time.RFC3339)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	return encoder.Encode(meta)
}
