package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

// One-time tool: migrates candle_staging rows from metadata.db into the
// transient DB's options_YYYYMMDD tables.
//
//	go run ./cmd/migrate-cst
//	go run ./cmd/migrate-cst -metadata-db G:\data\metadata\metadata.db -transient-db G:\data\sqLite\transient.db
func main() {
	cfg := config.Load()

	metadataDBPath := flag.String(
		"metadata-db",
		filepath.Join(cfg.MetadataRoot, "metadata.db"),
		"path to metadata.db",
	)
	transientDBPath := flag.String(
		"transient-db",
		filepath.Join(cfg.SQLiteRoot, "transient.db"),
		"path to transient.db",
	)
	flag.Parse()

	start := time.Now()
	logger.Infof("migrate-cst: metadata=%s transient=%s", *metadataDBPath, *transientDBPath)

	metadataDB, err := db.Open(db.Options{
		Path:    *metadataDBPath,
		Schemas: db.SchemaMetadata,
	})
	if err != nil {
		logger.Fatalf("open metadata DB: %v", err)
	}
	defer metadataDB.Close()

	transientDB, err := db.Open(db.Options{
		Path:    *transientDBPath,
		Schemas: db.SchemaTransient,
	})
	if err != nil {
		logger.Fatalf("open transient DB: %v", err)
	}
	defer transientDB.Close()

	serialToContract, err := metadataDB.LoadAllContracts()
	if err != nil {
		logger.Fatalf("load contracts: %v", err)
	}
	logger.Infof("loaded %d contracts", len(serialToContract))

	if _, err := transientDB.Exec(`
		CREATE TABLE IF NOT EXISTS migrate_cst_progress (
			id         INTEGER PRIMARY KEY,
			serial_no  INTEGER NOT NULL,
			candle_time INTEGER NOT NULL
		)
	`); err != nil {
		logger.Fatalf("create progress table: %v", err)
	}

	var totalMigrated int64
	batchSize := 2000
	var lastSerial int64
	var lastTime int64
	hasLast := false

	var savedSerial sql.NullInt64
	var savedTime sql.NullInt64
	err = transientDB.QueryRow(`
		SELECT serial_no, candle_time
		FROM migrate_cst_progress
		WHERE id = 1
	`).Scan(&savedSerial, &savedTime)
	if err == nil && savedSerial.Valid && savedTime.Valid {
		lastSerial = savedSerial.Int64
		lastTime = savedTime.Int64
		hasLast = true
		logger.Infof("resuming from bookmark serial=%d candleTime=%d", lastSerial, lastTime)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Fatalf("read progress: %v", err)
	}

	for {
		query := `
			SELECT serialNo, candleTime, open, high, low, close, volume, runNo, batchNo
			FROM candle_staging
			ORDER BY serialNo, candleTime
			LIMIT ?
		`
		args := []any{batchSize}
		if hasLast {
			query = `
				SELECT serialNo, candleTime, open, high, low, close, volume, runNo, batchNo
				FROM candle_staging
				WHERE serialNo > ? OR (serialNo = ? AND candleTime > ?)
				ORDER BY serialNo, candleTime
				LIMIT ?
			`
			args = []any{lastSerial, lastSerial, lastTime, batchSize}
		}

		rows, err := metadataDB.Query(query, args...)
		if err != nil {
			logger.Fatalf("query candle_staging: %v", err)
		}

		var batch []db.CandleStagingRow
		for rows.Next() {
			var r db.CandleStagingRow
			var volume sql.NullFloat64
			if err := rows.Scan(
				&r.SerialNo,
				&r.Candle.Time,
				&r.Candle.Open,
				&r.Candle.High,
				&r.Candle.Low,
				&r.Candle.Close,
				&volume,
				&r.RunNo,
				&r.BatchNo,
			); err != nil {
				rows.Close()
				logger.Fatalf("scan candle_staging: %v", err)
			}
			r.Candle.Volume = config.DXFloat(volume.Float64)
			lastSerial = r.SerialNo
			lastTime = r.Candle.Time
			hasLast = true
			batch = append(batch, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			logger.Fatalf("iterate candle_staging: %v", err)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		n, err := transientDB.InsertCandlesToTransient(batch, serialToContract)
		if err != nil {
			logger.Fatalf("insert batch after serial=%d time=%d: %v", lastSerial, lastTime, err)
		}

		totalMigrated += n

		if _, err := transientDB.Exec(`
			INSERT OR REPLACE INTO migrate_cst_progress (id, serial_no, candle_time)
			VALUES (1, ?, ?)
		`, lastSerial, lastTime); err != nil {
			logger.Fatalf("save progress at serial=%d time=%d: %v", lastSerial, lastTime, err)
		}

		if totalMigrated%500000 < int64(len(batch)) {
			logger.Infof("  ... migrated %d rows so far (bookmark serial=%d time=%d)", totalMigrated, lastSerial, lastTime)
		}
	}

	elapsed := time.Since(start).Truncate(time.Millisecond)
	logger.Infof("migration complete: %d rows in %s", totalMigrated, elapsed)
	fmt.Printf("migrated %d rows from candle_staging to transient DB in %s\n", totalMigrated, elapsed)
}
