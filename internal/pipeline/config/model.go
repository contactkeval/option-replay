package config

import "time"

type ParsedTicker struct {
	Underlying string
	ExpiryDate time.Time
	OptionType bool
	Strike     uint32
}

type CompactCandidate struct {
	Ticker        string
	ParquetPath   string
	FirstExpiry   time.Time
	RowGroupCount int
}

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
	ExpiryDate  uint32 `parquet:"name=expiry_date, type=UINT32"`
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

type ActiveMetadataRow struct {
	Ticker     string
	ExpiryDate time.Time
	RowCount   int

	Status        string
	ParquetPath   *string
	RowGroupCount *int
}
type ActiveParquetMetadataRow struct {
	Ticker     string
	ExpiryDate time.Time
	RowCount   int

	Status        string
	ParquetPath   string
	StartRowGroup int
	RowGroupCount int

	CreatedAt string
}

type ArchivedMetadataRow struct {
	Ticker     string
	ExpiryDate time.Time
	RowCount   int

	ParquetPath   string
	RowGroupCount int
	StartRowGroup int

	CreatedAt  string
	ArchivedAt string
}

type TransientRow struct {
	Ticker string

	ParquetRow
}

type DXFloat float64

type Candle struct {
	EventType   string `json:"eventType"`
	EventSymbol string `json:"eventSymbol"`
	EventTime   int64  `json:"eventTime"`
	EventFlags  int32  `json:"eventFlags"`

	Index     int64   `json:"index"`
	Time      int64   `json:"time"`
	Sequence  int64   `json:"sequence"`
	BidVolume DXFloat `json:"bidVolume"`
	AskVolume DXFloat `json:"askVolume"`

	Open  DXFloat `json:"open"`
	High  DXFloat `json:"high"`
	Low   DXFloat `json:"low"`
	Close DXFloat `json:"close"`

	Volume        DXFloat `json:"volume"`
	Count         int64   `json:"count"`
	VWAP          DXFloat `json:"vwap"`
	ImpVolatility DXFloat `json:"impVolatility"`
	OpenInterest  DXFloat `json:"openInterest"`
}
