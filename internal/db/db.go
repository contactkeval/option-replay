package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Schema uint

const (
	SchemaContracts Schema = 1 << iota
	SchemaDownload
	SchemaParquet
	SchemaTransient
	SchemaOCC

	SchemaMetadata = SchemaContracts | SchemaDownload | SchemaParquet | SchemaOCC
)

type Options struct {
	Path    string
	Schemas Schema
}

type DB struct {
	*sql.DB
}

func Open(opts Options) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := configure(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}

	if err := ensureSchemas(sqlDB, opts.Schemas); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ensure schemas: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

func configure(db *sql.DB) error {
	// One connection avoids SQLITE_BUSY from the sql pool opening
	// multiple writers against the same SQLite file.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
		`PRAGMA temp_store=MEMORY;`,
		`PRAGMA foreign_keys=ON;`,
		`PRAGMA busy_timeout=30000;`,
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("set pragma %s: %w", pragma, err)
		}
	}

	return nil
}

func (db *DB) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}
