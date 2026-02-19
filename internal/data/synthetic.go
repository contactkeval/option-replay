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
			out = append(out, Bar{Date: cur, Open: open, High: high, Low: low, Close: close, Volume: uint32(1000 + rand.Intn(5000))})
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

func (synthDataProv *synthDataProvider) OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	// Simple formatter: <UNDERLYING>-<YYMMDD>-<C|P>-<STRIKE>
	return synthDataProv.Secondary().OptionSymbolFromParts(underlying, expiryDate, optionType, strike)
}

func (synthDataProv *synthDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	return synthDataProv.Secondary().parseExpiryFromSymbol(symbol)
}

func (synthDataProv *synthDataProvider) getIntervals(underlying string) float64 {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.getIntervals(underlying)
	}
	return 0 // default
}

// extractCloses transforms a slice of data bars into a simple slice of close prices.
func ExtractCloses(bars []Bar) []float64 {
	var closes []float64
	for _, b := range bars {
		closes = append(closes, b.Close)
	}
	return closes
}

// TODO: move to a data.BlackScholes package
// AnnualizedVolatility calculates the standard deviation of logarithmic returns normalized for a 252-day trading year.
func AnnualizedVolatility(closes []float64) float64 {
	if len(closes) < 2 {
		return 0.30
	}
	var rets []float64
	for i := 1; i < len(closes); i++ {
		rets = append(rets, math.Log(closes[i]/closes[i-1]))
	}
	mean := 0.0
	for _, v := range rets {
		mean += v
	}
	mean /= float64(len(rets))
	sd := 0.0
	for _, v := range rets {
		sd += (v - mean) * (v - mean)
	}
	sd = math.Sqrt(sd / float64(len(rets)-1))
	return sd * math.Sqrt(252.0)
}
