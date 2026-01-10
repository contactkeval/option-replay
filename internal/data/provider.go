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
	GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error)
	GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error)
	GetBars(underlying string, fromDate, toDate time.Time) ([]Bar, error)
	GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optType string, openDate time.Time) (float64, error)
	GetRelevantExpiries(underlying string, fromDate, toDate time.Time) ([]time.Time, error)
	RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64
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
	ExpiryDate time.Time
	Strike     float64
	Type       string // "call" or "put"
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
