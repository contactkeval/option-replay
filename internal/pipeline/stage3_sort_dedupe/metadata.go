package stage3_sort_dedupe

type FileMetadata struct {
	Rows              uint32 `json:"rows"`
	DuplicatesRemoved uint32 `json:"duplicates_removed"`
	ProcessedAtUnix   int64  `json:"processed_at_unix"`
}
