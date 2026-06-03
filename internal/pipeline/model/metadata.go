package model

type ActiveMetadataRow struct {
	Expiry            string
	Rows              int
	DuplicatesRemoved int
	Status            string
}

type ActiveParquetMetadataRow struct {
	Ticker string
	Expiry string
	Rows   int

	ParquetFile   string
	StartRowGroup int
	RowGroupCount int
	Status        string

	CreatedAt string
}
