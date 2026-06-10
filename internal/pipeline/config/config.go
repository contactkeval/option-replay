package config

type Config struct {
	ArchiveRawRoot string
	MetadataRoot   string
	ParquetRoot    string
	RawRoot        string
	SQLiteRoot     string
	TempRoot       string

	// Deleted as the parameters are now defined as constants in internal/pipeline/constants/constants.go
	// MaxOpenFiles        int
	// MinRowsPerRowGroup  int
	// MaxRowGroupsPerFile int
}

func Load() Config {

	return Config{
		ArchiveRawRoot: "G:\\data\\rawdata\\archive",
		MetadataRoot:   "G:\\data\\metadata",
		ParquetRoot:    "G:\\data\\parquet",
		RawRoot:        "G:\\data\\rawdata\\pending",
		SQLiteRoot:     "G:\\data\\sqLite",
		TempRoot:       "G:\\data\\temp",

		// Deleted as the parameters are now defined as constants in internal/pipeline/constants/constants.go
		// MaxOpenFiles:        128,
		// MinRowsPerRowGroup:  100000,
		// MaxRowGroupsPerFile: 100,
	}
}
