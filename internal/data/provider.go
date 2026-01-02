package data

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Provider supplies market data
type Provider interface {
	Secondary() Provider
	GetContracts(ticker string, strike float64, start, end, expiryDt time.Time) ([]OptionContract, error)
	GetDailyBars(underlying string, from, to time.Time) ([]Bar, error)
	GetOptionMidPrice(underlying string, strike float64, expiry time.Time, optType string) (float64, error)
	GetRelevantExpiries(underlying string, from, to time.Time) ([]time.Time, error)
	RoundToNearestStrike(underlying string, asOfPrice float64, openDate, expiryDate time.Time) float64
	getIntervals(underlying string) float64
}

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
	ExpirationDate time.Time
	Strike         float64
	Type           string // "call" or "put"
}

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

// OptionSymbolFromParts: improved OCC-like formatter (best-effort)
func OptionSymbolFromParts(underlying string, expiration time.Time, optType string, strike float64) string {
	// OCC: <root><YYMMDD><C|P><strike*1000 padded to 8 digits>
	expDt := expiration.UTC().Format("060102")
	t := "C"
	if strings.ToLower(optType) == "put" || strings.ToLower(optType) == "p" {
		t = "P"
	}
	strikeInt := int(math.Round(strike * 1000))
	strFmt := fmt.Sprintf("%08d", strikeInt)
	return fmt.Sprintf("%s%s%s%s", strings.ToUpper(underlying), expDt, t, strFmt)
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
