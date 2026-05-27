package config

type Config struct {
	RawRoot  string
	TempRoot string

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
		RawRoot:  "G:\\data\\minute_aggs_v1",
		TempRoot: "G:\\data\\temp",

		Stage2Root:  "G:\\data\\stage2",
		Stage3Root:  "G:\\data\\stage3",
		ParquetRoot: "G:\\data\\parquet",

		ArchiveRawRoot:    "G:\\archive\\minute_aggs_v1",
		ArchiveSortedRoot: "G:\\archive\\expiry_sorted",

		MaxOpenFiles: 128,

		MinRowsPerRowGroup:  100000,
		MaxRowGroupsPerFile: 100,
	}
}
