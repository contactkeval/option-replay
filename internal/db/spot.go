package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ContractTypeSpot marks underlying equity/index minute bars (not options).
const ContractTypeSpot = "spot"

// SpotContractExpiry is a sentinel far-future expiry so spot rows are never
// treated as expired options. Option selection queries exclude type=spot.
var SpotContractExpiry = time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)

// IsSpotContract reports whether c represents underlying spot minutes.
func IsSpotContract(c Contract) bool {
	return strings.EqualFold(c.Type, ContractTypeSpot)
}

// EnsureSpotContracts inserts missing spot rows for the given underlyings.
func (db *DB) EnsureSpotContracts(underlyings []string) error {
	symbols := uniqueUpperSymbols(underlyings)
	if len(symbols) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	seen := time.Now().UTC()
	for _, symbol := range symbols {
		if err := db.InsertContractIgnore(
			tx,
			symbol,
			SpotContractExpiry,
			0,
			ContractTypeSpot,
			-1,
			seen,
		); err != nil {
			return fmt.Errorf("ensure spot contract %s: %w", symbol, err)
		}
	}

	return tx.Commit()
}

// ListSpotContracts returns all non-archived spot contracts ordered by symbol.
func (db *DB) ListSpotContracts() ([]Contract, error) {
	rows, err := db.Query(`
		SELECT`+contractSelectCols+`
		FROM contracts
		WHERE archived = 0
			AND type = ?
		ORDER BY underlying ASC, serialNo ASC
	`, ContractTypeSpot)
	if err != nil {
		return nil, fmt.Errorf("list spot contracts: %w", err)
	}
	defer rows.Close()

	return scanContracts(rows)
}

// LatestSpotBarTime returns the newest candle_staging timestamp for the given
// underlying's spot contract. A zero time means no bars have been stored yet.
func (db *DB) LatestSpotBarTime(underlying string) (time.Time, error) {
	underlying = strings.ToUpper(strings.TrimSpace(underlying))
	if underlying == "" {
		return time.Time{}, fmt.Errorf("empty underlying")
	}

	var candleTime sql.NullInt64
	err := db.QueryRow(`
		SELECT MAX(cs.candleTime)
		FROM candle_staging cs
		INNER JOIN contracts c ON c.serialNo = cs.serialNo
		WHERE c.archived = 0
			AND c.type = ?
			AND c.underlying = ?
	`, ContractTypeSpot, underlying).Scan(&candleTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("latest spot bar time for %s: %w", underlying, err)
	}
	if !candleTime.Valid || candleTime.Int64 <= 0 {
		return time.Time{}, nil
	}
	return time.UnixMilli(candleTime.Int64).UTC(), nil
}

// SpotBarsStale reports whether the probe underlying's newest stored spot bar
// is missing or older than maxAge.
func (db *DB) SpotBarsStale(probe string, maxAge time.Duration) (bool, time.Time, error) {
	last, err := db.LatestSpotBarTime(probe)
	if err != nil {
		return false, time.Time{}, err
	}
	if last.IsZero() {
		return true, last, nil
	}
	return time.Since(last) > maxAge, last, nil
}

func uniqueUpperSymbols(underlyings []string) []string {
	seen := make(map[string]struct{}, len(underlyings))
	out := make([]string, 0, len(underlyings))
	for _, raw := range underlyings {
		symbol := strings.ToUpper(strings.TrimSpace(raw))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}
