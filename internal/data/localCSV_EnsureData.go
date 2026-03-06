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

func (p *LocalFileDataProvider) EnsureLocalData(symbol string, startDate, endDate time.Time) error {
	records, err := p.loadManifest()
	if err != nil {
		return fmt.Errorf("ensure local: %w", err)
	}

	record, exists := records[symbol]
	isOption := strings.HasPrefix(symbol, "O:")
	now := time.Now()

	var updatedRecord DataRecord
	if exists {
		updatedRecord, err = p.handleExistingRecord(symbol, record, isOption, startDate, endDate, now)
	} else {
		updatedRecord, err = p.handleNewRecord(symbol, isOption, startDate, endDate, now)
	}

	if err != nil {
		return err
	}

	records[symbol] = updatedRecord
	return p.saveManifest(records)
}

// --- Specialized Handlers ---

func (p *LocalFileDataProvider) handleExistingRecord(
	symbol string,
	record DataRecord,
	isOption bool,
	startDate, endDate, asOfTime time.Time,
) (DataRecord, error) {
	// Options: Only extend the end date to today if needed
	if isOption {
		if !endDate.IsZero() && endDate.After(record.LastDate) {
			logger.Infof("Option %s: Extending data to today", symbol)
			if err := p.fetchAndAppend(symbol, record.LastDate, asOfTime); err != nil {
				return record, err
			}
			record.LastDate = asOfTime
		}
		return record, nil
	}

	// Stocks/Indices: Check both historical and recent gaps
	if startDate.Before(record.FirstDate) {
		logger.Infof("Symbol %s: Fetching historical gap", symbol)
		if err := p.fetchAndAppend(symbol, startDate, record.FirstDate); err != nil {
			return record, err
		}
		record.FirstDate = startDate
	}

	if endDate.After(record.LastDate) {
		logger.Infof("Symbol %s: Fetching recent gap", symbol)
		if err := p.fetchAndAppend(symbol, record.LastDate, asOfTime); err != nil {
			return record, err
		}
		record.LastDate = asOfTime
	}

	return record, nil
}

func (p *LocalFileDataProvider) handleNewRecord(
	symbol string,
	isOption bool,
	startDate, endDate, asOfTime time.Time) (DataRecord, error) {
	if isOption {
		return p.initializeOptionRecord(symbol, startDate, asOfTime)
	}

	// Standard Stock/Index initialization
	if err := p.fetchAndAppend(symbol, startDate, endDate); err != nil {
		return DataRecord{}, err
	}
	return DataRecord{
		Symbol:    symbol,
		FirstDate: startDate,
		LastDate:  endDate,
	}, nil
}

func (p *LocalFileDataProvider) initializeOptionRecord(symbol string, requestedStart, asOfTime time.Time) (DataRecord, error) {
	expiryDate := p.parseExpiryFromSymbol(symbol)
	if expiryDate.IsZero() {
		return DataRecord{}, fmt.Errorf("failed to parse expiry from %s", symbol)
	}

	fetchStart := expiryDate.AddDate(-2, 0, 0)
	if err := p.fetchAndAppend(symbol, fetchStart, asOfTime); err != nil {
		return DataRecord{}, err
	}

	newRec := DataRecord{Symbol: symbol}
	// firstDate added only if expiry within 6 months from requested start
	if expiryDate.Before(requestedStart.AddDate(0, 6, 0)) {
		newRec.FirstDate = fetchStart
	}
	// lastDate added only if expiry is in the future
	if expiryDate.After(asOfTime) {
		newRec.LastDate = asOfTime
	}
	return newRec, nil
}

func (localFileDataProv *LocalFileDataProvider) RunMaintenancePipeline() error {
	records, err := localFileDataProv.loadManifest()
	if err != nil {
		return err
	}

	now := time.Now()
	twoYearsAgo := now.AddDate(-2, 0, 0)
	oneMonthAgo := now.AddDate(0, -1, 0)

	for symbol, record := range records {
		isOption := strings.HasPrefix(symbol, "O:")
		updated := false

		if isOption {
			// Option: Update to today if last date is older than 1 month
			if record.LastDate.Before(oneMonthAgo) {
				if err := localFileDataProv.fetchAndAppend(symbol, record.LastDate, now); err == nil {
					record.LastDate = now
					updated = true
				}
			}
		} else {
			// Stock/Index:
			// Fill historical gap if data starts after 2 years ago
			if record.FirstDate.After(twoYearsAgo) {
				if err := localFileDataProv.fetchAndAppend(symbol, twoYearsAgo, record.FirstDate); err == nil {
					record.FirstDate = twoYearsAgo
					updated = true
				}
			}
			// Fill recent gap if data is older than 1 month
			if record.LastDate.Before(oneMonthAgo) {
				if err := localFileDataProv.fetchAndAppend(symbol, record.LastDate, now); err == nil {
					record.LastDate = now
					updated = true
				}
			}
		}

		if updated {
			records[symbol] = record
		}
	}

	return localFileDataProv.saveManifest(records)
}
