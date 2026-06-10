package stage3_parquet

import (
	"database/sql"
	"fmt"
	"time"
)

func DeleteProcessedTickerRows(
	tx *sql.Tx,
	ticker string,
	expiryDate time.Time,
) error {

	tableName := fmt.Sprintf(
		"options_%s",
		expiryDate.Format("20060102"),
	)

	query := fmt.Sprintf(`
        DELETE FROM %s
        WHERE ticker = ?
    `, tableName)

	_, err := tx.Exec(
		query,
		ticker,
	)

	if err != nil {
		return err
	}

	return DropTableIfEmpty(
		tx,
		tableName,
	)
}

func DropTableIfEmpty(
	tx *sql.Tx,
	tableName string,
) error {

	query := fmt.Sprintf(`
        SELECT COUNT(*)
        FROM %s
    `, tableName)

	var count int

	err := tx.QueryRow(
		query,
	).Scan(&count)

	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	dropQuery := fmt.Sprintf(`
        DROP TABLE %s
    `, tableName)

	_, err = tx.Exec(
		dropQuery,
	)

	return err
}
