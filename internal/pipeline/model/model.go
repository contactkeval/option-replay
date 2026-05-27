package model

type CsvRow struct {
	Strike      string
	OptionType  string
	WindowStart uint64

	Open  float64
	High  float64
	Low   float64
	Close float64

	Volume       uint32
	Transactions uint32
}

type ParquetRow struct {
	Strike      uint32 `parquet:"name=strike, type=UINT32"`
	OptionType  bool   `parquet:"name=option_type, type=BOOLEAN"`
	WindowStart uint32 `parquet:"name=window_start, type=UINT32"`

	Open  uint32 `parquet:"name=open, type=UINT32"`
	High  uint32 `parquet:"name=high, type=UINT32"`
	Low   uint32 `parquet:"name=low, type=UINT32"`
	Close uint32 `parquet:"name=close, type=UINT32"`

	Volume       uint32 `parquet:"name=volume, type=UINT32"`
	Transactions uint32 `parquet:"name=transactions, type=UINT32"`
}

type TickerMetadata struct {
	CurrentFile  string   `json:"current_file"`
	RowGroups    int      `json:"row_groups"`
	PendingRows  int      `json:"pending_rows"`
	PendingFiles []string `json:"pending_files"`
}

type ParsedTicker struct {
	Underlying string
	Expiry     string
	OptionType bool
	Strike     uint32
}
