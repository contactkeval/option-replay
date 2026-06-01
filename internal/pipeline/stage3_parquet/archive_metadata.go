package stage3_parquet

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func AppendArchiveMetadata(
	path string,
	row model.ArchiveMetadataRow,
) error {

	newFile := false

	if _, err := os.Stat(path); os.IsNotExist(err) {
		newFile = true
	}

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)

	if err != nil {
		return fmt.Errorf(
			"open archive metadata %s: %w",
			path,
			err,
		)
	}

	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if newFile {

		if err := writer.Write([]string{
			"ticker",
			"expiry",
			"rows",
			"parquet_file",
			"start_row_group",
			"row_group_count",
			"archived_at",
		}); err != nil {
			return err
		}
	}

	return writer.Write([]string{
		row.Ticker,
		row.Expiry,
		strconv.Itoa(row.Rows),
		row.ParquetFile,
		strconv.Itoa(row.StartRowGroup),
		strconv.Itoa(row.RowGroupCount),
		row.ArchivedAt,
	})
}
