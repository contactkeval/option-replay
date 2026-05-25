package stage3_parquet

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func LoadRows(path string) ([]ParquetRow, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var rows []ParquetRow

	for scanner.Scan() {

		line := scanner.Text()
		parts := strings.Split(line, ",")

		strike, _ := strconv.ParseUint(parts[0], 10, 32)

		optionType := false
		if parts[1] == "C" {
			optionType = true
		}

		windowStart, _ := strconv.ParseUint(parts[2], 10, 32)

		openVal, _ := strconv.ParseUint(parts[3], 10, 32)
		highVal, _ := strconv.ParseUint(parts[4], 10, 32)
		lowVal, _ := strconv.ParseUint(parts[5], 10, 32)
		closeVal, _ := strconv.ParseUint(parts[6], 10, 32)

		volume, _ := strconv.ParseUint(parts[7], 10, 32)
		transactions, _ := strconv.ParseUint(parts[8], 10, 32)

		rows = append(rows, ParquetRow{
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

	return rows, scanner.Err()
}
