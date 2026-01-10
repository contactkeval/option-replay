package data

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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

func (localFileDataProv *localFileDataProvider) GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	return 0, 0, 0, fmt.Errorf("GetATMOptionPrices not implemented for localFileDataProvider")
}

func (localFileDataProv *localFileDataProvider) GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetContracts(underlying, strike, expiryDate, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetContracts not implemented for localFileDataProvider")
}

func (localFileDataProv *localFileDataProvider) GetBars(underlying string, fromDate, toDate time.Time) ([]Bar, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetBars(underlying, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetBars not implemented for localFileDataProvider")
}

func (localFileDataProv *localFileDataProvider) GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optType string, openDate time.Time) (float64, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetOptionPrice(underlying, strike, expiryDate, optType, openDate)
	}
	return 0, fmt.Errorf("GetOptionMidPrice not implemented for localFileDataProvider")
}

func (localFileDataProv *localFileDataProvider) GetRelevantExpiries(ticker string, fromDate, toDate time.Time) ([]time.Time, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetRelevantExpiries(ticker, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for localFileDataProvider")
}

// getIntervals reads the CSV once and caches it
func (localFileDataProv *localFileDataProvider) getIntervals(underlying string) float64 {
	intervals := make(map[string]float64)

	f, err := os.Open(filepath.Join(localFileDataProv.dir, "intervals.csv"))
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

	for _, row := range records {
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

// RoundToNearestStrike rounds `price` using the interval for the underlying
func (localFileDataProv *localFileDataProvider) RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64 {
	intervals := 0.0
	strike := 0.0
	var loadOnce sync.Once
	loadOnce.Do(func() {
		intervals = localFileDataProv.getIntervals(underlying)
	})

	if intervals == 0.0 {
		// fail safe: no rounding
		return asOfPrice
	}

	for {
		strike = math.Round(asOfPrice/intervals) * intervals

		bars, err := localFileDataProv.GetBars(underlying, openDate, openDate)
		if err != nil {
			return asOfPrice
		}

		if len(bars) == 0 {
			intervals += intervals // double interval and retry
			continue
		} else {
			// success case
			break
		}
	}
	return strike
}
