package stage3_parquet

import (
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func SelectEligibleMetadataRows(
	rows []config.ActiveMetadataRow,
	maxRowsPerRowGroup int,
	maxShortRows int,
) []config.ActiveMetadataRow {

	if len(rows) == 0 {
		return nil
	}

	selected := []config.ActiveMetadataRow{
		rows[0],
	}

	pendingRows := rows[0].RowCount

	for i := 1; i < len(rows); i++ {
		nextRows := rows[i].RowCount

		capacity :=
			((pendingRows + maxRowsPerRowGroup - 1) /
				maxRowsPerRowGroup) *
				maxRowsPerRowGroup

		if pendingRows+nextRows <= capacity {
			selected = append(selected, rows[i])
			pendingRows += nextRows
			continue
		}

		shortfall := capacity - pendingRows

		if shortfall <= maxShortRows {
			return selected
		}

		selected = append(selected, rows[i])
		pendingRows += nextRows
	}

	return nil
}

func GroupMetadataRowsByTicker(
	rows []config.ActiveMetadataRow,
) map[string][]config.ActiveMetadataRow {

	result := make(map[string][]config.ActiveMetadataRow)

	for _, row := range rows {
		result[row.Ticker] = append(result[row.Ticker], row)
	}

	return result
}
