package data

import (
	"fmt"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

type DataRecord struct {
	Symbol    string    `csv:"symbol"`
	FirstDate time.Time `csv:"first_date"`
	LastDate  time.Time `csv:"last_date"`
}

func (localFileDataProv *localFileDataProvider) EnsureLocalData(symbol string, startDate, endDate time.Time) error {
	// 1. Identify instrument type using your prefix convention
	isOption := strings.HasPrefix(symbol, "O:")

	// 2. Open list file (manifest)
	records, err := localFileDataProv.loadManifest()
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
				if err := localFileDataProv.fetchAndAppend(symbol, record.LastDate, now); err != nil {
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
				if err := localFileDataProv.fetchAndAppend(symbol, startDate, record.FirstDate); err != nil {
					return err
				}
				record.FirstDate = startDate
			}
			// if endDate after lastDate, fetch lastDate -> today
			if endDate.After(record.LastDate) {
				logger.Infof("Symbol %s: Fetching recent gap", symbol)
				if err := localFileDataProv.fetchAndAppend(symbol, record.LastDate, now); err != nil {
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
			expiryDate := localFileDataProv.parseExpiryFromSymbol(symbol)
			if expiryDate.IsZero() {
				return fmt.Errorf("failed to parse expiry from %s", symbol)
			}

			fetchStart := expiryDate.AddDate(-2, 0, 0)
			if err := localFileDataProv.fetchAndAppend(symbol, fetchStart, now); err != nil {
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
			if err := localFileDataProv.fetchAndAppend(symbol, startDate, endDate); err != nil {
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
	return localFileDataProv.saveManifest(records)
}

func (localFileDataProv *localFileDataProvider) RunMaintenancePipeline() error {
	records, err := localFileDataProv.loadManifest()
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
				if err := localFileDataProv.fetchAndAppend(sym, rec.LastDate, now); err == nil {
					rec.LastDate = now
					updated = true
				}
			}
		} else {
			// Stock/Index:
			// Fill historical gap if data starts after 2 years ago
			if rec.FirstDate.After(twoYearsAgo) {
				if err := localFileDataProv.fetchAndAppend(sym, twoYearsAgo, rec.FirstDate); err == nil {
					rec.FirstDate = twoYearsAgo
					updated = true
				}
			}
			// Fill recent gap if data is older than 1 month
			if rec.LastDate.Before(oneMonthAgo) {
				if err := localFileDataProv.fetchAndAppend(sym, rec.LastDate, now); err == nil {
					rec.LastDate = now
					updated = true
				}
			}
		}

		if updated {
			records[sym] = rec
		}
	}

	return localFileDataProv.saveManifest(records)
}
