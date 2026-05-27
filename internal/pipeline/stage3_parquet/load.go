package stage3_parquet

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadRows(path string) ([]model.ParquetRow, error) {
	file, err := os.Open(path)
	if err != nil {

		return nil, fmt.Errorf(
			"open csv file %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(
		buf,
		10*1024*1024,
	)

	var rows []model.ParquetRow
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) != 9 {

			return nil, fmt.Errorf(
				"invalid column count file=%s line=%d columns=%d",
				path,
				lineNumber,
				len(parts),
			)
		}

		strike, err := strconv.ParseUint(
			parts[0],
			10,
			32,
		)
		if err != nil {

			return nil, fmt.Errorf(
				"parse strike file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[0],
				err,
			)
		}

		optionType := false
		if parts[1] == "C" {
			optionType = true
		}

		windowStart, err := strconv.ParseUint(
			parts[2],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse window_start file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[2],
				err,
			)
		}

		openVal, err := strconv.ParseUint(
			parts[3],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse open file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[3],
				err,
			)
		}

		highVal, err := strconv.ParseUint(
			parts[4],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse high file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[4],
				err,
			)
		}

		lowVal, err := strconv.ParseUint(
			parts[5],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse low file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[5],
				err,
			)
		}

		closeVal, err := strconv.ParseUint(
			parts[6],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse close file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[6],
				err,
			)
		}

		volume, err := strconv.ParseUint(
			parts[7],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse volume file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[7],
				err,
			)
		}

		transactions, err := strconv.ParseUint(
			parts[8],
			10,
			32,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"parse transactions file=%s line=%d value=%s: %w",
				path,
				lineNumber,
				parts[8],
				err,
			)
		}

		rows = append(rows, model.ParquetRow{
			Strike:      uint32(strike),
			OptionType:  optionType,
			WindowStart: uint32(windowStart),

			Open:  uint32(openVal),
			High:  uint32(highVal),
			Low:   uint32(lowVal),
			Close: uint32(closeVal),

			Volume:       uint32(volume),
			Transactions: uint32(transactions),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"scan csv file %s: %w",
			path,
			err,
		)
	}

	return rows, nil
}

func CountRows(path string) (int, error) {

	file, err := os.Open(path)

	if err != nil {
		return 0, fmt.Errorf(
			"open csv file %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf(
			"scan csv file %s: %w",
			path,
			err,
		)
	}

	return count, nil
}
