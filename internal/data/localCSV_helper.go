package data

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

type DataRecord struct {
	Symbol    string    `csv:"symbol"`
	FirstDate time.Time `csv:"first_date"`
	LastDate  time.Time `csv:"last_date"`
}

func (p *localFileDataProvider) EnsureLocalData(symbol string, startDate, endDate time.Time) error {
	// 1. Identify instrument type using your prefix convention
	isOption := strings.HasPrefix(symbol, "O:")

	// 2. Open list file (manifest)
	records, err := p.loadManifest()
	if err != nil {
		return fmt.Errorf("ensure local: %w", err)
	}

	record, exists := records[symbol]
	now := time.Now()

	if exists {
		// 3. Record exists: Check if we need to expand the data
		if isOption {
			// Requirements:
			// a. If within range, return
			// b. No need to check startDate
			// c. if end after lastDate, fetch till TODAY
			if endDate.After(record.LastDate) {
				logger.Infof("Option %s: Extending data to today", symbol)
				if err := p.fetchAndAppend(symbol, record.LastDate, now); err != nil {
					return err
				}
				record.LastDate = now
				records[symbol] = record // Update map
			}
		} else {
			// Stock or Index Logic:
			// if startDate prior to firstDate, fetch start -> first
			if startDate.Before(record.FirstDate) {
				logger.Infof("Symbol %s: Fetching historical gap", symbol)
				if err := p.fetchAndAppend(symbol, startDate, record.FirstDate); err != nil {
					return err
				}
				record.FirstDate = startDate
			}
			// if endDate after lastDate, fetch lastDate -> today
			if endDate.After(record.LastDate) {
				logger.Infof("Symbol %s: Fetching recent gap", symbol)
				if err := p.fetchAndAppend(symbol, record.LastDate, now); err != nil {
					return err
				}
				record.LastDate = now
			}
			records[symbol] = record
		}
	} else {
		// 4. Record does not exist: Initial Fetch
		if isOption {
			// Requirement: fetch 2 years prior from expiryDate till today
			expiryDate := p.parseExpiryFromSymbol(symbol)
			if expiryDate.IsZero() {
				return fmt.Errorf("failed to parse expiry from %s", symbol)
			}

			fetchStart := expiryDate.AddDate(-2, 0, 0)
			if err := p.fetchAndAppend(symbol, fetchStart, now); err != nil {
				return err
			}

			// Add record to file with specific conditions
			newRec := DataRecord{Symbol: symbol}
			// firstDate added only if expiry within 6 months from requested start
			if expiryDate.Before(startDate.AddDate(0, 6, 0)) {
				newRec.FirstDate = fetchStart
			}
			// lastDate added only if expiry is in the future
			if expiryDate.After(now) {
				newRec.LastDate = now
			}
			records[symbol] = newRec
		} else {
			// Stocks/Indices: Fetch for given period
			if err := p.fetchAndAppend(symbol, startDate, endDate); err != nil {
				return err
			}
			records[symbol] = DataRecord{
				Symbol:    symbol,
				FirstDate: startDate,
				LastDate:  endDate,
			}
		}
	}

	// 5. Update the manifest file
	return p.saveManifest(records)
}

func (p *localFileDataProvider) RunMaintenancePipeline() error {
	records, err := p.loadManifest()
	if err != nil {
		return err
	}

	now := time.Now()
	twoYearsAgo := now.AddDate(-2, 0, 0)
	oneMonthAgo := now.AddDate(0, -1, 0)

	for sym, rec := range records {
		isOption := strings.HasPrefix(sym, "O:")
		updated := false

		if isOption {
			// Option: Update to today if last date is older than 1 month
			if rec.LastDate.Before(oneMonthAgo) {
				if err := p.fetchAndAppend(sym, rec.LastDate, now); err == nil {
					rec.LastDate = now
					updated = true
				}
			}
		} else {
			// Stock/Index:
			// Fill historical gap if data starts after 2 years ago
			if rec.FirstDate.After(twoYearsAgo) {
				if err := p.fetchAndAppend(sym, twoYearsAgo, rec.FirstDate); err == nil {
					rec.FirstDate = twoYearsAgo
					updated = true
				}
			}
			// Fill recent gap if data is older than 1 month
			if rec.LastDate.Before(oneMonthAgo) {
				if err := p.fetchAndAppend(sym, rec.LastDate, now); err == nil {
					rec.LastDate = now
					updated = true
				}
			}
		}

		if updated {
			records[sym] = rec
		}
	}

	return p.saveManifest(records)
}

func (p *localFileDataProvider) fetchAndAppend(symbol string, startDate, endDate time.Time) error {
	// 1. Fetch from secondary provider (e.g. massive)
	newData, err := p.GetSecondary().GetBars(symbol, startDate, endDate, 1, "minute")
	if err != nil {
		return err
	}

	// 2. Open local file in Append mode
	f, err := os.OpenFile(p.getSymbolPath(symbol), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 3. Logic to check if file is empty (to write header)
	info, _ := f.Stat()
	writer := csv.NewWriter(f)

	if info.Size() == 0 {
		writer.Write([]string{"date", "open", "high", "low", "close", "volume"})
	}

	// 4. Write data rows
	for _, bar := range newData {
		writer.Write([]string{
			bar.Date.Format(time.RFC3339),
			fmt.Sprintf("%.2f", bar.Open),
			fmt.Sprintf("%.2f", bar.High),
			fmt.Sprintf("%.2f", bar.Low),
			fmt.Sprintf("%.2f", bar.Close),
			fmt.Sprintf("%.0f", bar.Volume),
		})
	}
	writer.Flush()
	return nil
}

// loadManifest reads the CSV tracking file and returns a map for fast lookup.
func (p *localFileDataProvider) loadManifest() (map[string]DataRecord, error) {
	path := p.getManifestPath() // e.g., "data/manifest.csv"
	records := make(map[string]DataRecord)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return records, nil // Return empty map if file doesn't exist yet
		}
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header
	if _, err := reader.Read(); err != nil {
		return records, nil
	}

	rows, _ := reader.ReadAll()
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}

		first, _ := time.Parse("2006-01-02", row[1])
		last, _ := time.Parse("2006-01-02", row[2])

		records[row[0]] = DataRecord{
			Symbol:    row[0],
			FirstDate: first,
			LastDate:  last,
		}
	}
	return records, nil
}

// saveManifest overwrites the manifest file with the updated data.
func (p *localFileDataProvider) saveManifest(records map[string]DataRecord) error {
	file, err := os.Create(p.getManifestPath())
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write Header
	writer.Write([]string{"symbol", "first_date", "last_date"})

	for _, rec := range records {
		writer.Write([]string{
			rec.Symbol,
			rec.FirstDate.Format("2006-01-02"),
			rec.LastDate.Format("2006-01-02"),
		})
	}
	return nil
}

// getManifestPath returns the absolute path to the data catalog file.
func (p *localFileDataProvider) getManifestPath() string {
	// p.BaseDir is likely something like "./data" or "/var/lib/option-replay"
	return filepath.Join(p.dir, "manifest.csv")
}

// getSymbolPath returns the path for a specific instrument's data file.
func (p *localFileDataProvider) getSymbolPath(symbol string) string {
	// Sanitize symbol for filenames (e.g., replacing ":" or "/" with "-")
	safeSymbol := strings.ReplaceAll(symbol, ":", "-")
	filename := fmt.Sprintf("%s.csv", strings.ToUpper(safeSymbol))

	return filepath.Join(p.dir, filename)
}
