package model

type ActiveMetadataRow struct {
	Expiry string
	Rows   int
	Status string
}

type ArchiveMetadataRow struct {
	Ticker string
	Expiry string
	Rows   int

	ParquetFile   string
	StartRowGroup int
	RowGroupCount int

	ArchivedAt string
}
