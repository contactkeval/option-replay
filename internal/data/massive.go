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
)

// TODO: use official Massive SDK (massive-com/client-go) instead of raw HTTP calls.

// massiveDataProvider implements Data Provider using Massive's contracts API.
type massiveDataProvider struct {
	APIKey    string
	Client    *http.Client
	BaseURL   string // e.g., "https://api.massive.com" or "https://api.massive.xyz"
	secondary Provider
}

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

type massiveContractsResp struct {
	Results   []massiveContract `json:"results"`
	Status    string            `json:"status"`
	RequestID string            `json:"request_id"`
	NextURL   string            `json:"next_url"`
}

// NewMassiveDataProvider creates and returns a new massiveDataProvider instance.
// It initializes an HTTP client with optimized timeout and transport settings,
// including TLS configuration, connection pooling, and gzip decompression support.
// The provided apiKey is used for authentication with the Massive API.
func NewMassiveDataProvider(apiKey string) *massiveDataProvider {
	return &massiveDataProvider{
		APIKey: apiKey,
		Client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableCompression:    false, // MUST be false to auto-decompress gzip
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		BaseURL: "https://api.massive.com", // change if required
	}
}

// Secondary returns the secondary Provider associated with this massive data provider.
func (massiveDataProv *massiveDataProvider) Secondary() Provider {
	return massiveDataProv.secondary
}

func (massiveDataProv *massiveDataProvider) GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
	strike = math.Round(asOfPrice*100) / 100
	callPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)
	putPrice = 1.0 + math.Abs(rand.NormFloat64()*0.5)

	if massiveDataProv.secondary != nil {
		return massiveDataProv.secondary.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
	}
	return strike, callPrice, putPrice, nil
}

func (massiveDataProv *massiveDataProvider) GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error) {
	out := []OptionContract{}

	// Build initial URL with required filters.
	url, err := url.Parse(massiveDataProv.BaseURL + "/v3/reference/options/contracts")
	if err != nil {
		return nil, err
	}
	query := url.Query()
	query.Set("underlying_ticker", underlying)
	if strike > 0.0 {
		query.Set("strike_price", fmt.Sprintf("%.8g", strike))
	}
	if expiryDate.IsZero() {
		// expiration date greater than or equal to start, less than or equal to end
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

	// Paginate through results
	for reqURL != "" {
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+massiveDataProv.APIKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "massive-client/1.0")

		resp, err := massiveDataProv.Client.Do(req)
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
			// try to read body text for debugging
			var dbg struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &dbg)
			return nil, fmt.Errorf("massive returned status %d: %s", resp.StatusCode, dbg.Message)
		}

		var massiveResp massiveContractsResp
		if err := json.Unmarshal(body, &massiveResp); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}

		for _, result := range massiveResp.Results {
			// parse expiration
			t, err := time.Parse("2006-01-02", result.ExpiryDate)
			if err != nil {
				// skip malformed
				continue
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

func (massiveDataProv *massiveDataProvider) GetBars(underlying string, fromDate, toDate time.Time, timespan int, multiplier string) ([]Bar, error) {
	url := fmt.Sprintf(
		"%s/v2/aggs/ticker/%s/range/%d/%s/%s/%s?adjusted=true&sort=asc&limit=50000&apiKey=%s",
		massiveDataProv.BaseURL,
		underlying,
		timespan,
		multiplier,
		fromDate.Format("2006-01-02"),
		toDate.Format("2006-01-02"),
		massiveDataProv.APIKey,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-api-key", massiveDataProv.APIKey)

	resp, err := massiveDataProv.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("massive api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("massive daily bars status=%d body=%s",
			resp.StatusCode, string(bodyBytes))
	}

	// Massive/POLYGON style response model
	var body struct {
		Ticker   string `json:"ticker"`
		Adjusted bool   `json:"adjusted"`
		Results  []struct {
			Volume    float64 `json:"v"`
			Open      float64 `json:"o"`
			Close     float64 `json:"c"`
			High      float64 `json:"h"`
			Low       float64 `json:"l"`
			VWAP      float64 `json:"vw"`
			Timestamp int64   `json:"t"` // epoch millis
			Trades    int64   `json:"n"`
		} `json:"results"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("parsing massive response: %w", err)
	}

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
func (massiveDataProv *massiveDataProvider) GetRelevantExpiries(ticker string, fromDate, toDate time.Time) ([]time.Time, error) {

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
		low + step, // level 1
		// low + 2*step, // level 2
		low + 3*step, // level 3
	}

	// Step 6: Round levels to nearest multiplier
	roundedStrikes := make([]float64, len(levels))
	for i, v := range levels {
		roundedStrikes[i] = math.Round(v/multiplier) * multiplier
	}

	// Step 7: Fetch contracts for each strike
	expiryMap := map[string]time.Time{}

	for _, strike := range roundedStrikes {
		contracts, err := massiveDataProv.GetContracts(ticker, strike, fromDate, time.Time{}, toDate)
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
func (massiveDataProv *massiveDataProvider) GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optType string, tradeDateTime time.Time) (float64, error) {
	symbol := OptionSymbolFromParts(underlying, expiryDate, optType, strike)
	price := 0.0

	bars, err := massiveDataProv.GetBars(symbol, tradeDateTime.Add(-5*time.Minute), tradeDateTime, 1, "minute")
	if err != nil {
		return 0, fmt.Errorf("fetch option bars: %w", err)
	}
	if len(bars) != 0 {
		price = bars[len(bars)-1].Close
	} else {
		bars, err := massiveDataProv.GetBars(symbol, tradeDateTime, tradeDateTime.Add(5*time.Minute), 1, "minute")
		if err != nil {
			return 0, fmt.Errorf("fetch option bars: %w", err)
		}
		if len(bars) == 0 {
			return 0, fmt.Errorf("no option bars found for %s on %s", symbol, tradeDateTime.Format("2006-01-02 15:04"))
		}
		price = bars[0].Open
	}

	return price, nil
}

func (massiveDataProv *massiveDataProvider) RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64 {
	var strikeList []float64
	// Fetch all contracts for the underlying, expiry date as of open date as trading date and collect strikes
	strike := 0.0 // zero means to fetch all strikes
	OptionContracts, err := massiveDataProv.GetContracts(underlying, strike, expiryDate, openDate, openDate)
	if err != nil {
		return asOfPrice
	}

	for OptionContract := range OptionContracts {
		if OptionContracts[OptionContract].ExpiryDate.Equal(expiryDate) {
			strikeList = append(strikeList, OptionContracts[OptionContract].Strike)
		}
	}

	if len(strikeList) != 0 {
		sort.Float64s(strikeList)
		return Closest(strikeList, asOfPrice)
	}
	return massiveDataProv.RoundToNearestStrike(underlying, expiryDate, openDate, asOfPrice)
}

func (massiveDataProv *massiveDataProvider) getIntervals(underlying string) float64 {
	return 0.0 // TODO: implement proper intervals reading
}
