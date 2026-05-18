package parquetbuilder

import (
	"os"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

func WriteParquet(path string, rows []OptionRow) error {

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[OptionRow](
		file,
		parquet.Compression(&zstd.Codec{}),
		parquet.MaxRowsPerRowGroup(2_000_000),
	)
	defer writer.Close()

	_, err = writer.Write(rows)

	return err
}
