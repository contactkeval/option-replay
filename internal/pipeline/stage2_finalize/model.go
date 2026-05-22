package stage2_finalize

type CsvRow struct {
	Strike string

	OptionType string

	WindowStart uint64

	Open  float64
	High  float64
	Low   float64
	Close float64

	Volume       uint32
	Transactions uint32
}
