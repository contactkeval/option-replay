package stage2_dxfeeddatadownloader

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type MetadataDB struct {
	db *sql.DB
}

func OpenMetadataDB(
	path string,
) (*MetadataDB, error) {

	db, err := sql.Open(
		"sqlite",
		path,
	)
	if err != nil {
		return nil, err
	}

	m := &MetadataDB{
		db: db,
	}

	if err := m.createTables(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *MetadataDB) Close() error {
	return m.db.Close()
}

func (m *MetadataDB) createTables() error {

	stmts := []string{

		`
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
		`,

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
	}

	for _, stmt := range stmts {

		if _, err := m.db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
