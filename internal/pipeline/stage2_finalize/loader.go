package stage2_finalize

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func LoadRows(path string) ([]model.CsvRow, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var rows []model.CsvRow

	for scanner.Scan() {

		line := scanner.Text()
		parts := strings.Split(line, ",")

		if len(parts) < 9 {

			logger.Errorf(
				"invalid csv row file=%s len=%d row=%q",
				path,
				len(parts),
				line,
			)

			continue
		}

		windowStart, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {

			logger.Errorf(
				"invalid timestamp file=%s value=%q row=%q",
				path,
				parts[2],
				line,
			)

			continue
		}
		openVal, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			logger.Errorf(
				"invalid open value file=%s value=%q row=%q",
				path,
				parts[3],
				line,
			)
			continue
		}
		highVal, err := strconv.ParseFloat(parts[4], 64)
		if err != nil {
			logger.Errorf(
				"invalid high value file=%s value=%q row=%q",
				path,
				parts[4],
				line,
			)
			continue
		}
		lowVal, err := strconv.ParseFloat(parts[5], 64)
		if err != nil {
			logger.Errorf(
				"invalid low value file=%s value=%q row=%q",
				path,
				parts[5],
				line,
			)
			continue
		}
		closeVal, err := strconv.ParseFloat(parts[6], 64)
		if err != nil {
			logger.Errorf(
				"invalid close value file=%s value=%q row=%q",
				path,
				parts[6],
				line,
			)
			continue
		}

		volume, _ := strconv.ParseUint(parts[7], 10, 32)
		transactions, _ := strconv.ParseUint(parts[8], 10, 32)

		rows = append(rows, model.CsvRow{
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
