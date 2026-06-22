package data

import (
	"errors"
	"math"
	"os"
	"sort"
	"time"
)

type DateMatchType string

// Provider supplies market data
type Provider interface {
	GetName() string
	GetSecondary() Provider
	SetSecondary(secondary Provider)
	GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error)
	GetContracts(underlying string, strike float64, fromDate, toDate time.Time) ([]OptionContract, error)
	GetBars(underlying string, fromDate, toDate time.Time, multiplier int, timespan string) ([]Bar, error)
	GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optionType string, openDate time.Time) (float64, error)
	GetRelevantExpiries(underlying string, fromDate, toDate time.Time) ([]time.Time, error)
	RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64
	OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string
	parseExpiryFromSymbol(symbol string) time.Time
	GetStrikeIntervals(underlying string, expiryDate time.Time) []float64
}

const (
	MatchExact   DateMatchType = "exact"   // must match exactly
	MatchHigher  DateMatchType = "higher"  // next available date after target
	MatchLower   DateMatchType = "lower"   // last available date before target
	MatchNearest DateMatchType = "nearest" // closest available date (default)
)

// Bar simplified OHLC
type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Count  uint32
}

type OptionContract struct {
	ExpiryDate time.Time
	Strike     float64
	Type       string // "call" or "put"
}

var ErrNoDataFound = errors.New("no data found for given symbol")

func GetLocalFileDataProvider() Provider {
	var dataProv Provider
	dataProv = NewLocalFileDataProvider("dir", dataProv)
	// dataProv.Secondary = NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")) // Massive data provider as secondary
	return dataProv
}

func GetMassiveDataProvider() Provider {
	return NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
}

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

func MatchBarDate(candidate time.Time, barDates []time.Time, mode DateMatchType) time.Time {

	// Search useful info
	var (
		exact  time.Time
		lower  time.Time
		higher time.Time
	)

	// Normalize candidate to date-only (strip time) for matching
	candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, time.UTC)

	// default to MatchNearest
	switch mode {
	case MatchExact, MatchHigher, MatchLower, MatchNearest:
		// ok
	default:
		mode = MatchNearest
	}

	sort.Slice(barDates, func(i, j int) bool { return barDates[i].Before(barDates[j]) })

	for _, barDate := range barDates {
		if barDate.Equal(candidate) {
			exact = barDate
		}
		if barDate.Before(candidate) {
			lower = barDate // will keep last ≤ d
		}
		if barDate.After(candidate) && higher.IsZero() {
			higher = barDate
		}
	}

	// if exact is found, return it immediately regardless of mode
	if !exact.IsZero() {
		return exact
	}

	switch mode {

	case MatchExact:
		return exact // may be zero → caller skips it

	case MatchLower:
		return lower // last date before d

	case MatchHigher:
		return higher // first date after d

	case MatchNearest:
		// choose whichever is closer
		switch {
		case !lower.IsZero() && !higher.IsZero():
			if candidate.Sub(lower) <= higher.Sub(candidate) {
				return lower
			}
			return higher
		case !lower.IsZero():
			return lower
		case !higher.IsZero():
			return higher
		}
	}

	return time.Time{} // nothing found
}

// Closest finds the closest float64 in a sorted slice to the target value using binary search (sort.Search).
func Closest(numList []float64, target float64) float64 {
	n := len(numList)
	if n == 0 {
		panic("empty list")
	}

	i := sort.Search(n, func(i int) bool {
		return numList[i] >= target
	})

	if i == 0 {
		return numList[0]
	}
	if i == n {
		return numList[n-1]
	}

	before := numList[i-1]
	after := numList[i]

	if math.Abs(before-target) < math.Abs(after-target) {
		return before
	}
	return after
}
