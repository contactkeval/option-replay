package constants

const (

	// PriceScale converts float prices
	// into fixed-point uint32 values.
	//
	// Example:
	// 1.2345 -> 12345
	PriceScale uint32 = 10000

	// StrikeScale converts strike values
	// into fixed-point uint32 values.
	//
	// Example:
	// 450.00 -> 4500000
	StrikeScale uint32 = 10000

	ScannerBufferInitial = 64 * 1024
	ScannerBufferMax     = 10 * 1024 * 1024

	RowGroupTargetRows     = 100_000
	MaxTrailingRows        = 10_000
	TargetRowGroupsPerFile = 100
)
