package stage3_parquet

import (
	"database/sql"
	"fmt"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadTickerRows(
	db *sql.DB,
	metadataRows []model.ActiveMetadataRow,
) ([]model.ParquetRow, error) {

	var result []model.ParquetRow

	for _, meta := range metadataRows {

		expiryString := meta.ExpiryDate.Format(
			"20060102",
		)

		table := fmt.Sprintf(
			"options_%s",
			expiryString,
		)

		query := fmt.Sprintf(`
		SELECT
			strike,
			option_type,
			window_start,
			open,
			high,
			low,
			close,
			volume,
			transactions
		FROM %s
		WHERE ticker = ?
		ORDER BY
			strike,
			option_type,
			window_start
		`, table)

		rows, err := db.Query(
			query,
			meta.Ticker,
		)

		if err != nil {
			return nil, err
		}

		for rows.Next() {

			var row model.ParquetRow

			err := rows.Scan(
				&row.Strike,
				&row.OptionType,
				&row.WindowStart,
				&row.Open,
				&row.High,
				&row.Low,
				&row.Close,
				&row.Volume,
				&row.Transactions,
			)

			if err != nil {
				rows.Close()
				return nil, err
			}

			result = append(
				result,
				row,
			)
		}

		rows.Close()
	}

	return result, nil
}

func LoadRowsFromSQLite(
	db *sql.DB,
	expiry string,
	ticker string,
) ([]model.ParquetRow, error) {

	table := fmt.Sprintf(
		"options_%s",
		expiry,
	)

	query := fmt.Sprintf(`
	SELECT
		strike,
		option_type,
		window_start,
		open,
		high,
		low,
		close,
		volume,
		transactions
	FROM %s
	WHERE ticker = ?
	ORDER BY
		strike,
		option_type,
		window_start
	`, table)

	rows, err := db.Query(
		query,
		ticker,
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.ParquetRow

	for rows.Next() {

		var r model.ParquetRow

		err := rows.Scan(
			&r.Strike,
			&r.OptionType,
			&r.WindowStart,
			&r.Open,
			&r.High,
			&r.Low,
			&r.Close,
			&r.Volume,
			&r.Transactions,
		)

		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}

	return result, rows.Err()
}
