// Package data provides market data provider implementations.
//
// This file contains a Massive-backed Provider implementation that retrieves
// option contracts, bars, expiries, and option prices via Massive HTTP APIs.
//
// Design notes:
//   - Uses raw HTTP calls instead of the official Massive SDK
//   - Supports pagination, rate-limiting retries, and fallback providers
//   - Logging is intentionally verbose at Debug/Trace levels for diagnostics
//
// IMPORTANT:
//
//	This file intentionally preserves all existing logic and behavior.
//	Changes in this version are limited to documentation, comments,
//	naming consistency, and cosmetic formatting only.
package data

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// massiveDataProvider implements the Provider interface using Massive APIs.
type massiveDataProvider struct {
	// APIKey used for authenticating requests with Massive.
	APIKey string

	// Client is the HTTP client used to make API requests.
	Client *http.Client

	// BaseURL is the root endpoint for Massive APIs
	// (e.g., https://api.massive.com).
	BaseURL string

	// secondary is an optional fallback provider.
	secondary Provider
}

// massiveContract represents a single option contract
// returned by Massive's contracts reference endpoint.
type massiveContract struct {
	CFI               string  `json:"cfi"`
	ContractType      string  `json:"contract_type"`
	ExerciseStyle     string  `json:"exercise_style"`
	ExpiryDate        string  `json:"expiration_date"`
	PrimaryExchange   string  `json:"primary_exchange"`
	SharesPerContract int     `json:"shares_per_contract"`
	StrikePrice       float64 `json:"strike_price"`
	Ticker            string  `json:"ticker"`
	UnderlyingTicker  string  `json:"underlying_ticker"`
}

// massiveContractsResp models the paginated response
// returned by Massive's option contracts API.
type massiveContractsResp struct {
	Results   []massiveContract `json:"results"`
	Status    string            `json:"status"`
	RequestID string            `json:"request_id"`
	NextURL   string            `json:"next_url"`
}

// NewMassiveDataProvider constructs a Massive-backed data provider.
//
// It initializes an HTTP client with sensible defaults for:
//   - timeouts
//   - connection pooling
//   - HTTP/2 support
//   - gzip decompression
//
// Parameters:
//   - apiKey: Massive API key for authentication
//
// Returns:
//   - *massiveDataProvider: initialized provider instance
func NewMassiveDataProvider(apiKey string) *massiveDataProvider {
	logger.Infof("initializing Massive data provider")

	return &massiveDataProvider{
		APIKey: apiKey,
		Client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    false, // must be false to enable gzip auto-decompression
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		BaseURL: "https://api.massive.com",
	}
}

// Secondary returns the configured secondary Provider, if any.
func (massiveDataProv *massiveDataProvider) Secondary() Provider {
	return massiveDataProv.secondary
}

// GetATMOptionPrices returns the ATM strike along with call and put prices.
//
// NOTE:
//   - This implementation currently generates synthetic prices.
//   - If a secondary provider is configured, the request is delegated.
//
// Parameters:
//   - underlying: underlying symbol
//   - expiryDate: option expiration date
//   - openDate: trading date
//   - asOfPrice: underlying spot price
//
// Returns:
//   - strike: ATM strike
//   - callPrice: simulated call premium
//   - putPrice: simulated put premium
//   - err: error if retrieval fails
func (massiveDataProv *massiveDataProvider) GetATMOptionPrices(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {

	logger.Debugf(
		"ATM prices request: %s price=%.2f expiry=%s",
		underlying,
		asOfPrice,
		expiryDate.Format("2006-01-02"),
	)

	//TODO: implement real ATM option price fetching from Massive API
	strike = math.Round(asOfPrice*100) / 100
	callPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	putPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)

	logger.Tracef(
		"ATM resolved strike=%.2f call=%.2f put=%.2f",
		strike, callPrice, putPrice,
	)

	// Delegate to secondary provider if present
	if massiveDataProv.secondary != nil {
		logger.Tracef("delegating ATM pricing to secondary provider")
		return massiveDataProv.secondary.GetATMOptionPrices(
			underlying, expiryDate, openDate, asOfPrice,
		)
	}

	return strike, callPrice, putPrice, nil
}

