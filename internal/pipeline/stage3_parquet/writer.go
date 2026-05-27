package stage3_parquet

import (
	"fmt"
	"os"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

func WriteRowGroup(
	path string,
	rows []model.ParquetRow,
) error {

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("open parquet file %s: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := parquet.NewGenericWriter[model.ParquetRow](
		file,
		parquet.Compression(&zstd.Codec{}),
	)
	if _, err := writer.Write(rows); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write parquet rows file=%s rows=%d: %w", path, len(rows), err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close parquet writer file=%s: %w", path, err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf(
			"sync parquet file %s: %w",
			path,
			err,
		)
	}

	return nil
}
