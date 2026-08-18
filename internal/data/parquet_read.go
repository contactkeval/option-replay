package data

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
	"github.com/parquet-go/parquet-go"
)

type parquetFilter struct {
	expiry     *uint32
	strike     *uint32
	optionType *bool
	fromTime   time.Time
	toTime     time.Time
	fromExpiry time.Time
	toExpiry   time.Time
}

func (p *ParquetDataProvider) scanRows(
	ticker string,
	filter parquetFilter,
	fn func(config.ParquetRow) error,
) error {
	sources, err := p.metadata.LookupParquetSources(ticker, filter.fromExpiry, filter.toExpiry)
	if err != nil {
		return err
	}
	if filter.expiry != nil {
		want := util.DecodeExpiryDate(*filter.expiry)
		filtered := sources[:0]
		for _, src := range sources {
			if sameDate(src.ExpiryDate, want) {
				filtered = append(filtered, src)
			}
		}
		sources = filtered
	}
	if len(sources) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		key := fmt.Sprintf("%s:%d:%d", src.ParquetPath, src.StartRowGroup, src.RowGroupCount)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if err := p.scanParquetFile(src, filter, fn); err != nil {
			return err
		}
	}
	return nil
}

func (p *ParquetDataProvider) scanParquetFile(
	src db.ParquetSource,
	filter parquetFilter,
	fn func(config.ParquetRow) error,
) error {
	file, err := os.Open(src.ParquetPath)
	if err != nil {
		logger.Warnf("parquet file missing=%s err=%v", src.ParquetPath, err)
		return nil
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat parquet %s: %w", src.ParquetPath, err)
	}

	pf, err := parquet.OpenFile(file, stat.Size())
	if err != nil {
		return fmt.Errorf("open parquet %s: %w", src.ParquetPath, err)
	}

	rowGroups := pf.RowGroups()
	start := src.StartRowGroup
	count := src.RowGroupCount
	if start < 0 {
		start = 0
	}
	if count <= 0 || start+count > len(rowGroups) {
		count = len(rowGroups) - start
	}
	if count <= 0 {
		return nil
	}

	schemaCols := pf.Schema().Columns()
	colIndex := make(map[string]int, len(schemaCols))
	for i, names := range schemaCols {
		name := names[0]
		name = strings.TrimPrefix(name, "name=")
		colIndex[name] = i
	}

	buf := make([]config.ParquetRow, 1024)
	end := start + count
	for i := start; i < end; i++ {
		rg := rowGroups[i]
		if !rowGroupMatches(rg, colIndex, filter) {
			continue
		}

		reader := parquet.NewGenericRowGroupReader[config.ParquetRow](rg)
		for {
			n, readErr := reader.Read(buf)
			for _, row := range buf[:n] {
				if !rowMatches(row, filter) {
					continue
				}
				if err := fn(row); err != nil {
					_ = reader.Close()
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				_ = reader.Close()
				return fmt.Errorf("read parquet %s rowgroup=%d: %w", src.ParquetPath, i, readErr)
			}
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("close parquet reader %s: %w", src.ParquetPath, err)
		}
	}

	return nil
}

func rowMatches(row config.ParquetRow, filter parquetFilter) bool {
	if filter.expiry != nil && row.ExpiryDate != *filter.expiry {
		return false
	}
	if filter.strike != nil && row.Strike != *filter.strike {
		return false
	}
	if filter.optionType != nil && row.OptionType != *filter.optionType {
		return false
	}
	if !filter.fromTime.IsZero() && int64(row.WindowStart) < filter.fromTime.Unix() {
		return false
	}
	if !filter.toTime.IsZero() && int64(row.WindowStart) > filter.toTime.Unix() {
		return false
	}
	return true
}

func rowGroupMatches(
	rg parquet.RowGroup,
	colIndex map[string]int,
	filter parquetFilter,
) bool {
	if filter.expiry != nil {
		minV, maxV, ok := columnUint32Range(rg, colIndex, "expiry_date")
		if ok && (*filter.expiry < minV || *filter.expiry > maxV) {
			return false
		}
	}
	if filter.strike != nil {
		minV, maxV, ok := columnUint32Range(rg, colIndex, "strike")
		if ok && (*filter.strike < minV || *filter.strike > maxV) {
			return false
		}
	}
	if !filter.fromTime.IsZero() || !filter.toTime.IsZero() {
		minV, maxV, ok := columnUint32Range(rg, colIndex, "window_start")
		if ok {
			if !filter.toTime.IsZero() && minV > uint32(filter.toTime.Unix()) {
				return false
			}
			if !filter.fromTime.IsZero() && maxV < uint32(filter.fromTime.Unix()) {
				return false
			}
		}
	}
	if filter.optionType != nil {
		minV, maxV, ok := columnUint32Range(rg, colIndex, "option_type")
		if ok {
			want := uint32(0)
			if *filter.optionType {
				want = 1
			}
			if want < minV || want > maxV {
				return false
			}
		}
	}
	return true
}

func columnUint32Range(
	rg parquet.RowGroup,
	colIndex map[string]int,
	name string,
) (uint32, uint32, bool) {
	idx, ok := colIndex[name]
	if !ok {
		return 0, 0, false
	}
	chunks := rg.ColumnChunks()
	if idx < 0 || idx >= len(chunks) {
		return 0, 0, false
	}

	fc, ok := chunks[idx].(*parquet.FileColumnChunk)
	if !ok {
		return 0, 0, false
	}

	colIdx, err := fc.ColumnIndex()
	if err != nil || colIdx.NumPages() == 0 {
		return 0, 0, false
	}

	minV, minOK := parquetValueUint32(colIdx.MinValue(0))
	maxV, maxOK := parquetValueUint32(colIdx.MaxValue(0))
	if !minOK || !maxOK {
		return 0, 0, false
	}

	for page := 1; page < colIdx.NumPages(); page++ {
		pageMin, okMin := parquetValueUint32(colIdx.MinValue(page))
		pageMax, okMax := parquetValueUint32(colIdx.MaxValue(page))
		if !okMin || !okMax {
			return 0, 0, false
		}
		if pageMin < minV {
			minV = pageMin
		}
		if pageMax > maxV {
			maxV = pageMax
		}
	}

	return minV, maxV, true
}

func parquetValueUint32(v parquet.Value) (uint32, bool) {
	switch v.Kind() {
	case parquet.Boolean:
		if v.Boolean() {
			return 1, true
		}
		return 0, true
	case parquet.Int32:
		return v.Uint32(), true
	case parquet.Int64:
		return uint32(v.Int64()), true
	default:
		return 0, false
	}
}
