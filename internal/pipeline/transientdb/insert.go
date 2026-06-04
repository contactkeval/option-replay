package transientdb

import (
	"database/sql"
	"fmt"
)

func InsertBars(
	tx *sql.Tx,
	expiry string,
	bars []TransientRow,
) error {
	table := tableName(expiry)

	query := fmt.Sprintf(`
    INSERT OR IGNORE INTO %s (
        ticker,
        strike,
        option_type,
        window_start,
        open,
        high,
        low,
        close,
        volume
    )
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, table)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, bar := range bars {
		_, err := stmt.Exec(
			bar.Ticker,
			bar.Strike,
			bar.OptionType,
			bar.WindowStart,
			bar.Open,
			bar.High,
			bar.Low,
			bar.Close,
			bar.Volume,
		)

		if err != nil {
			return err
		}
	}

	return nil
}
