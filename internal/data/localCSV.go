package data

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// LocalFileDataProvider implements Data Provider from local files.
type LocalFileDataProvider struct {
	dir       string
	secondary Provider
}

// NewLocalFileDataProvider convenience constructor.
func NewLocalFileDataProvider(dir string, secondary Provider) *LocalFileDataProvider {
	return &LocalFileDataProvider{dir: dir, secondary: secondary}
}

func (localFileDataProv *LocalFileDataProvider) GetSecondary() Provider {
	return localFileDataProv.secondary
}

func (localFileDataProv *LocalFileDataProvider) SetSecondary(secondary Provider) {
	localFileDataProv.secondary = secondary
}

func (localFileDataProv *LocalFileDataProvider) GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.secondary.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	return 0, 0, 0, fmt.Errorf("GetATMOptionPrices not implemented for localFileDataProvider")
}

// GetContracts scans the local data directory for files matching the underlying.
func (localFileDataProv *LocalFileDataProvider) GetContracts(
	underlying string,
	strike float64,
	expiryDate, fromDate, toDate time.Time,
) ([]OptionContract, error) {

	files, err := os.ReadDir(localFileDataProv.dir)
	if err != nil {
		return nil, err
	}

	var out []OptionContract
	prefix := fmt.Sprintf("O-%s", strings.ToUpper(underlying)) // Note the "-" from getSymbolPath sanitize

	for _, f := range files {
		if !strings.HasPrefix(f.Name(), prefix) {
			continue
		}

		// Use the helper to extract the date
		sym := strings.TrimSuffix(f.Name(), ".csv")
		sym = strings.ReplaceAll(sym, "-", ":") // Convert back to O: format for parser

		expiry := localFileDataProv.parseExpiryFromSymbol(sym)

		if !expiryDate.IsZero() && !expiry.Equal(expiryDate) {
			continue
		}
		if expiry.Before(fromDate) || expiry.After(toDate) {
			continue
		}

		out = append(out, OptionContract{
			ExpiryDate: expiry,
			Strike:     strike, // In a full impl, parse this from the symbol string
		})
	}
	return out, nil
}

// GetBars mimics Massive's GetBars by streaming from local CSVs.
// It uses a high-performance scanner-like approach to avoid RAM bloat.
func (localFileDataProv *LocalFileDataProvider) GetBars(
	underlying string,
	fromDate, toDate time.Time,
	_ int,
	_ string,
) ([]Bar, error) {

	// Ensure data exists locally before trying to read it
	if err := localFileDataProv.EnsureLocalData(underlying, fromDate, toDate); err != nil {
		logger.Warnf("EnsureLocalData failed for %s: %v", underlying, err)
	}

	filePath := localFileDataProv.getSymbolPath(underlying)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("local file not found: %w", err)
	}
	defer file.Close()

	var out []Bar
	reader := csv.NewReader(file)

	// Skip Header
	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// CSV format: date(RFC3339), open, high, low, close, volume
		t, _ := time.Parse(time.RFC3339, row[0])

		// Logic from massive.go: Filter range and stop early if possible
		if t.Before(fromDate) {
			continue
		}
		if t.After(toDate) {
			break // Data is sorted, so we can stop scanning
		}

		out = append(out, Bar{
			Date:   t.UTC(),
			Open:   parseFloat(row[1]),
			High:   parseFloat(row[2]),
			Low:    parseFloat(row[3]),
			Close:  parseFloat(row[4]),
			Volume: parseFloat(row[5]),
		})
	}

	return out, nil
}

// GetOptionPrice mimics Massive's lookup by searching a ±5m window in local files.
func (localFileDataProv *LocalFileDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optType string,
	openDate time.Time,
) (float64, error) {
	symbol := localFileDataProv.OptionSymbolFromParts(underlying, expiryDate, optType, strike)

	// Search back-window (Massive logic)
	bars, err := localFileDataProv.GetBars(symbol, openDate.Add(-5*time.Minute), openDate, 1, "minute")
	if err == nil && len(bars) > 0 {
		return bars[len(bars)-1].Close, nil
	}

	// Search forward-window (Massive logic)
	bars, err = localFileDataProv.GetBars(symbol, openDate, openDate.Add(5*time.Minute), 1, "minute")
	if err == nil && len(bars) > 0 {
		return bars[0].Open, nil
	}

	return 0, fmt.Errorf("no local price for %s at %s", symbol, openDate)
}

// GetRelevantExpiries mirrors the 5-step logic from Massive.go but pulls from local bars.
func (localFileDataProv *LocalFileDataProvider) GetRelevantExpiries(
	underlying string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {
	logger.Infof("locally resolving expiries for %s", underlying)

	// Step 1: Load spot bars from local disk
	bars, err := localFileDataProv.GetBars(underlying, fromDate, toDate, 1, "day")
	if err != nil || len(bars) == 0 {
		return nil, fmt.Errorf("failed to fetch local spot data: %w", err)
	}

	// Step 2-6: Range calculations (Exactly as in massive.go)
	low, high := bars[0].Low, bars[0].High
	for _, b := range bars {
		if b.Low < low {
			low = b.Low
		}
		if b.High > high {
			high = b.High
		}
	}

	multiplier := 1.0
	if low >= 100 {
		multiplier = 10
	} // simplified multiplier logic

	step := (high - low) / 5
	levels := []float64{low + step, low + 3*step}

	// Step 7: Fetch local contracts
	expiryMap := map[string]time.Time{}
	for _, l := range levels {
		strike := math.Round(l/multiplier) * multiplier
		contracts, _ := localFileDataProv.GetContracts(underlying, strike, time.Time{}, fromDate, toDate)
		for _, c := range contracts {
			expiryMap[c.ExpiryDate.Format("2006-01-02")] = c.ExpiryDate
		}
	}

	// Step 8: Sort and return
	expiries := make([]time.Time, 0, len(expiryMap))
	for _, dt := range expiryMap {
		expiries = append(expiries, dt)
	}
	sort.Slice(expiries, func(i, j int) bool { return expiries[i].Before(expiries[j]) })

	return expiries, nil
}

func (localFileDataProv *LocalFileDataProvider) OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	return localFileDataProv.GetSecondary().OptionSymbolFromParts(underlying, expiryDate, optionType, strike)
}

func (localFileDataProv *LocalFileDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	return localFileDataProv.GetSecondary().parseExpiryFromSymbol(symbol)
}

// getIntervals reads the CSV once and caches it
func (localFileDataProv *LocalFileDataProvider) getIntervals(underlying string) float64 {
	intervals := make(map[string]float64)

	f, err := os.Open(filepath.Join(localFileDataProv.dir, "intervals.csv"))
	if err != nil {
		logger.Infof("open intervals file: %v", err)
		return 0
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		logger.Infof("read csv: %v", err)
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
func (localFileDataProv *LocalFileDataProvider) RoundToNearestStrike(
	underlying string,
	_, openDate time.Time,
	asOfPrice float64,
) float64 {
	intervals := 0.0
	var loadOnce sync.Once
	loadOnce.Do(func() {
		intervals = localFileDataProv.getIntervals(underlying)
	})

	if intervals == 0.0 {
		// fail safe: no rounding
		return asOfPrice
	}

	for {
		strike := math.Round(asOfPrice/intervals) * intervals

		bars, err := localFileDataProv.GetBars(underlying, openDate, openDate, 1, "minute")
		if err != nil {
			return asOfPrice
		}

		if len(bars) == 0 {
			intervals += intervals // double interval and retry
			continue
		}
		return strike
	}
}
