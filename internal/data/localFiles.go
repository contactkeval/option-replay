package data

import (
	"encoding/csv"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
)

// localFileDataProvider implements Data Provider from local files.
type localFileDataProvider struct {
	dir       string
	secondary Provider
}

// NewLocalFileDataProvider convenience constructor.
func NewLocalFileDataProvider(dir string, secondary Provider) *localFileDataProvider {
	return &localFileDataProvider{dir: dir, secondary: secondary}
}

func (localFileDataProv *localFileDataProvider) Secondary() Provider {
	return localFileDataProv.secondary
}

// TODO: delete this old version after verifying no usage
func (localFileDataProv *localFileDataProvider) roundToNearestStrikeOld(underlying string, v float64) float64 {
	strikeInterval := 50.0 // Example for NIFTY, change as needed
	return math.Round(v/strikeInterval) * strikeInterval
}

// getIntervals reads the CSV once and caches it
func (localFileDataProv *localFileDataProvider) getIntervals(underlying string) float64 {
	intervals := make(map[string]float64)

	f, err := os.Open(localFileDataProv.dir + "intervals.csv")
	if err != nil {
		log.Printf("open intervals file: %v", err)
		return 0
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		log.Printf("read csv: %v", err)
		return 0
	}

	// Skip header
	for i, row := range records {
		if i == 0 {
			continue
		}
		if len(row) < 2 {
			continue
		}

		underlying := strings.ToUpper(strings.TrimSpace(row[0]))
		interval, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}

		intervals[underlying] = interval
	}

	if val, ok := intervals[strings.ToUpper(underlying)]; ok {
		return float64(val)
	}

	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.getIntervals(underlying)
		//TODO: consider logging missing underlying
	}

	return 0
}

// roundToNearestStrike rounds `v` using the interval for the underlying
func (localFileDataProv *localFileDataProvider) roundToNearestStrike(underlying string, v float64) float64 {
	intervals := 0.0
	var loadOnce sync.Once
	loadOnce.Do(func() {
		intervals = localFileDataProv.getIntervals(underlying)
	})

	if intervals == 0.0 {
		// fail safe: no rounding
		return v
	}

	return math.Round(v/intervals) * intervals
}
