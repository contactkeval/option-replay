package data

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// PolygonDataProvider implements Data Provider using Polygon.io API.
type PolygonDataProvider struct {
	apiKey    string
	client    *http.Client
	secondary Provider
}

// NewPolygonDataProvider creates a new instance of PolygonDataProvider with the specified API key.
// It initializes an HTTP client with a 30-second timeout.
// Returns a Provider interface implementation for interacting with Polygon data services.
func NewPolygonDataProvider(apiKey string) Provider {
	return &PolygonDataProvider{apiKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

// GetSecondary returns the secondary data provider associated with the PolygonDataProvider.
// This can be used to access backup or alternative data sources.
func (polygonDataProv *PolygonDataProvider) GetSecondary() Provider {
	return polygonDataProv.secondary
}

// SetSecondary sets the secondary data provider for the PolygonDataProvider.
// This allows the PolygonDataProvider to use an alternative provider for data retrieval
// when the primary source is unavailable or insufficient.
//
// secondary: The Provider instance to be used as the secondary data source.
func (polygonDataProv *PolygonDataProvider) SetSecondary(secondary Provider) {
	polygonDataProv.secondary = secondary
}

// Set retrieves the at-the-money (ATM) option prices for the specified underlying asset.
// It takes the underlying symbol, expiry date, open date, and the as-of price as input parameters.
// Returns the strike price, call option price, put option price, and an error if any occurs during retrieval.
func (polygonDataProv *PolygonDataProvider) Set(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {
	return polygonDataProv.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
}

// GetATMOptionPrices retrieves the at-the-money (ATM) option prices for a given underlying asset.
// It queries the Polygon API for a snapshot of option data, finds the strike price closest to the provided asOfPrice,
// and returns the strike, call price, put price, and any error encountered.
// The call and put prices are calculated as the average of their ask and bid prices if both are available.
// If no suitable option data is found, an error is returned.
//
// Parameters:
//
//	underlying   - The symbol of the underlying asset (e.g., "AAPL").
//	openDate     - The date for which to retrieve option prices (unused in this implementation).
//	asOfPrice    - The price used to determine the ATM strike.
//
// Returns:
//
//	strike       - The strike price closest to asOfPrice.
//	callPrice    - The average call option price at the ATM strike.
//	putPrice     - The average put option price at the ATM strike.
//	err          - Any error encountered during the process.
func (polygonDataProv *PolygonDataProvider) GetATMOptionPrices(
	underlying string,
	_, _ time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {
	// Try snapshot v3; this requires that your plan supports option snapshot access.
	url := fmt.Sprintf("https://api.polygon.io/v3/snapshot/underlying/%s?apiKey=%s", underlying, polygonDataProv.apiKey)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := polygonDataProv.client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, 0, 0, fmt.Errorf("polygon snapshot status %d", resp.StatusCode)
	}
	var res struct {
		Day struct {
			Close float64 `json:"c"`
		} `json:"day"`
		Options struct {
			Strikes []struct {
				Strike float64 `json:"strike"`
				Call   struct {
					Ask float64 `json:"ask"`
					Bid float64 `json:"bid"`
				} `json:"call"`
				Put struct {
					Ask float64 `json:"ask"`
					Bid float64 `json:"bid"`
				} `json:"put"`
			} `json:"strikes"`
		} `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, 0, 0, err
	}
	strike = math.Round(asOfPrice*100) / 100
	var closestStrikeData *struct {
		Strike float64 `json:"strike"`
		Call   struct {
			Ask float64 `json:"ask"`
			Bid float64 `json:"bid"`
		} `json:"call"`
		Put struct {
			Ask float64 `json:"ask"`
			Bid float64 `json:"bid"`
		} `json:"put"`
	}
	minDiff := math.MaxFloat64
	for _, strike := range res.Options.Strikes {
		diff := math.Abs(strike.Strike - asOfPrice)
		if diff < minDiff {
			minDiff = diff
			strike := strike // capture range variable for pointer use in loop (Go ≤ 1.21 compatibility consideration - not strictly necessary in Go 1.21+ but good practice to avoid potential issues with closures and loop variables in older Go versions and to ensure clarity that we are taking the address of the current iteration's value not the loop variable itself which can lead to bugs if not done correctly - see)
			closestStrikeData = &strike
		}
	}
	if closestStrikeData == nil {
		return 0, 0, 0, fmt.Errorf("no option data found for %s", underlying)
	}
	strike = closestStrikeData.Strike
	if closestStrikeData.Call.Ask > 0 && closestStrikeData.Call.Bid > 0 {
		callPrice = (closestStrikeData.Call.Ask + closestStrikeData.Call.Bid) / 2.0
	} else {
		callPrice = 0
	}
	if closestStrikeData.Put.Ask > 0 && closestStrikeData.Put.Bid > 0 {
		putPrice = (closestStrikeData.Put.Ask + closestStrikeData.Put.Bid) / 2.0
	} else {
		putPrice = 0
	}
	return strike, callPrice, putPrice, nil
}

func (polygonDataProv *PolygonDataProvider) GetContracts(
	_ string,
	_ float64,
	_, _, _ time.Time,
) ([]OptionContract, error) {
	// Polygon does not provide an endpoint to list option contracts by strike.
	// This method is not implemented.
	return nil, fmt.Errorf("GetContracts not implemented for PolygonProvider")
}

// GetBars retrieves historical bar data (OHLCV) for a specified underlying ticker symbol
// from the Polygon API within a given date range and timespan.
//
// Parameters:
//   - underlying: the symbol of the underlying asset.
//   - strike: the strike price of the option contract.
//   - expiryDate: the expiration date of the option contract.
//   - fromDate: the start date of the search range.
//   - toDate: the end date of the search range.
//
// Returns:
//   - a slice of OptionContract (always nil).
//   - an error indicating the method is not implemented.
func (polygonDataProv *PolygonDataProvider) GetBars(
	underlying string,
	fromDate, toDate time.Time,
	timespan int,
	multiplier string,
) ([]Bar, error) {
	base := "https://api.polygon.io"
	url := fmt.Sprintf("%s/v2/aggs/ticker/%s/range/%d/%s/%s/%s?adjusted=true&sort=asc&limit=50000&apiKey=%s",
		base, underlying, timespan, multiplier, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"), polygonDataProv.apiKey)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := polygonDataProv.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("polygon aggs status %d", resp.StatusCode)
	}
	var body struct {
		Results []struct {
			Time   int64   `json:"t"`
			Open   float64 `json:"o"`
			High   float64 `json:"h"`
			Low    float64 `json:"l"`
			Close  float64 `json:"c"`
			Volume float64 `json:"v"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Bar, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Bar{Date: time.UnixMilli(r.Time).UTC(), Open: r.Open, High: r.High, Low: r.Low, Close: r.Close, Volume: r.Volume})
	}
	return out, nil
}

// GetOptionPrice retrieves the price of an option contract for the specified underlying asset,
// strike price, expiry date, option type, and open date. It attempts to fetch the option price
// using Polygon's snapshot API. If both ask and bid prices are available, it returns their average;
// otherwise, it returns the last traded price if available. Returns an error if no usable price
// is found or if the API request fails.
//
// Parameters:
//   - underlying: The symbol of the underlying asset (e.g., "AAPL").
//   - strike: The strike price of the option.
//   - expiryDate: The expiration date of the option.
//   - optType: The option type ("call" or "put").
//   - openDate: The date when the option was opened.
//
// Returns:
//   - float64: The calculated option price.
//   - error: An error if the price cannot be retrieved or parsed.
func (polygonDataProv *PolygonDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optType string,
	_ time.Time,
) (float64, error) {
	// Try snapshot v3; this requires that your plan supports option snapshot access.
	symbol := polygonDataProv.OptionSymbolFromParts(underlying, expiryDate, optType, strike)
	url := fmt.Sprintf("https://api.polygon.io/v3/snapshot/options/%s?apiKey=%s", symbol, polygonDataProv.apiKey)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := polygonDataProv.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("polygon options snapshot status %d", resp.StatusCode)
	}
	var res struct {
		Min struct {
			Ask float64 `json:"ask"`
			Bid float64 `json:"bid"`
		} `json:"min"`
		Last struct {
			Price float64 `json:"price"`
		} `json:"last"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}
	if res.Min.Ask > 0 && res.Min.Bid > 0 {
		return (res.Min.Ask + res.Min.Bid) / 2.0, nil
	}
	if res.Last.Price > 0 {
		return res.Last.Price, nil
	}
	return 0, fmt.Errorf("no usable option price for %s", symbol)
}

// GetRelevantExpiries retrieves a list of relevant option expiry dates for the specified ticker
// within the provided date range [fromDate, toDate]. If a secondary data provider is configured,
// the request is delegated to it. Otherwise, an error is returned indicating that the method is
// not implemented for PolygonProvider.
//
// Parameters:
//   - ticker: The symbol for which to fetch expiry dates.
//   - fromDate: The start of the date range.
//   - toDate: The end of the date range.
//
// Returns:
//   - A slice of time.Time representing the relevant expiry dates.
//   - An error if the operation is not supported or fails.
func (polygonDataProv *PolygonDataProvider) GetRelevantExpiries(
	ticker string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {
	if polygonDataProv.secondary != nil {
		return polygonDataProv.secondary.GetRelevantExpiries(ticker, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for PolygonProvider")
}

// RoundToNearestStrike rounds the given asOfPrice to the nearest valid strike price interval
// for the specified underlying asset. The interval is determined by the underlying's strike
// price configuration. The function takes the underlying symbol, expiry date, open date, and
// the price as of which to round, and returns the nearest strike price.
//
// Parameters:
//   - underlying: the symbol of the underlying asset.
//   - expiryDate: the expiration date of the option.
//   - openDate: the date the option was opened.
//   - asOfPrice: the price to round to the nearest strike.
//
// Returns:
//   - float64: the nearest strike price rounded based on the underlying's interval.
func (polygonDataProv *PolygonDataProvider) RoundToNearestStrike(
	underlying string,
	_ time.Time, _ time.Time,
	asOfPrice float64,
) float64 {
	intervals := polygonDataProv.getIntervals(underlying)
	return math.Round(asOfPrice/intervals) * intervals
}

// OptionSymbolFromParts: improved OCC-like formatter (best-effort)
func (polygonDataProv *PolygonDataProvider) OptionSymbolFromParts(
	underlying string,
	expiryDate time.Time,
	optionType string,
	strike float64,
) string {
	// OCC: <root><YYMMDD><C|P><strike*1000 padded to 8 digits>
	expDt := expiryDate.UTC().Format("060102")
	optType := "C"
	if opt := strings.ToLower(optionType); opt == "put" || opt == "p" {
		optType = "P"
	}
	strikeStr := fmt.Sprintf("%08d", int(math.Round(strike*1000)))
	return fmt.Sprintf("O:%s%s%s%s", strings.ToUpper(underlying), expDt, optType, strikeStr)
}

// parseExpiryFromSymbol extracts the expiry date from an OCC-formatted option symbol.
// It removes the "O:" prefix if present, validates the symbol length, and parses the
// 6-digit expiry date (YYMMDD) from the symbol. Returns the parsed expiry as time.Time,
// or zero time if parsing fails.
func (polygonDataProv *PolygonDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	// Strip the "O:" prefix if present
	cleanSym := strings.TrimPrefix(symbol, "O:")

	// The OCC format suffix is fixed length:
	// YYMMDD (6) + Type (1) + Strike (8) = 15 characters
	if len(cleanSym) < 15 {
		logger.Errorf("invalid option symbol length: %s", symbol)
		return time.Time{}
	}

	// Extract the 6 digits for the date.
	// It starts at (Total Length - 15) and ends 6 characters later.
	datePart := cleanSym[len(cleanSym)-15 : len(cleanSym)-9]

	expiry, err := time.Parse("060102", datePart)
	if err != nil {
		logger.Errorf("failed to parse expiry from %s: %v", symbol, err)
		return time.Time{}
	}

	return expiry
}

// getIntervals returns the interval value for the specified underlying asset.
// Currently, it returns a fixed value of 50.0. In the future, this function should be
// updated to read and determine proper intervals based on the underlying asset.
//
// Parameters:
//
//	underlying - the symbol or identifier of the underlying asset.
//
// Returns:
//
//	float64 - the interval value for the given underlying asset.
func (polygonDataProv *PolygonDataProvider) getIntervals(_ string) float64 {
	return 50.0 // TODO: implement proper intervals reading
}
