package stage3_parquet

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadActiveMetadata(
	path string,
) ([]model.ActiveMetadataRow, error) {

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf(
			"open active metadata %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf(
			"read active metadata %s: %w",
			path,
			err,
		)
	}

	var rows []model.ActiveMetadataRow

	for i, r := range records {

		if i == 0 {
			continue
		}

		rowCount, _ := strconv.Atoi(r[1])

		rows = append(rows, model.ActiveMetadataRow{
			Expiry: r[0],
			Rows:   rowCount,
			Status: r[2],
		})
	}

	return rows, nil
}

func SaveActiveMetadata(
	path string,
	rows []model.ActiveMetadataRow,
) error {

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf(
			"create active metadata %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"expiry",
		"rows",
		"status",
	}); err != nil {
		return err
	}

	for _, r := range rows {

		if err := writer.Write([]string{
			r.Expiry,
			strconv.Itoa(r.Rows),
			r.Status,
		}); err != nil {
			return err
		}
	}

	return nil
}
