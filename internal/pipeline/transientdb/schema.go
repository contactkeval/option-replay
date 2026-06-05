package transientdb

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

type TransientRow struct {
	Ticker string

	model.ParquetRow
}

func tableName(expiry string) string {
	expiry = strings.ReplaceAll(expiry, "-", "")
	return fmt.Sprintf("options_%s", expiry)
}

func EnsureExpiryTable(
	tx *sql.Tx,
	expiry string,
) error {
	table := tableName(expiry)

	query := fmt.Sprintf(`
    CREATE TABLE IF NOT EXISTS %s (
        ticker       TEXT NOT NULL,
        strike       INTEGER NOT NULL,
        option_type  INTEGER NOT NULL,
        window_start INTEGER NOT NULL,

        open  INTEGER NOT NULL,
        high  INTEGER NOT NULL,
        low   INTEGER NOT NULL,
        close INTEGER NOT NULL,

        volume       INTEGER NOT NULL,
        transactions INTEGER NOT NULL,

        PRIMARY KEY (
            ticker,
            strike,
            option_type,
            window_start
        )
    );
    `, table)

	_, err := tx.Exec(query)
	if err != nil {
		return err
	}

	return createIndexes(tx, table)
}

func createIndexes(tx *sql.Tx, table string) error {
	queries := []string{
		fmt.Sprintf(`
        CREATE INDEX IF NOT EXISTS idx_%s_ticker
        ON %s(ticker);
        `, table, table),

		fmt.Sprintf(`
        CREATE INDEX IF NOT EXISTS idx_%s_scan
        ON %s(
            ticker,
            strike,
            option_type,
            window_start
        );
        `, table, table),
	}

	for _, q := range queries {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}

	return nil
}
