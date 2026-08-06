package db

import (
	"database/sql"
	"fmt"
)

func ensureSchemas(db *sql.DB, schemas Schema) error {
	if schemas&SchemaContracts != 0 {
		if err := ensureContractsTables(db); err != nil {
			return err
		}
	}

	if schemas&SchemaDownload != 0 {
		if err := ensureDownloadTables(db); err != nil {
			return err
		}
	}

	if schemas&SchemaParquet != 0 {
		if err := ensureParquetTables(db); err != nil {
			return err
		}
	}

	if schemas&SchemaTransient != 0 {
		if err := ensureTransientTables(db); err != nil {
			return err
		}
	}

	if schemas&SchemaOCC != 0 {
		if err := ensureOCCTables(db); err != nil {
			return err
		}
	}

	return nil
}

func execStatements(db *sql.DB, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func ensureContractsTables(db *sql.DB) error {
	return execStatements(db, []string{`
		CREATE TABLE IF NOT EXISTS contracts (
			serialNo INTEGER PRIMARY KEY AUTOINCREMENT,

			underlying TEXT NOT NULL,
			expiry TEXT NOT NULL,
			type TEXT NOT NULL,
			strike REAL NOT NULL,

			groupNo INTEGER NOT NULL,

			firstSeenDate TEXT NOT NULL,
			lastDownloadedDate TEXT,

			downloadAttempts INTEGER NOT NULL DEFAULT 0,

			UNIQUE (
				underlying,
				expiry,
				type,
				strike
			)
		)
	`})
}

func ensureDownloadTables(db *sql.DB) error {
	return execStatements(db, []string{
		`
		CREATE TABLE IF NOT EXISTS runs (
			runNo INTEGER PRIMARY KEY AUTOINCREMENT,

			groupNo INTEGER NOT NULL,

			runDateTime TEXT NOT NULL,

			contractCount INTEGER NOT NULL,

			batchCount INTEGER NOT NULL
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS batches (
			runNo INTEGER NOT NULL,

			batchNo INTEGER NOT NULL,

			startTime TEXT,
			endTime TEXT,

			contractCount INTEGER,

			candleCount INTEGER,

			PRIMARY KEY (
				runNo,
				batchNo
			)
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS batch_contracts (
			runNo INTEGER NOT NULL,

			batchNo INTEGER NOT NULL,

			serialNo INTEGER NOT NULL,

			listNo INTEGER NOT NULL,

			PRIMARY KEY (
				runNo,
				batchNo,
				serialNo
			)
		)
		`,
		`CREATE INDEX IF NOT EXISTS idx_batch_contracts_run_batch
			ON batch_contracts(runNo, batchNo)`,
		`
		CREATE TABLE IF NOT EXISTS candle_staging (
			serialNo INTEGER NOT NULL,

			candleTime INTEGER NOT NULL,

			open REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			close REAL NOT NULL,

			volume REAL,

			runNo INTEGER NOT NULL,
			batchNo INTEGER NOT NULL,

			PRIMARY KEY (
				serialNo,
				candleTime
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_candle_staging_run_batch
			ON candle_staging(runNo, batchNo)`,
	})
}

func ensureParquetTables(db *sql.DB) error {
	return execStatements(db, []string{
		`
		CREATE TABLE IF NOT EXISTS active_metadata (
			ticker TEXT NOT NULL,
			expiry_date DATE NOT NULL,

			row_count INTEGER NOT NULL,

			status TEXT NOT NULL,

			parquet_path TEXT,
			row_group_count INTEGER,

			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

			PRIMARY KEY (
				ticker,
				expiry_date
			)
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS archive_metadata (
			ticker TEXT NOT NULL,
			expiry_date DATE NOT NULL,

			row_count INTEGER NOT NULL,

			parquet_path TEXT NOT NULL,

			start_row_group INTEGER NOT NULL,
			row_group_count INTEGER NOT NULL,

			created_at DATETIME NOT NULL,
			archived_at DATETIME DEFAULT CURRENT_TIMESTAMP,

			PRIMARY KEY (
				ticker,
				expiry_date
			)
		)
		`,
	})
}

func ensureTransientTables(db *sql.DB) error {
	return execStatements(db, []string{`
		CREATE TABLE IF NOT EXISTS rejected_rows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			raw_line TEXT NOT NULL,

			reason TEXT NOT NULL,

			source_file TEXT NOT NULL,

			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`})
}

func ensureOCCTables(db *sql.DB) error {
	if err := execStatements(db, []string{`
		CREATE TABLE IF NOT EXISTS occ_imports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			file_name TEXT NOT NULL,
			file_date TEXT,
			download_type TEXT,

			started_at TEXT NOT NULL,
			ended_at TEXT,

			records_read INTEGER NOT NULL DEFAULT 0,
			processed INTEGER NOT NULL DEFAULT 0,
			ignored INTEGER NOT NULL DEFAULT 0,
			inserted INTEGER NOT NULL DEFAULT 0,
			existing INTEGER NOT NULL DEFAULT 0,
			deleted INTEGER NOT NULL DEFAULT 0,
			updated INTEGER NOT NULL DEFAULT 0,
			skipped INTEGER NOT NULL DEFAULT 0,
			errors INTEGER NOT NULL DEFAULT 0,

			status TEXT NOT NULL
		)
	`}); err != nil {
		return err
	}

	// Additive columns for databases created before the enriched audit schema.
	for _, stmt := range []string{
		`ALTER TABLE occ_imports ADD COLUMN file_date TEXT`,
		`ALTER TABLE occ_imports ADD COLUMN download_type TEXT`,
		`ALTER TABLE occ_imports ADD COLUMN processed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE occ_imports ADD COLUMN ignored INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE occ_imports ADD COLUMN existing INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = db.Exec(stmt)
	}

	return nil
}

func expiryTableName(expiry string) string {
	table := expiry
	for i := 0; i < len(table); i++ {
		if table[i] == '-' {
			table = table[:i] + table[i+1:]
			i--
		}
	}
	return fmt.Sprintf("options_%s", table)
}
