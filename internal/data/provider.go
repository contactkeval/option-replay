package data

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

type DateMatchType string

// Provider supplies market data
type Provider interface {
	Secondary() Provider
	GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error)
	GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error)
	GetBars(underlying string, fromDate, toDate time.Time, timespan int, multiplier string) ([]Bar, error)
	GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optType string, openDate time.Time) (float64, error)
	GetRelevantExpiries(underlying string, fromDate, toDate time.Time) ([]time.Time, error)
	RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64
	getIntervals(underlying string) float64
}

const (
	MatchExact   DateMatchType = "exact"   // must match exactly
	MatchHigher  DateMatchType = "higher"  // next available date after target
	MatchLower   DateMatchType = "lower"   // last available date before target
	MatchNearest DateMatchType = "nearest" // closest available date (default)
)

// Bar simplified OHLC
type Bar struct {
	Date  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
	Vol   float64
	Count int64
}

type OptionContract struct {
	ExpiryDate time.Time
	Strike     float64
	Type       string // "call" or "put"
}

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

// OptionSymbolFromParts: improved OCC-like formatter (best-effort)
func OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	// OCC: <root><YYMMDD><C|P><strike*1000 padded to 8 digits>
	expDt := expiryDate.UTC().Format("060102")
	optType := "C"
	if strings.ToLower(optionType) == "put" || strings.ToLower(optionType) == "p" {
		optType = "P"
	}
	strikeInt := int(math.Round(strike * 1000))
	strFmt := fmt.Sprintf("%08d", strikeInt)
	return fmt.Sprintf("O:%s%s%s%s", strings.ToUpper(underlying), expDt, optType, strFmt)
}

func MatchBarDate(d time.Time, dates []time.Time, mode DateMatchType) time.Time {

	// Search useful info
	var (
		exact  time.Time
		lower  time.Time
		higher time.Time
	)

	// default to MatchNearest
	switch mode {
	case MatchExact, MatchHigher, MatchLower, MatchNearest:
		// ok
	default:
		mode = MatchNearest
	}

	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	for _, dt := range dates {
		if dt.Equal(d) {
			exact = dt
		}
		if dt.Before(d) {
			lower = dt // will keep last ≤ d
		}
		if dt.After(d) && higher.IsZero() {
			higher = dt
		}
	}

	switch mode {

	case MatchExact:
		return exact // may be zero → caller skips it

	case MatchLower:
		return lower // last date before d

	case MatchHigher:
		return higher // first date after d

	case MatchNearest:
		if !exact.IsZero() {
			return exact
		}
		// choose whichever is closer
		switch {
		case !lower.IsZero() && !higher.IsZero():
			if d.Sub(lower) <= higher.Sub(d) {
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
