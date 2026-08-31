package db

import "time"

type Contract struct {
	SerialNo           int64
	Underlying         string
	Expiry             time.Time
	Type               string
	Strike             float64
	BarCount           int
	LastDownloadedDate time.Time // zero if never downloaded
	DownloadAttempts   int
	Archived           bool
}

type ImportStatistics struct {
	FileName     string
	FileDate     time.Time
	DownloadType string
	StartedAt    time.Time
	EndedAt      time.Time
	RecordsRead  int
	Processed    int
	Ignored      int
	Inserted     int
	Existing     int // add records that were already in contracts
	Deleted      int
	Updated      int
	Skipped      int // retained for older call sites; prefer Ignored
	Errors       int
}

type OCCRecord struct {
	Action       string
	Underlying   string
	OCCSymbol    string
	Exchange     string
	ExpiryDate   time.Time
	Strike       float64
	Type         string
	ActivityDate time.Time
}
