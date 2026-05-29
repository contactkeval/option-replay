package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/reader"
)

type RowGroupRecord struct {
	FileName         string
	FileSizeMB       float64
	TotalFileRows    int64
	RowGroupNo       int
	RowGroupRows     int64
	ColumnName       string
	MinValue         string
	MaxValue         string
	CompressionCodec string
}

func main() {

	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/metadata/main.go <parquet-directory> <output-csv>")
		os.Exit(1)
	}

	parquetRoot := os.Args[1]
	outputCSV := os.Args[2]

	records := make([]RowGroupRecord, 0)

	err := filepath.Walk(parquetRoot, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".parquet" {
			return nil
		}

		fmt.Printf("Processing: %s\n", path)

		recs, err := processParquet(path)
		if err != nil {
			return err
		}

		records = append(records, recs...)

		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	if err := writeCSV(outputCSV, records); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nMetadata exported successfully to: %s\n", outputCSV)
}

func processParquet(filePath string) ([]RowGroupRecord, error) {

	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return nil, err
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, nil, 1)
	if err != nil {
		return nil, err
	}
	defer pr.ReadStop()

	footer := pr.Footer

	records := make([]RowGroupRecord, 0)

	for rgNo, rg := range footer.RowGroups {

		for _, col := range rg.Columns {

			meta := col.MetaData
			if meta == nil || meta.Statistics == nil {
				continue
			}

			colName := ""
			if len(meta.PathInSchema) > 0 {
				colName = meta.PathInSchema[0]
			}

			minVal := decodeStatValue(meta.Type, meta.Statistics.Min)
			maxVal := decodeStatValue(meta.Type, meta.Statistics.Max)

			codec := meta.Codec.String()

			records = append(records, RowGroupRecord{
				FileName:         filepath.Base(filePath),
				FileSizeMB:       float64(stat.Size()) / (1024 * 1024),
				TotalFileRows:    footer.GetNumRows(),
				RowGroupNo:       rgNo,
				RowGroupRows:     rg.GetNumRows(),
				ColumnName:       colName,
				MinValue:         minVal,
				MaxValue:         maxVal,
				CompressionCodec: codec,
			})
		}
	}

	return records, nil
}

func writeCSV(filePath string, records []RowGroupRecord) error {

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"fileName",
		"fileSizeMB",
		"totalFileRows",
		"rowGroupNo",
		"rowGroupRows",
		"columnName",
		"minValue",
		"maxValue",
		"compressionCodec",
	}

	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range records {

		row := []string{
			r.FileName,
			fmt.Sprintf("%.2f", r.FileSizeMB),
			strconv.FormatInt(r.TotalFileRows, 10),
			strconv.Itoa(r.RowGroupNo),
			strconv.FormatInt(r.RowGroupRows, 10),
			r.ColumnName,
			r.MinValue,
			r.MaxValue,
			r.CompressionCodec,
		}

		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func decodeStatValue(t parquet.Type, b []byte) string {

	if b == nil {
		return ""
	}

	switch t {

	case parquet.Type_INT32:
		if len(b) < 4 {
			return ""
		}
		v := int32(binary.LittleEndian.Uint32(b))
		return strconv.FormatInt(int64(v), 10)

	case parquet.Type_INT64:
		if len(b) < 8 {
			return ""
		}
		v := int64(binary.LittleEndian.Uint64(b))
		return strconv.FormatInt(v, 10)

	case parquet.Type_FLOAT:
		if len(b) < 4 {
			return ""
		}
		bits := binary.LittleEndian.Uint32(b)
		v := math.Float32frombits(bits)
		return fmt.Sprintf("%.4f", v)

	case parquet.Type_DOUBLE:
		if len(b) < 8 {
			return ""
		}
		bits := binary.LittleEndian.Uint64(b)
		v := math.Float64frombits(bits)
		return fmt.Sprintf("%.4f", v)

	case parquet.Type_BYTE_ARRAY:
		return string(b)

	default:
		return fmt.Sprintf("%v", b)
	}
}
