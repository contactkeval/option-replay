package parquetbuilder

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ParseStagingFile(path string) ([]OptionRow, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	expiry, err := ExtractExpiryFromFilename(path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	// Larger line buffer
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var rows []OptionRow

	for scanner.Scan() {

		line := scanner.Text()

		parts := strings.Split(line, ",")

		if len(parts) != 9 {
			continue
		}

		strikeVal, _ := strconv.ParseInt(parts[0], 10, 64)
		optionType := false
		if (parts[1]) == "C" {
			optionType = true
		}
		windowStartVal, _ := strconv.ParseInt(parts[2], 10, 64)

		openVal, _ := strconv.ParseFloat(parts[3], 64)
		highVal, _ := strconv.ParseFloat(parts[4], 64)
		lowVal, _ := strconv.ParseFloat(parts[5], 64)
		closeVal, _ := strconv.ParseFloat(parts[6], 64)

		volume, _ := strconv.ParseInt(parts[7], 10, 64)
		transactions, _ := strconv.ParseInt(parts[8], 10, 32)

		strikeInt := uint32(strikeVal * PriceScale)
		windowStartInt := uint32(windowStartVal / 1_000_000_000)
		openInt := uint32(openVal * PriceScale)
		highInt := uint32(highVal * PriceScale)
		lowInt := uint32(lowVal * PriceScale)
		closeInt := uint32(closeVal * PriceScale)

		rows = append(rows, OptionRow{
			Expiry:       expiry,
			Strike:       strikeInt,
			OptionType:   optionType,
			WindowStart:  windowStartInt,
			Open:         openInt,
			High:         highInt,
			Low:          lowInt,
			Close:        closeInt,
			Volume:       uint32(volume),
			Transactions: uint32(transactions),
		})
	}

	return rows, scanner.Err()
}

func ExtractExpiryFromFilename(path string) (uint32, error) {

	base := filepath.Base(path)

	// AAPL_260515.csv

	parts := strings.Split(base, "_")

	expiryStr := strings.TrimSuffix(parts[1], ".csv")

	expiry, err := strconv.ParseInt(expiryStr, 10, 32)

	return uint32(expiry), err
}
