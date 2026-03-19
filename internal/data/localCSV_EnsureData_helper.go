package data

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// parseFloat safely converts a string to float64, logging an error on failure.
func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		logger.Tracef("failed to parse float '%s': %v", s, err)
		return 0.0
	}
	return val
}

// fetchAndAppend fetches bars for the specified symbol from the secondary provider for the
// inclusive date range [startDate, endDate] and appends them to the provider's local CSV file
// for that symbol. The file is created if it does not exist; if the file is empty a header row
// ("date","open","high","low","close","volume") is written before appending. Each bar is written
// as a CSV row using RFC3339 for the timestamp, prices formatted to two decimal places, and
// volume formatted as an integer. Any error encountered while fetching or writing is returned.
func (localFileDataProv *LocalFileDataProvider) fetchAndAppend(symbol string, startDate, endDate time.Time) error {
	// 1. Fetch from secondary provider (e.g. massive)
	newData, err := localFileDataProv.GetSecondary().GetBars(symbol, startDate, endDate, 1, "minute")
	if err != nil {
		return err
	}

	// 2. Open local file in Append mode
	f, err := os.OpenFile(localFileDataProv.getSymbolPath(symbol), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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
func (localFileDataProv *LocalFileDataProvider) loadManifest() (map[string]DataRecord, error) {
	path := localFileDataProv.getManifestPath() // e.g., "data/manifest.csv"
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
func (localFileDataProv *LocalFileDataProvider) saveManifest(records map[string]DataRecord) error {
	file, err := os.Create(localFileDataProv.getManifestPath())
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
func (localFileDataProv *LocalFileDataProvider) getManifestPath() string {
	// p.BaseDir is likely something like "./data" or "/var/lib/option-replay"
	return filepath.Join(localFileDataProv.dir, localFileDataProv.GetSecondary().GetName(), "manifest.csv")
}

// getSymbolPath returns the path for a specific instrument's data file.
func (localFileDataProv *LocalFileDataProvider) getSymbolPath(symbol string) string {
	// Sanitize symbol for filenames (e.g., replacing ":" or "/" with "-")
	safeSymbol := strings.ReplaceAll(symbol, ":", "-")
	filename := fmt.Sprintf("%s.csv", strings.ToUpper(safeSymbol))

	return filepath.Join(localFileDataProv.dir, localFileDataProv.GetSecondary().GetName(), filename)
}
