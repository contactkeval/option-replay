package transientdb

import "github.com/contactkeval/option-replay/internal/pipeline/model"

type TransientRow struct {
	Ticker string

	model.ParquetRow
}
