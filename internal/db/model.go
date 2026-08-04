package db

import "time"

type Contract struct {
	SerialNo   int64
	Underlying string
	Expiry     time.Time
	Type       string
	Strike     float64
	GroupNo    int
}

type ImportStatistics struct {
	FileName     string
	StartedAt    time.Time
	EndedAt      time.Time
	RecordsRead  int
	Inserted     int
	Deleted      int
	Updated      int
	Skipped      int
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
