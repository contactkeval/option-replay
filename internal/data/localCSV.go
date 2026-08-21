package data

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
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

// GetName returns the name of the provider.
func (*LocalFileDataProvider) GetName() string {
	return "localCSV"
}

func (localFileDataProv *LocalFileDataProvider) GetSecondary() Provider {
	return localFileDataProv.secondary
}

func (localFileDataProv *LocalFileDataProvider) SetSecondary(secondary Provider) {
	localFileDataProv.secondary = secondary
}

func (localFileDataProv *LocalFileDataProvider) GetATMOptionPrices(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.GetSecondary().GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	return 0, 0, 0, fmt.Errorf("GetATMOptionPrices not implemented for localFileDataProvider")
}

// GetContracts scans the local data directory for files matching the underlying.
// TODO: take a closer look at the method
func (localFileDataProv *LocalFileDataProvider) GetContracts(
	underlying string,
	strike float64,
	fromDate, toDate time.Time,
	expired bool,
) ([]OptionContract, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.GetSecondary().GetContracts(underlying, strike, fromDate, toDate, expired)
	}
	return nil, fmt.Errorf("GetContracts not implemented for localFileDataProvider")
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
		dir, err := os.Getwd()
		if err != nil {
			logger.Errorf("Error: %v", err)
			return nil, fmt.Errorf("local file not found: %w", err)
		}
		logger.Infof("Current directory: %s", dir)
		return nil, fmt.Errorf("local file not found: %w", err)
	}
	defer file.Close()

	var out []Bar
	reader := csv.NewReader(file)

	// Skip Header
	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	emptyFile := true
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		emptyFile = false

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
	if emptyFile {
		return nil, fmt.Errorf("%w: %s", ErrNoDataFound, underlying)
	}

	return out, nil
}

// GetOptionPrice mimics Massive's lookup by searching a ±5m window in local files.
func (localFileDataProv *LocalFileDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optionType string,
	openDate time.Time,
) (float64, error) {
	symbol := localFileDataProv.OptionSymbolFromParts(underlying, expiryDate, optionType, strike)

	// Search back-window
	bars, err := localFileDataProv.GetBars(symbol, openDate.Add(-5*time.Minute), openDate, 1, "minute")
	if err != nil {
		if errors.Is(err, ErrNoDataFound) {
			return 0, err
		} else {
			return 0, fmt.Errorf("error while search back for option price %w", err)
		}
	}
	if len(bars) != 0 {
		return bars[len(bars)-1].Close, nil
	}

	// Search forward-window
	bars, err = localFileDataProv.GetBars(symbol, openDate, openDate.Add(5*time.Minute), 1, "minute")
	if err != nil {
		if errors.Is(err, ErrNoDataFound) {
			return 0, err
		} else {
			return 0, fmt.Errorf("error while search forward for option price %w", err)
		}
	}
	if len(bars) != 0 {
		return bars[0].Close, nil
	}

	return 0, fmt.Errorf("no local price for %s at %s", symbol, openDate.Format("2006-01-02 15:04"))
}

// GetRelevantExpiries mirrors the 5-step logic from Massive.go but pulls from local bars.
func (localFileDataProv *LocalFileDataProvider) GetRelevantExpiries(
	underlying string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {
	if localFileDataProv.secondary != nil {
		return localFileDataProv.GetSecondary().GetRelevantExpiries(underlying, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for localFileDataProvider")
}

func (localFileDataProv *LocalFileDataProvider) OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	return localFileDataProv.GetSecondary().OptionSymbolFromParts(underlying, expiryDate, optionType, strike)
}

func (localFileDataProv *LocalFileDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	return localFileDataProv.GetSecondary().parseExpiryFromSymbol(symbol)
}

// GetStrikeInterval reads the CSV once and caches it
func (localFileDataProv *LocalFileDataProvider) GetStrikeIntervals(underlying string, expiryDate time.Time) []float64 {
	return localFileDataProv.GetSecondary().GetStrikeIntervals(underlying, expiryDate)
}

// RoundToNearestStrike rounds `price` using the interval for the underlying
func (localFileDataProv *LocalFileDataProvider) RoundToNearestStrike(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) float64 {
	return localFileDataProv.GetSecondary().RoundToNearestStrike(underlying, expiryDate, openDate, asOfPrice)
}
