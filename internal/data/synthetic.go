package data

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// synthDataProvider implements Data Provider generating synthetic data.
type synthDataProvider struct {
	secondary Provider
}

func NewSyntheticProvider() Provider { return &synthDataProvider{} }

func (synthDataProv *synthDataProvider) Secondary() Provider {
	return synthDataProv.secondary
}

func (synthDataProv *synthDataProvider) GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	strike = math.Round(asOfPrice*100) / 100
	callPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	putPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	return strike, callPrice, putPrice, nil
}

func (synthDataProv *synthDataProvider) GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetContracts(underlying, strike, expiryDate, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetContracts not implemented for SyntheticProvider")
}

func (synthDataProv *synthDataProvider) GetBars(underlying string, fromDate, toDate time.Time, timespan int, multiplier string) ([]Bar, error) {
	//TODO: support timespan and multiplier
	cur := fromDate
	price := 100.0 + float64(rand.Intn(200))
	var out []Bar
	for !cur.After(toDate) {
		if cur.Weekday() != time.Saturday && cur.Weekday() != time.Sunday {
			delta := rand.NormFloat64() * 0.01 * price
			open := price
			close := price + delta
			high := math.Max(open, close) + math.Abs(rand.NormFloat64()*0.3)
			low := math.Min(open, close) - math.Abs(rand.NormFloat64()*0.3)
			out = append(out, Bar{Date: cur, Open: open, High: high, Low: low, Close: close, Vol: float64(1000 + rand.Intn(5000))})
			price = close
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return out, nil
}

func (synthDataProv *synthDataProvider) GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optionType string, openDate time.Time) (float64, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetOptionPrice(underlying, strike, expiryDate, optionType, openDate)
	}
	return 0, fmt.Errorf("no option market data in synthetic provider")
}

func (synthDataProv *synthDataProvider) GetRelevantExpiries(ticker string, fromDate, toDate time.Time) ([]time.Time, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetRelevantExpiries(ticker, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for SyntheticProvider")
}

func (synthDataProv *synthDataProvider) RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64 {
	intervals := synthDataProv.getIntervals(underlying)
	return math.Round(asOfPrice/intervals) * intervals
}

func (synthDataProv *synthDataProvider) getIntervals(underlying string) float64 {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.getIntervals(underlying)
	}
	return 0 // default
}
