package stage3_parquet

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func DiscoverExpiredTables(
	transientDB *sql.DB,
	metadataDB *sql.DB,
	today time.Time,
) error {

	tables, err := ListExpiryTables(
		transientDB,
	)

	if err != nil {
		return err
	}

	for _, table := range tables {

		expiryString := strings.TrimPrefix(
			table,
			"options_",
		)

		expiryDate, err := time.Parse(
			"20060102",
			expiryString,
		)

		if err != nil {
			return err
		}

		if !expiryDate.Before(today) {
			continue
		}

		query := fmt.Sprintf(`
		SELECT
			ticker,
			COUNT(*)
		FROM %s
		GROUP BY ticker
		`, table)

		rows, err := transientDB.Query(query)
		if err != nil {
			return err
		}

		for rows.Next() {

			var ticker string
			var rowCount int

			err := rows.Scan(
				&ticker,
				&rowCount,
			)

			if err != nil {
				rows.Close()
				return err
			}

			err = InsertMetadataRow(
				metadataDB,
				model.ActiveMetadataRow{
					Ticker: ticker,

					ExpiryDate: expiryDate,

					RowCount: rowCount,

					Status: "created",
				},
			)

			if err != nil {
				rows.Close()
				return err
			}
		}

		rows.Close()
	}

	return nil
}

func ListExpiryTables(
	db *sql.DB,
) ([]string, error) {

	query := `
	SELECT name
	FROM sqlite_master
	WHERE type='table'
	AND name LIKE 'options_%'
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string

	for rows.Next() {

		var table string

		err := rows.Scan(&table)
		if err != nil {
			return nil, err
		}

		tables = append(tables, table)
	}

	return tables, rows.Err()
}
