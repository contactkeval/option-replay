package stage3_parquet

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/model"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

type ParquetFileWriter struct {
	file   *os.File
	writer *parquet.GenericWriter[model.ParquetRow]
}

func NewParquetFileWriter(
	path string,
) (*ParquetFileWriter, error) {

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)

	if err != nil {

		return nil, fmt.Errorf(
			"open parquet file %s: %w",
			path,
			err,
		)
	}

	writer := parquet.NewGenericWriter[model.ParquetRow](
		file,
		parquet.Compression(&zstd.Codec{}),
	)

	return &ParquetFileWriter{
		file:   file,
		writer: writer,
	}, nil
}

func (p *ParquetFileWriter) WriteRowGroup(
	rows []model.ParquetRow,
) error {

	if _, err := p.writer.Write(rows); err != nil {

		return fmt.Errorf(
			"write row group: %w",
			err,
		)
	}

	return nil
}

func (p *ParquetFileWriter) Close() error {

	if err := p.writer.Close(); err != nil {

		return fmt.Errorf(
			"close parquet writer: %w",
			err,
		)
	}

	if err := p.file.Close(); err != nil {

		return fmt.Errorf(
			"close parquet file: %w",
			err,
		)
	}

	return nil
}

func (p *ParquetFileWriter) FlushRowGroup() error {

	if err := p.writer.Flush(); err != nil {

		return fmt.Errorf(
			"flush row group: %w",
			err,
		)
	}

	return nil
}

func WriteTinyParquet(
	cfg config.Config,
	ticker string,
	firstExpiry time.Time,
	rowGroups []RowGroup,
) (string, error) {

	filename := fmt.Sprintf(
		"%s_%s.parquet",
		ticker,
		firstExpiry.Format("20060102"),
	)

	tickerDir := filepath.Join(
		cfg.ParquetRoot,
		ticker,
	)

	if err := os.MkdirAll(
		tickerDir,
		0755,
	); err != nil {
		return "", err
	}

	outputPath := filepath.Join(
		tickerDir,
		filename,
	)

	writer, err := NewParquetFileWriter(
		outputPath,
	)

	if err != nil {
		return "", fmt.Errorf("create parquet writer: %w", err)
	}

	for _, rowGroup := range rowGroups {

		err := writer.WriteRowGroup(
			rowGroup.Rows,
		)

		if err != nil {
			return "", fmt.Errorf("write row group: %w", err)
		}

		err = writer.FlushRowGroup()

		if err != nil {
			return "", fmt.Errorf("flush row group: %w", err)
		}
	}

	err = writer.Close()

	if err != nil {
		return "", fmt.Errorf(
			"close parquet writer: %w",
			err,
		)
	}

	return outputPath, nil
}
