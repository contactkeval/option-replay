package stage3_parquet

import "github.com/contactkeval/option-replay/internal/pipeline/model"

func GroupMetadataRowsByTicker(
	rows []model.ActiveMetadataRow,
) map[string][]model.ActiveMetadataRow {

	result := make(
		map[string][]model.ActiveMetadataRow,
	)

	for _, row := range rows {

		result[row.Ticker] = append(
			result[row.Ticker],
			row,
		)
	}

	return result
}