// GetContracts retrieves option contracts matching the supplied filters.
//
// Parameters:
//   - underlying: underlying ticker symbol
//   - strike: specific strike (0 means all strikes)
//   - expiryDate: specific expiry (zero value enables range query)
//   - fromDate: expiry range start
//   - toDate: expiry range end
//
// Returns:
//   - []OptionContract: matching contracts
//   - error: if request or decoding fails
func (massiveDataProv *massiveDataProvider) GetContracts(
	underlying string,
	strike float64,
	expiryDate, fromDate, toDate time.Time,
) ([]OptionContract, error) {

	logger.Tracef(
		"fetching option contracts: %s strike=%.2f expiry=%s",
		underlying,
		strike,
		expiryDate.Format("2006-01-02"),
	)

	out := []OptionContract{}

	// Build base URL
	url, err := url.Parse(massiveDataProv.BaseURL + "/v3/reference/options/contracts")
	if err != nil {
		return nil, err
	}

	// Query parameters
	query := url.Query()
	query.Set("underlying_ticker", underlying)

	if strike > 0.0 {
		query.Set("strike_price", fmt.Sprintf("%.8g", strike))
	}

	if expiryDate.IsZero() {
		query.Set("expiration_date.lte", toDate.Format("2006-01-02"))
		query.Set("expiration_date.gte", fromDate.Format("2006-01-02"))
	} else {
		query.Set("expiration_date", expiryDate.Format("2006-01-02"))
	}

	query.Set("expired", "true")
	query.Set("limit", "1000")
	query.Set("apiKey", massiveDataProv.APIKey)

	url.RawQuery = query.Encode()
	reqURL := url.String()

	// Handle pagination
	for reqURL != "" {
		logger.Debugf("contracts request URL: %s", reqURL)

		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+massiveDataProv.APIKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "massive-client/1.0")

		resp, err := massiveDataProv.processGetRequest(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if len(body) == 0 {
			return nil, fmt.Errorf("empty response body")
		}

		if resp.StatusCode != http.StatusOK {
			var dbg struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &dbg)

			logger.Errorf(
				"massive contracts API error status=%d message=%s",
				resp.StatusCode,
				dbg.Message,
			)
			return nil, fmt.Errorf(
				"massive returned status %d: %s",
				resp.StatusCode,
				dbg.Message,
			)
		}

		var massiveResp massiveContractsResp
		if err := json.Unmarshal(body, &massiveResp); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}

		logger.Tracef("received %d contracts", len(massiveResp.Results))

		for _, result := range massiveResp.Results {
			// parse expiration
			t, err := time.Parse("2006-01-02", result.ExpiryDate)
			if err != nil {
				continue // skip malformed expiry dates
			}

			out = append(out, OptionContract{
				ExpiryDate: t,
				Strike:     result.StrikePrice,
				Type:       result.ContractType,
			})
		}

		reqURL = massiveResp.NextURL
	}

	return out, nil
}

