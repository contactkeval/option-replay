package stage3_sort_dedupe

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func LoadRows(path string) ([]Stage3Row, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var rows []Stage3Row

	for scanner.Scan() {

		line := scanner.Text()
		parts := strings.Split(line, ",")
		strike, _ := strconv.ParseUint(parts[0], 10, 32)
		optionType := parts[1] == "C"
		windowStartNs, _ := strconv.ParseUint(parts[2], 10, 64)
		windowStartSec := uint32(windowStartNs / 1_000_000_000)

		openFloat, _ := strconv.ParseFloat(parts[3], 64)
		highFloat, _ := strconv.ParseFloat(parts[4], 64)
		lowFloat, _ := strconv.ParseFloat(parts[5], 64)
		closeFloat, _ := strconv.ParseFloat(parts[6], 64)

		volume, _ := strconv.ParseUint(parts[7], 10, 32)
		transactions, _ := strconv.ParseUint(parts[8], 10, 32)

		rows = append(rows, Stage3Row{
			Strike:      uint32(strike),
			OptionType:  optionType,
			WindowStart: windowStartSec,

			Open:  uint32(openFloat * 100),
			High:  uint32(highFloat * 100),
			Low:   uint32(lowFloat * 100),
			Close: uint32(closeFloat * 100),

			Volume:       uint32(volume),
			Transactions: uint32(transactions),
		})
	}

	return rows, scanner.Err()
}
