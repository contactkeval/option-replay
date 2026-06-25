package stage2_dxfeeddatadownloader

import (
	"fmt"
	"time"
)

func (m *MetadataDB) GetDistinctExpiries() ([]time.Time, error) {

	rows, err := m.db.Query(`
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

		t, err := time.Parse(
			"2006-01-02",
			expiry,
		)
		if err != nil {
			return nil, err
		}

		expiries = append(
			expiries,
			t,
		)
	}

	return expiries, nil
}

func WeekOfMonth(
	t time.Time,
) int {

	return ((t.Day() - 1) / 7) + 1
}

func (m *MetadataDB) GetWeekdayContracts() (
	[]Contract,
	error,
) {

	expiries, err :=
		m.GetDistinctExpiries()

	if err != nil {
		return nil, err
	}

	var selected []string

	for i := 0; i < len(expiries); i += 4 {

		selected = append(
			selected,
			expiries[i].Format(
				"2006-01-02",
			),
		)

		if len(selected) >= 4 {
			break
		}
	}

	if len(selected) == 0 {
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

	args := make([]any, 0, len(selected))

	for i := range selected {

		if i > 0 {
			query += ","
		}

		query += "?"

		args = append(
			args,
			selected[i],
		)
	}

	query += ")"

	rows, err :=
		m.db.Query(
			query,
			args...,
		)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

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

		c.Expiry, _ =
			time.Parse(
				"2006-01-02",
				expiry,
			)

		contracts =
			append(
				contracts,
				c,
			)
	}

	fmt.Printf(
		"Selected expiries: %v\n",
		selected,
	)

	return contracts, nil
}

func (m *MetadataDB) GetWeekendContracts() (
	[]Contract,
	error,
) {

	now := time.Now()

	week :=
		WeekOfMonth(now)

	if week > 4 {

		fmt.Println(
			"Week 5 detected. No download scheduled.",
		)

		return nil, nil
	}

	groupNo :=
		week - 1

	rows, err := m.db.Query(`
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
	`,
		groupNo,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

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

		c.Expiry, _ =
			time.Parse(
				"2006-01-02",
				expiry,
			)

		contracts =
			append(
				contracts,
				c,
			)
	}

	fmt.Printf(
		"Weekend group=%d contracts=%d\n",
		groupNo,
		len(contracts),
	)

	return contracts, nil
}
