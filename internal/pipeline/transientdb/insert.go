package transientdb

import (
	"database/sql"
	"fmt"

	"github.com/contactkeval/option-replay/internal/logger"
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
        volume,
		transactions
    )
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, table)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	ignored := 0
	for _, bar := range bars {
		result, err := stmt.Exec(
			bar.Ticker,
			bar.Strike,
			bar.OptionType,
			bar.WindowStart,
			bar.Open,
			bar.High,
			bar.Low,
			bar.Close,
			bar.Volume,
			bar.Transactions,
		)

		if err != nil {
			return fmt.Errorf("execute statement: %w", err)
		}
		affected, err := result.RowsAffected()

		if err != nil {
			return err
		}

		if affected == 1 {
			inserted++
		} else {
			ignored++
		}
	}

	if ignored > 0 {
		logger.Warnf("Expiry=%s: Inserted: %d, Ignored: %d", expiry, inserted, ignored)
	}
	return nil
}
