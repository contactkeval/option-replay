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

// GetSecondary returns the secondary data provider associated with the synthDataProvider.
// It implements the Provider interface.
func (synthDataProv *synthDataProvider) GetSecondary() Provider {
	return synthDataProv.secondary
}

// SetSecondary sets the secondary data provider to be used by the synthDataProvider.
// This allows the synthDataProvider to delegate data retrieval to another Provider.
func (synthDataProv *synthDataProvider) SetSecondary(secondary Provider) {
	synthDataProv.secondary = secondary
}

// GetATMOptionPrices returns the at-the-money (ATM) option prices for a given underlying asset.
// It calculates the strike price based on the provided as-of price, and generates synthetic call and put prices.
// If a secondary data provider is available, it delegates the request to it.
// Parameters:
//   - underlying: the symbol of the underlying asset
//   - expiryDate: the expiration date of the option
//   - openDate: the date the option is opened
//   - asOfPrice: the price of the underlying asset as of the specified date
//
// Returns:
//   - strike: the calculated ATM strike price
//   - callPrice: the synthetic ATM call option price
//   - putPrice: the synthetic ATM put option price
//   - err: error if any occurred during price retrieval
func (synthDataProv *synthDataProvider) GetATMOptionPrices(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	strike = math.Round(asOfPrice*100) / 100
	callPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	putPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	return strike, callPrice, putPrice, nil
}

// GetContracts retrieves option contracts for a given underlying asset, strike price, and expiry date
// within the specified date range (fromDate to toDate). If a secondary data provider is available,
// it delegates the request to it. Returns a slice of OptionContract and an error if the operation fails.
//
// Parameters:
//   - underlying: the symbol of the underlying asset
//   - strike: the strike price of the option
//   - expiryDate: the expiration date of the option contract
//   - fromDate: the start date for the contract search range
//   - toDate: the end date for the contract search range
//
// Returns:
//   - []OptionContract: a slice of matching option contracts
//   - error: error if contracts cannot be retrieved or not implemented
func (synthDataProv *synthDataProvider) GetContracts(
	underlying string,
	strike float64,
	expiryDate, fromDate, toDate time.Time,
) ([]OptionContract, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetContracts(underlying, strike, expiryDate, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetContracts not implemented for SyntheticProvider")
}

// GetBars generates synthetic bar data for a given underlying asset between two dates.
// It simulates daily price movements, skipping weekends, and returns a slice of Bar structs.
// Parameters:
//   - underlying: the symbol of the asset.
//   - fromDate: the start date for the data.
//   - toDate: the end date for the data.
//   - timespan: (currently unused) intended for specifying bar interval.
//   - multiplier: (currently unused) intended for specifying bar size.
//
// Returns:
//   - []Bar: a slice of synthetic bar data.
//   - error: always nil in current implementation.
func (synthDataProv *synthDataProvider) GetBars(
	underlying string,
	fromDate, toDate time.Time,
	timespan int,
	multiplier string,
) ([]Bar, error) {
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
			out = append(out, Bar{Date: cur, Open: open, High: high, Low: low, Close: close, Volume: float64(1000 + rand.Intn(5000))})
			price = close
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return out, nil
}

// GetOptionPrice retrieves the price of an option contract for the specified underlying asset,
// strike price, expiry date, option type, and open date. If a secondary data provider is available,
// it delegates the request to it. Returns the option price and an error if no market data is available.
//
// Parameters:
//   - underlying: the symbol of the underlying asset.
//   - strike: the strike price of the option.
//   - expiryDate: the expiration date of the option.
//   - optionType: the type of the option ("call" or "put").
//   - openDate: the date when the option is opened.
//
// Returns:
//   - float64: the price of the option.
//   - error: error if no market data is available.
func (synthDataProv *synthDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optionType string,
	openDate time.Time,
) (float64, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetOptionPrice(underlying, strike, expiryDate, optionType, openDate)
	}
	return 0, fmt.Errorf("no option market data in synthetic provider")
}

// GetRelevantExpiries returns a slice of option expiry dates for the given ticker symbol
// within the specified date range [fromDate, toDate]. If a secondary data provider is present,
// the request is delegated to it. Otherwise, an error is returned indicating that the method
// is not implemented for SyntheticProvider.
//
// Parameters:
//
//	ticker   - the symbol for which to retrieve expiry dates
//	fromDate - the start of the date range
//	toDate   - the end of the date range
//
// Returns:
//
//	[]time.Time - slice of relevant expiry dates
//	error       - error if expiries cannot be retrieved
func (synthDataProv *synthDataProvider) GetRelevantExpiries(
	ticker string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {
	if synthDataProv.secondary != nil {
		return synthDataProv.secondary.GetRelevantExpiries(ticker, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for SyntheticProvider")
}

// RoundToNearestStrike rounds the given asOfPrice to the nearest strike price interval for the specified underlying asset.
// It determines the interval using the underlying asset and returns the strike price closest to asOfPrice.
// Parameters:
//   - underlying: the symbol of the underlying asset.
//   - expiryDate: the expiration date of the option (unused in this function).
//   - openDate: the date the option is opened (unused in this function).
//   - asOfPrice: the price to round to the nearest strike.
//
// Returns:
//   - The nearest strike price rounded based on the interval for the underlying asset.
func (synthDataProv *synthDataProvider) RoundToNearestStrike(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64) float64 {
	intervals := synthDataProv.getIntervals(underlying)
	return math.Round(asOfPrice/intervals) * intervals
}

// OptionSymbolFromParts generates an option symbol string by combining the underlying asset,
// expiry date, option type, and strike price into a standardized format.
// It takes the underlying asset symbol, expiration date, option type (e.g., "call" or "put"),
// and strike price as parameters and returns the formatted option symbol string.
func (synthDataProv *synthDataProvider) OptionSymbolFromParts(
	underlying string,
	expiryDate time.Time,
	optionType string,
	strike float64,
) string {
	// Simple formatter: <UNDERLYING>-<YYMMDD>-<C|P>-<STRIKE>
	return synthDataProv.GetSecondary().OptionSymbolFromParts(underlying, expiryDate, optionType, strike)
}

// parseExpiryFromSymbol parses the expiration date from an options symbol by delegating
// to the secondary data provider's parseExpiryFromSymbol method.
// It returns the parsed expiration date as a time.Time value.
func (synthDataProv *synthDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	return synthDataProv.GetSecondary().parseExpiryFromSymbol(symbol)
}

// getIntervals returns the interval value for the given underlying asset.
// If a secondary data provider is available, it delegates to that provider.
// Otherwise, it returns the default value of 0.
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
