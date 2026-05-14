package parquetbuilder

type OptionRow struct {
	Expiry     int32  `parquet:"expiry"`
	Strike     int64  `parquet:"strike"`
	OptionType string `parquet:"option_type"`

	WindowStart int64 `parquet:"window_start"`

	Open  float32 `parquet:"open"`
	High  float32 `parquet:"high"`
	Low   float32 `parquet:"low"`
	Close float32 `parquet:"close"`

	Volume       int64 `parquet:"volume"`
	Transactions int32 `parquet:"transactions"`
}
