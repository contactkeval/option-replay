package stage3_sort_dedupe

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/contactkeval/option-replay/internal/pipeline/constants"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

func LoadRows(path string) ([]Stage3Row, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	buf := make([]byte, 0, constants.ScannerBufferInitial)
	scanner.Buffer(buf, constants.ScannerBufferMax)
	var rows []Stage3Row

	for scanner.Scan() {

		line := scanner.Text()
		parts := strings.Split(line, ",")
		strike := util.ParseStrike(parts[0])
		optionType := util.ParseOptionType(parts[1])
		windowStartNs, _ := strconv.ParseUint(parts[2], 10, 64)
		windowStartSec := util.NanosecondsToSeconds(windowStartNs)

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

			Open:  util.PriceToUint32(openFloat),
			High:  util.PriceToUint32(highFloat),
			Low:   util.PriceToUint32(lowFloat),
			Close: util.PriceToUint32(closeFloat),

			Volume:       uint32(volume),
			Transactions: uint32(transactions),
		})
	}

	return rows, scanner.Err()
}
