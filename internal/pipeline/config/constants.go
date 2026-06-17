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

	MaxRowsPerRowGroup     = 256_000
	MaxShortRows           = 16_000
	TargetRowGroupsPerFile = 100
)
