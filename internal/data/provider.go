package data

import "time"

// Provider supplies market data
type Provider interface {
	Secondary() Provider
	GetContracts(ticker string, strike float64, start, end time.Time) ([]OptionContract, error)
	GetDailyBars(symbol string, from, to time.Time) ([]Bar, error)
	GetOptionMidPrice(symbol string, strike float64, expiry time.Time, optType string) (float64, error)
	GetRelevantExpiries(underlying string, from, to time.Time) ([]time.Time, error)
	RoundToNearestStrike(underlying string, price float64, openDate, expiryDate time.Time) float64
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
