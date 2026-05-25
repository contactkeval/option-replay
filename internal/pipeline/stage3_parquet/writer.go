package stage3_parquet

import (
	"os"

	"github.com/parquet-go/parquet-go"
)

func WriteRowGroup(
	path string,
	rows []ParquetRow,
) error {

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)

	if err != nil {
		return err
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[ParquetRow](file)
	defer writer.Close()

	_, err = writer.Write(rows)
	return err
}
