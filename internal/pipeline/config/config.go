package config

type Config struct {
	ArchiveRawRoot string
	MetadataRoot   string
	ParquetRoot    string
	RawRoot        string
	SQLiteRoot     string
	TempRoot       string

	// TODO: delete
	ArchiveSortedRoot string
	Stage2Root        string
	Stage3Root        string

	// Deleted as the parameters are now defined as constants in internal/pipeline/constants/constants.go
	// MaxOpenFiles        int
	// MinRowsPerRowGroup  int
	// MaxRowGroupsPerFile int
}

func Load() Config {

	return Config{
		ArchiveRawRoot: "G:\\data\\rawdata\\archive\\minute_aggs_v1",
		MetadataRoot:   "G:\\data\\metadata",
		ParquetRoot:    "G:\\data\\parquet",
		RawRoot:        "G:\\data\\rawdata\\pending\\minute_aggs_v1",
		SQLiteRoot:     "G:\\data\\sqLite",
		TempRoot:       "G:\\data\\temp",

		// TODO: delete
		Stage2Root:        "G:\\data\\stage2",
		Stage3Root:        "G:\\data\\stage3",
		ArchiveSortedRoot: "G:\\data\\archive\\expiry_sorted",

		// Deleted as the parameters are now defined as constants in internal/pipeline/constants/constants.go
		// MaxOpenFiles:        128,
		// MinRowsPerRowGroup:  100000,
		// MaxRowGroupsPerFile: 100,
	}
}
