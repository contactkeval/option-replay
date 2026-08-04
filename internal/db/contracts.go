package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (db *DB) InsertContractIgnore(
	tx *sql.Tx,
	underlying string,
	expiry time.Time,
	strike float64,
	contractType string,
	groupNo int,
	firstSeenDate time.Time,
) error {
	query := `
		INSERT OR IGNORE INTO contracts (
			underlying,
			expiry,
			strike,
			type,
			groupNo,
			firstSeenDate
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := tx.Exec(
		query,
		underlying,
		expiry.Format("2006-01-02"),
		strike,
		contractType,
		groupNo,
		firstSeenDate.Format("2006-01-02"),
	)

	return err
}

func (db *DB) GetDistinctExpiries() ([]time.Time, error) {
	rows, err := db.Query(`
		SELECT DISTINCT expiry
		FROM contracts
		WHERE expiry < date('now')
		ORDER BY expiry DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expiries []time.Time

	for rows.Next() {
		var expiry string

		if err := rows.Scan(&expiry); err != nil {
			return nil, err
		}

		t, err := time.Parse("2006-01-02", expiry)
		if err != nil {
			return nil, err
		}

		expiries = append(expiries, t)
	}

	return expiries, rows.Err()
}

func (db *DB) GetContractsByExpiries(
	expiries []string,
) ([]Contract, error) {
	if len(expiries) == 0 {
		return nil, nil
	}

	query := `
		SELECT
			serialNo,
			underlying,
			expiry,
			type,
			strike,
			groupNo
		FROM contracts
		WHERE expiry IN (
	`

	args := make([]any, 0, len(expiries))

	for i, expiry := range expiries {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, expiry)
	}

	query += ")"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) GetContractsByGroupNo(
	groupNo int,
) ([]Contract, error) {
	rows, err := db.Query(`
		SELECT
			serialNo,
			underlying,
			expiry,
			type,
			strike,
			groupNo
		FROM contracts
		WHERE
			expiry > date('now', '+1 month')
			AND groupNo = ?
	`, groupNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) GetBatchContracts(
	runNo int64,
	batchNo int,
) ([]Contract, error) {
	rows, err := db.Query(`
		SELECT
			c.serialNo,
			c.underlying,
			c.expiry,
			c.type,
			c.strike,
			c.groupNo
		FROM batch_contracts bc
		JOIN contracts c
			ON c.serialNo = bc.serialNo
		WHERE
			bc.runNo = ?
			AND bc.batchNo = ?
		ORDER BY bc.listNo
	`, runNo, batchNo)
	if err != nil {
		return nil, fmt.Errorf("query batch contracts: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

func (db *DB) CountContracts() (int, error) {
	var count int

	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM contracts
	`).Scan(&count)

	return count, err
}

func scanContracts(rows *sql.Rows) ([]Contract, error) {
	var contracts []Contract

	for rows.Next() {
		var c Contract
		var expiry string

		if err := rows.Scan(
			&c.SerialNo,
			&c.Underlying,
			&expiry,
			&c.Type,
			&c.Strike,
			&c.GroupNo,
		); err != nil {
			return nil, err
		}

		var err error
		c.Expiry, err = time.Parse("2006-01-02", expiry)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, c)
	}

	return contracts, rows.Err()
}
