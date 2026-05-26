package stage2_finalize

import (
	"bufio"
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func WriteRows(path string, rows []model.ParquetRow) error {

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, r := range rows {

		optionType := "P"

		if r.OptionType {
			optionType = "C"
		}

		line := fmt.Sprintf(
			"%08d,%s,%d,%d,%d,%d,%d,%d,%d\n",
			r.Strike,
			optionType,
			r.WindowStart,
			r.Open,
			r.High,
			r.Low,
			r.Close,
			r.Volume,
			r.Transactions,
		)

		if _, err := writer.WriteString(line); err != nil {
			return err
		}
	}

	return nil
}
