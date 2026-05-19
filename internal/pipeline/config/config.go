package config

type Config struct {
	RawRoot string

	Stage2Root string
	Stage3Root string

	ParquetRoot string

	ArchiveRawRoot    string
	ArchiveSortedRoot string

	MaxOpenFiles int

	MinRowsPerRowGroup  int
	MaxRowGroupsPerFile int
}

func Load() Config {

	return Config{
		RawRoot: "/f/data/minute_aggs_v1",

		Stage2Root: "/f/data/stage2",
		Stage3Root: "/f/data/stage3",

		ParquetRoot: "/f/data/parquet",

		ArchiveRawRoot:    "/f/archive/minute_aggs_v1",
		ArchiveSortedRoot: "/f/archive/expiry_sorted",

		MaxOpenFiles: 128,

		MinRowsPerRowGroup:  100000,
		MaxRowGroupsPerFile: 100,
	}
}
