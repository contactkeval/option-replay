package stage2_finalize

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/contactkeval/option-replay/internal/pipeline/transientdb"
)

func Run(cfg config.Config) error {

	stage2Root := cfg.Stage2Root

	// today := time.Now()
	today := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	logger.Infof("Stage 2 processing started")

	transientDBPath := filepath.Join(
		cfg.SQLiteRoot,
		"transient.db",
	)

	db, err := transientdb.Open(transientDBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return filepath.Walk(
		stage2Root,
		func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".csv") {
				return nil
			}

			base := filepath.Base(path)
			parts := strings.Split(base, "_")

			if len(parts) < 2 {
				return nil
			}

			ticker := parts[0]

			expiry := parts[1]
			expiry = strings.TrimSuffix(
				expiry,
				".csv",
			)

			expiryTime, err := time.Parse(
				"060102",
				expiry,
			)
			if err != nil {
				return nil
			}

			// only finalized expiries
			if !expiryTime.Before(today) {
				return nil
			}

			rows, err := LoadRows(path)
			if err != nil {
				return err
			}

			optimizedRows := ConvertRows(rows)

			// sorting no longer required
			// SQLite index ordering replaces it

			finalRows := optimizedRows

			expiryString := expiryTime.Format(
				"2006-01-02",
			)

			err = transientdb.EnsureExpiryTable(
				db,
				expiryString,
			)
			if err != nil {
				return err
			}

			bars := make(
				[]transientdb.TransientRow,
				0,
				len(finalRows),
			)

			for _, row := range finalRows {

				bar := transientdb.TransientRow{
					Ticker: ticker,

					ParquetRow: model.ParquetRow{
						Strike: row.Strike,

						OptionType: row.OptionType,

						WindowStart: row.WindowStart,

						Open:  row.Open,
						High:  row.High,
						Low:   row.Low,
						Close: row.Close,

						Volume: row.Volume,

						Transactions: row.Transactions,
					},
				}

				bars = append(bars, bar)
			}

			tx, err := db.Begin()
			if err != nil {
				return err
			}

			err = transientdb.InsertBars(
				tx,
				expiryString,
				bars,
			)

			if err != nil {
				tx.Rollback()
				return err
			}

			if err := tx.Commit(); err != nil {
				return err
			}

			logger.Infof(
				"rows=%d inserted expiry=%s ticker=%s",
				len(finalRows),
				expiryString,
				ticker,
			)

			return nil
		},
	)
}
