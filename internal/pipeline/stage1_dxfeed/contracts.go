package stage1_dxfeed

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	_ "modernc.org/sqlite"
)

func OpenContractsDB() (*sql.DB, error) {

	db, err := sql.Open(
		"sqlite",
		"contracts.db",
	)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS contracts (
			underlying TEXT NOT NULL,
			expiry     TEXT NOT NULL,
			strike     REAL NOT NULL,
			type       TEXT NOT NULL,
			PRIMARY KEY (
				underlying,
				expiry,
				strike,
				type
			)
		)
	`)

	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func LoadContractsToSQLite() error {

	db, err := OpenContractsDB()
	if err != nil {
		return err
	}
	defer db.Close()

	provider := data.NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))

	totalContracts := 0
	totalUnderlyings := 0

	fromDate := time.Now().AddDate(-1, 0, 0)
	toDate := time.Now()

	for underlying := range config.AllowedUnderlyings {

		totalUnderlyings++

		contracts, err := provider.GetContracts(
			underlying,
			0, // all strikes
			fromDate,
			toDate,
		)

		if err != nil {

			fmt.Printf(
				"FAILED %s: %v\n",
				underlying,
				err,
			)

			continue
		}

		fmt.Printf(
			"%-10s -> %6d contracts\n",
			underlying,
			len(contracts),
		)

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO contracts (
				underlying,
				expiry,
				strike,
				type
			)
			VALUES (?, ?, ?, ?)
		`)
		if err != nil {
			tx.Rollback()
			return err
		}

		for _, c := range contracts {

			_, err := stmt.Exec(
				underlying,
				c.ExpiryDate.Format("2006-01-02"),
				c.Strike,
				c.Type,
			)

			if err != nil {
				stmt.Close()
				tx.Rollback()
				return err
			}

			totalContracts++
		}

		stmt.Close()

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	var uniqueContracts int

	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
	`).Scan(&uniqueContracts)

	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("========== SUMMARY ==========")

	fmt.Printf(
		"Underlyings processed : %d\n",
		totalUnderlyings,
	)

	fmt.Printf(
		"Contracts returned    : %d\n",
		totalContracts,
	)

	fmt.Printf(
		"Unique contracts      : %d\n",
		uniqueContracts,
	)

	return nil
}
