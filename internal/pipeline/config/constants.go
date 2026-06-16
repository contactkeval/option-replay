package config

const (

	// PriceScale converts float prices
	// into fixed-point uint32 values.
	//
	// Example:
	// 1.2345 -> 12345
	PriceScale uint32 = 10000

	MaxOpenFiles         = 128
	ScannerBufferInitial = 64 * 1024
	ScannerBufferMax     = 10 * 1024 * 1024

	TargetRowsPerRowGroup  = 246_000
	MaxTrailingRows        = 20_000
	TargetRowGroupsPerFile = 100
)
