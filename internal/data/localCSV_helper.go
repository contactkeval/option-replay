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

func (p *localFileDataProvider) EnsureLocalData(symbol string, start, end time.Time) error {
	// 1. Identify instrument type
	isOption := strings.Contains(symbol, "-") // Logic depends on your naming convention

	// 2. Load manifest (helper function to read your list file)
	records, err := p.loadManifest()
	if err != nil {
		return fmt.Errorf("ensure local: %w", err)
	}

	record, exists := records[symbol]
	now := time.Now()

	if exists {
		// 3. Record Exists - Handle Incremental Fetching
		if isOption {
			// Options: No start date check. Fetch till today if end is beyond lastDate.
			if end.After(record.LastDate) {
				err = p.fetchAndAppend(symbol, record.LastDate, now)
				record.LastDate = now
			}
		} else {
			// Stocks/Indices: Check both directions
			if start.Before(record.FirstDate) {
				err = p.fetchAndAppend(symbol, start, record.FirstDate)
				record.FirstDate = start
			}
			if end.After(record.LastDate) {
				err = p.fetchAndAppend(symbol, record.LastDate, now)
				record.LastDate = now
			}
		}
	} else {
		// 4. Record Does Not Exist - Initial Fetch
		if isOption {
			// Fetch 2 years prior to expiry till today
			expiry := p.parseExpiryFromSymbol(symbol)
			fetchStart := expiry.AddDate(-2, 0, 0)
			err = p.fetchAndAppend(symbol, fetchStart, now)

			// Custom logic for adding to manifest
			newRecord := DataRecord{Symbol: symbol}
			if expiry.Before(start.AddDate(0, 6, 0)) {
				newRecord.FirstDate = fetchStart
			}
			if expiry.After(now) {
				newRecord.LastDate = now
			}
			records[symbol] = newRecord
		} else {
			// Stocks/Indices: Standard fetch
			err = p.fetchAndAppend(symbol, start, end)
			records[symbol] = DataRecord{Symbol: symbol, FirstDate: start, LastDate: end}
		}
	}

	if err != nil {
		return fmt.Errorf("failed to hydrate local data for %s: %w", symbol, err)
	}

	// 5. Update/Save Manifest
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
		isOption := strings.Contains(sym, "-")
		updated := false

		if isOption {
			// Options Pipeline: Check only the tail end
			if rec.LastDate.Before(oneMonthAgo) {
				logger.Infof("Pipeline: Updating option %s to today", sym)
				p.fetchAndAppend(sym, rec.LastDate, now)
				rec.LastDate = now
				updated = true
			}
		} else {
			// Stock/Index Pipeline: Check both ends
			// Fill historical gap if less than 2 years of data
			if rec.FirstDate.After(twoYearsAgo) {
				p.fetchAndAppend(sym, twoYearsAgo, rec.FirstDate)
				rec.FirstDate = twoYearsAgo
				updated = true
			}
			// Fill current gap if data is older than 1 month
			if rec.LastDate.Before(oneMonthAgo) {
				p.fetchAndAppend(sym, rec.LastDate, now)
				rec.LastDate = now
				updated = true
			}
		}

		if updated {
			records[sym] = rec
		}
	}

	return p.saveManifest(records)
}

func (p *localFileDataProvider) fetchAndAppend(symbol string, start, end time.Time) error {
	// 1. Fetch from secondary provider (e.g., polygon)
	newData, err := p.Secondary().GetBars(symbol, start, end, 1, "minute")
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
			fmt.Sprintf("%d", bar.Volume),
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

