package data

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// NewMassiveDataProvider convenience constructor.
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

func (massiveDataProv *massiveDataProvider) Secondary() Provider {
	return massiveDataProv.secondary
}

func (massiveDataProv *massiveDataProvider) GetContracts(underlying string, strike float64, fromDate, toDate, expiryDate time.Time) ([]OptionContract, error) {
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

func (massiveDataProv *massiveDataProvider) GetDailyBars(underlying string, fromDate, toDate time.Time) ([]Bar, error) {
	url := fmt.Sprintf(
		"%s/v2/aggs/ticker/%s/range/1/day/%s/%s?adjusted=true&sort=asc&limit=50000&apiKey=%s",
		massiveDataProv.BaseURL,
		underlying,
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
	bars, err := massiveDataProv.GetDailyBars(ticker, fromDate, toDate)
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

func (massiveDataProv *massiveDataProvider) GetOptionMidPrice(underlying string, strike float64, expiryDate time.Time, optType string) (float64, error) {
	//TODO: implement option mid price fetching from Massive API
	return 0, fmt.Errorf("GetOptionMidPrice not implemented for MassiveDataProvider")
}

func (massiveDataProv *massiveDataProvider) RoundToNearestStrike(underlying string, asOfPrice float64, openDate, expiryDate time.Time) float64 {
	var strikeList []float64
	// Fetch all contracts for the underlying, expiry date as of open date as trading date and collect strikes
	OptionContracts, err := massiveDataProv.GetContracts(underlying, 0.0, openDate, openDate, expiryDate)
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
	return massiveDataProv.RoundToNearestStrike(underlying, asOfPrice, openDate, expiryDate)
}

func (massiveDataProv *massiveDataProvider) getIntervals(underlying string) float64 {
	return 0.0 // TODO: implement proper intervals reading
}
