package transientdb

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := configure(db); err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}

	_, err = db.Exec(`PRAGMA journal_mode=WAL;`)
	if err != nil {
		return nil, fmt.Errorf("set journal mode: %w", err)
	}
	_, err = db.Exec(`PRAGMA busy_timeout=5000;`)
	if err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	return db, nil
}

func configure(db *sql.DB) error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
		`PRAGMA temp_store=MEMORY;`,
		`PRAGMA foreign_keys=ON;`,
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("set pragma %s: %w", p, err)
		}
	}

	return nil
}
