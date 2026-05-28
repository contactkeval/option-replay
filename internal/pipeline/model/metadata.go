package model

type ActiveMetadataRow struct {
	Expiry string
	Rows   int
	Status string
}

type ArchiveMetadataRow struct {
	Ticker            string
	Expiry            string
	Rows              int
	DuplicatesRemoved int

	ParquetFile string
	RowGroup    int
	ArchivedAt  string
}
