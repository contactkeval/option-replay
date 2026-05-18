package parquetbuilder

const PriceScale = 10000

type OptionRow struct {
	Expiry     uint32 `parquet:"expiry"`
	Strike     uint32 `parquet:"strike"`
	OptionType bool   `parquet:"option_type"`

	WindowStart uint32 `parquet:"window_start"`

	Open  uint32 `parquet:"open"`
	High  uint32 `parquet:"high"`
	Low   uint32 `parquet:"low"`
	Close uint32 `parquet:"close"`

	Volume       uint32 `parquet:"volume"`
	Transactions uint32 `parquet:"transactions"`
}
