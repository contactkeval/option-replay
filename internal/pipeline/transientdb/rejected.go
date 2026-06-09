package transientdb

import "database/sql"

func EnsureRejectedRowsTable(
	db *sql.DB,
) error {

	query := `
	CREATE TABLE IF NOT EXISTS rejected_rows (
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		raw_line TEXT NOT NULL,

		reason TEXT NOT NULL,

		source_file TEXT NOT NULL,

		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(query)

	return err
}

func InsertRejectedRow(
	tx *sql.Tx,
	rawLine string,
	reason string,
	sourceFile string,
) error {

	query := `
	INSERT INTO rejected_rows (
		raw_line,
		reason,
		source_file
	)
	VALUES (?, ?, ?)
	`

	_, err := tx.Exec(
		query,
		rawLine,
		reason,
		sourceFile,
	)

	return err
}