// GetBars retrieves OHLCV bars for the given symbol and time range.
//
// Parameters:
//   - underlying: ticker symbol
//   - fromDate: start date
//   - toDate: end date
//   - timespan: aggregation interval size
//   - multiplier: aggregation unit (e.g., "day", "minute")
//
// Returns:
//   - []Bar: time-ordered bars
//   - error: if retrieval or decoding fails
func (massiveDataProv *massiveDataProvider) GetBars(
	underlying string,
	fromDate, toDate time.Time,
	timespan int,
	multiplier string,
) ([]Bar, error) {

	maxLimit := 50000

	logger.Debugf(
		"fetching bars: %s from=%s to=%s span=%d%s",
		underlying,
		fromDate.Format("2006-01-02"),
		toDate.Format("2006-01-02"),
		timespan,
		multiplier,
	)

	url := fmt.Sprintf(
		"%s/v2/aggs/ticker/%s/range/%d/%s/%s/%s?adjusted=true&sort=asc&limit=%d&apiKey=%s",
		massiveDataProv.BaseURL,
		underlying,
		timespan,
		multiplier,
		fromDate.Format("2006-01-02"),
		toDate.Format("2006-01-02"),
		maxLimit,
		massiveDataProv.APIKey,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Errorf("bars request errored=%v", err)
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("x-api-key", massiveDataProv.APIKey)

	resp, err := massiveDataProv.processGetRequest(req)
	if err != nil {
		logger.Errorf("bars request failed")
		return nil, fmt.Errorf("massive api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"massive daily bars status=%d body=%s",
			resp.StatusCode,
			string(bodyBytes),
		)
	}

	// Massive/POLYGON style response model
	var body struct {
		Ticker   string `json:"ticker"`
		Adjusted bool   `json:"adjusted"`
		Results  []struct {
			Open      float64 `json:"o"`
			Close     float64 `json:"c"`
			High      float64 `json:"h"`
			Low       float64 `json:"l"`
			VWAP      float64 `json:"vw"` // volume-weighted average price
			Volume    float64 `json:"v"`  // trading volume of the symbol in the given time period
			Trades    int64   `json:"n"`  // number of transactions in the aggregate window
			Timestamp int64   `json:"t"`  // epoch millis
		} `json:"results"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("parsing massive response: %w", err)
	}

	logger.Tracef("bars received: %d records", len(body.Results))

	out := make([]Bar, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Bar{
			Date:  time.UnixMilli(r.Timestamp).UTC(),
			Open:  r.Open,
			High:  r.High,
			Low:   r.Low,
			Close: r.Close,
			Vol:   r.Volume,
		})
	}

	return out, nil
}

// GetRelevantExpiries returns a sorted slice of unique option expiration dates
// for a given ticker within the specified time range.
//
// The function determines relevant option strike prices by analyzing spot price data
// and selecting three middle strike levels within the price range, then retrieves
// all available contracts for those strikes to extract their expiration dates.
//
// Parameters:
//   - ticker: The underlying symbol identifier (e.g., "SPY")
//   - start: The beginning of the date range for analysis
//   - end: The end of the date range for analysis
//   - provider: A data provider that supplies daily bars and contract information
//
// Returns:
//   - A sorted slice of unique time.Time values representing option expiration dates
//   - An error if spot data cannot be fetched, no data is available, or contract
//     retrieval fails
//
// The algorithm works as follows:
//  1. Fetches daily bar data for the ticker within the date range
//  2. Determines the high and low prices from the bars
//  3. Selects a rounding multiplier based on the low price
//  4. Divides the price range into 5 equal intervals and selects the middle 3 levels
//  5. Rounds strike prices to the nearest multiplier
//  6. Retrieves all available contracts for the rounded strike prices
//  7. Extracts and deduplicates expiration dates
//  8. Returns the sorted, unique expiration dates
func (massiveDataProv *massiveDataProvider) GetRelevantExpiries(
	ticker string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {

	logger.Infof(
		"resolving relevant expiries for %s [%s → %s]",
		ticker,
		fromDate.Format("2006-01-02"),
		toDate.Format("2006-01-02"),
	)

	// Step 1: Load spot bars
	bars, err := massiveDataProv.GetBars(ticker, fromDate, toDate, 1, "day")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spot data: %w", err)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no spot data found")
	}

	// Step 2: Compute high & low
	low := bars[0].Low
	high := bars[0].High
	for _, b := range bars {
		if b.Low < low {
			low = b.Low
		}
		if b.High > high {
			high = b.High
		}
	}

	logger.Debugf("spot range low=%.2f high=%.2f", low, high)

	// Step 3: Determine multiplier
	multiplier := 1.0
	switch {
	case low >= 100 && low < 1000:
		multiplier = 10
	case low >= 1000 && low < 10000:
		multiplier = 100
	case low >= 10000:
		multiplier = 1000
	}

	// Step 4: Divide range into 5 equal intervals
	step := (high - low) / 5

	// Step 5: Pick middle 3 levels
	levels := []float64{
		low + step,
		low + 3*step,
	}

	// Step 6: Round levels to nearest multiplier
	roundedStrikes := make([]float64, len(levels))
	for i, v := range levels {
		roundedStrikes[i] = math.Round(v/multiplier) * multiplier
	}

	// Step 7: Fetch contracts for each strike
	expiryMap := map[string]time.Time{}

	for _, strike := range roundedStrikes {
		logger.Tracef("fetching contracts for strike %.2f", strike)
		contracts, err := massiveDataProv.GetContracts(
			ticker, strike, time.Time{}, fromDate, toDate,
		)
		if err != nil {
			return nil, fmt.Errorf("fetch contracts strike %.2f: %w", strike, err)
		}

		for _, c := range contracts {
			key := c.ExpiryDate.Format("2006-01-02")
			expiryMap[key] = c.ExpiryDate
		}
	}

	// Step 8: Unique expiries & sorted slice
	expiries := make([]time.Time, 0, len(expiryMap))
	for _, dt := range expiryMap {
		expiries = append(expiries, dt)
	}

	sort.Slice(expiries, func(i, j int) bool {
		return expiries[i].Before(expiries[j])
	})

	logger.Infof("resolved %d unique expiries", len(expiries))
	return expiries, nil
}

// GetOptionPrice retrieves the price of an option at a specific trade date and time.
// It attempts to find the option price by first looking for bars from 5 minutes before
// the trade time and using the closing price if available. If no bars are found in that
// window, it looks for bars starting from the trade time and uses the opening price.
// Returns an error if no option bars are found in either time window.
//
// Parameters:
//   - underlying: the underlying asset symbol
//   - strike: the strike price of the option
//   - expiryDate: the expiration date of the option
//   - optType: the option type (e.g., "call" or "put")
//   - tradeDateTime: the date and time at which to retrieve the option price
//
// Returns:
//   - float64: the option price
//   - error: an error if the price cannot be determined
func (massiveDataProv *massiveDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optType string,
	tradeDateTime time.Time,
) (float64, error) {

	logger.Debugf(
		"option price lookup: %s strike=%.2f expiry=%s at %s",
		underlying,
		strike,
		expiryDate.Format("2006-01-02"),
		tradeDateTime.Format(time.RFC3339),
	)

	symbol := OptionSymbolFromParts(underlying, expiryDate, optType, strike)
	price := 0.0

	bars, err := massiveDataProv.GetBars(
		symbol,
		tradeDateTime.Add(-5*time.Minute),
		tradeDateTime,
		1,
		"minute",
	)
	if err != nil {
		return 0, fmt.Errorf("fetch option bars: %w", err)
	}

	if len(bars) != 0 {
		price = bars[len(bars)-1].Close
	} else {
		logger.Tracef("no bars before trade time, trying forward window")

		bars, err := massiveDataProv.GetBars(
			symbol,
			tradeDateTime,
			tradeDateTime.Add(5*time.Minute),
			1,
			"minute",
		)
		if err != nil {
			logger.Errorf("no option bars found for %s", symbol)
			return 0, fmt.Errorf("fetch option bars: %w", err)
		}
		if len(bars) == 0 {
			return 0, fmt.Errorf(
				"no option bars found for %s on %s",
				symbol,
				tradeDateTime.Format("2006-01-02 15:04"),
			)
		}
		price = bars[0].Open
	}

	return price, nil
}

// RoundToNearestStrike finds the nearest available option strike price to the given price.
// It retrieves all option contracts for the specified underlying asset and expiry date,
// extracts their strike prices, and returns the strike closest to asOfPrice.
// If no contracts are found or an error occurs, it returns asOfPrice unchanged.
//
// Parameters:
//   - underlying: the underlying asset symbol (e.g., "AAPL")
//   - expiryDate: the expiration date of the option contracts
//   - openDate: the trading date as of which to fetch contracts
//   - asOfPrice: the reference price to find the nearest strike for
//
// Returns:
//
//	The strike price closest to asOfPrice, or asOfPrice if no contracts are available.
func (massiveDataProv *massiveDataProvider) RoundToNearestStrike(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) float64 {

	var strikeList []float64
	// Fetch all contracts for the underlying, expiry date as of open date as trading date and collect strikes
	strike := 0.0 // zero means to fetch all strikes
	optionContracts, err := massiveDataProv.GetContracts(underlying, strike, expiryDate, openDate, openDate)
	if err != nil {
		return asOfPrice
	}

	for i := range optionContracts {
		if optionContracts[i].ExpiryDate.Equal(expiryDate) {
			strikeList = append(strikeList, optionContracts[i].Strike)
		}
	}

	if len(strikeList) != 0 {
		sort.Float64s(strikeList)
		return Closest(strikeList, asOfPrice)
	}

	return massiveDataProv.RoundToNearestStrike(
		underlying,
		expiryDate,
		openDate,
		asOfPrice,
	)
}

// processGetRequest executes an HTTP GET request with rate-limit handling.
//
// Behavior:
//   - Retries indefinitely on HTTP 429
//   - Sleeps until the next minute boundary
//   - Returns immediately on success (<400)
//   - Returns an error for other status codes
func (massiveDataProv *massiveDataProvider) processGetRequest(
	req *http.Request,
) (*http.Response, error) {

	for {
		resp, err := massiveDataProv.Client.Do(req)
		if err != nil {
			return nil, err
		}

		// Success
		if resp.StatusCode < 400 {
			return resp, nil
		}

		// Handle per-minute rate limit (commonly 429)
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			// Sleep until the next minute boundary
			now := time.Now()
			sleepDuration := time.Until(
				now.Truncate(time.Minute).Add(time.Minute),
			)

			logger.Infof("rate limit hit, sleeping for %s", sleepDuration)
			time.Sleep(sleepDuration)
			continue
		}

		return resp, fmt.Errorf(
			"unexpected status code: %d",
			resp.StatusCode,
		)
	}
}

// getIntervals is a placeholder for future interval logic.
func (massiveDataProv *massiveDataProvider) getIntervals(
	underlying string,
) float64 {
	return 0.0
}
