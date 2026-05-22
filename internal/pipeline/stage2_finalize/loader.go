package stage2_finalize

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func LoadRows(path string) ([]CsvRow, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var rows []CsvRow

	for scanner.Scan() {

		line := scanner.Text()

		parts := strings.Split(line, ",")

		windowStart, _ := strconv.ParseUint(parts[2], 10, 64)

		openVal, _ := strconv.ParseFloat(parts[3], 64)
		highVal, _ := strconv.ParseFloat(parts[4], 64)
		lowVal, _ := strconv.ParseFloat(parts[5], 64)
		closeVal, _ := strconv.ParseFloat(parts[6], 64)

		volume, _ := strconv.ParseUint(parts[7], 10, 32)
		transactions, _ := strconv.ParseUint(parts[8], 10, 32)

		rows = append(rows, CsvRow{
			Strike:      parts[0],
			OptionType:  parts[1],
			WindowStart: windowStart,

			Open:  openVal,
			High:  highVal,
			Low:   lowVal,
			Close: closeVal,

			Volume:       uint32(volume),
			Transactions: uint32(transactions),
		})
	}

	return rows, scanner.Err()
}
