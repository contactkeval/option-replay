package model

type ActiveMetadataRow struct {
	Expiry            string
	Rows              int
	DuplicatesRemoved int
}

type ActiveParquetMetadataRow struct {
	Ticker string
	Expiry string
	Rows   int

	ParquetFile   string
	StartRowGroup int
	RowGroupCount int

	CreatedAt string
}
