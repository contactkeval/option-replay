package stage2_dxfeeddatadownloader

import (
	"fmt"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
)

func WeekOfMonth(t time.Time) int {
	return ((t.Day() - 1) / 7) + 1
}

func GetWeekdayContracts(database *db.DB) ([]db.Contract, error) {
	expiries, err := database.GetDistinctExpiries()
	if err != nil {
		return nil, err
	}

	var selected []string

	for i := 0; i < len(expiries); i += 4 {
		selected = append(
			selected,
			expiries[i].Format("2006-01-02"),
		)

		if len(selected) >= 4 {
			break
		}
	}

	if len(selected) == 0 {
		return nil, nil
	}

	contracts, err := database.GetContractsByExpiries(selected)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Selected expiries: %v\n", selected)

	return contracts, nil
}

func GetWeekendContracts(database *db.DB) ([]db.Contract, error) {
	now := time.Now()
	week := WeekOfMonth(now)

	if week > 4 {
		fmt.Println("Week 5 detected. No download scheduled.")
		return nil, nil
	}

	groupNo := week - 1

	contracts, err := database.GetContractsByGroupNo(groupNo)
	if err != nil {
		return nil, err
	}

	fmt.Printf(
		"Weekend group=%d contracts=%d\n",
		groupNo,
		len(contracts),
	)

	return contracts, nil
}
