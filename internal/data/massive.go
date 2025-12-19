package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// massiveDataProvider implements Data Provider using Massive's contracts API.
type massiveDataProvider struct {
	APIKey    string
	Client    *http.Client
	BaseURL   string // e.g., "https://api.massive.com" or "https://api.massive.xyz"
	secondary Provider
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

type massiveContract struct {
	CFI               string  `json:"cfi"`
	ContractType      string  `json:"contract_type"`
	ExerciseStyle     string  `json:"exercise_style"`
	ExpirationDate    string  `json:"expiration_date"`
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

func (massiveDataProv *massiveDataProvider) GetContracts(underlying string, strike float64, start, end time.Time) ([]OptionContract, error) {
	out := []OptionContract{}

	// Build initial URL with required filters.
	u, err := url.Parse(massiveDataProv.BaseURL + "/v3/reference/options/contracts")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("underlying_ticker", underlying)
	q.Set("strike_price", fmt.Sprintf("%.8g", strike))
	q.Set("expired", "true")
	// expiration date greater than or equal to start, less than or equal to end
	q.Set("expiration_date.lte", end.Format("2006-01-02"))
	q.Set("expiration_date.gte", start.Format("2006-01-02"))
	q.Set("limit", "1000")
	q.Set("apiKey", massiveDataProv.APIKey)

	u.RawQuery = q.Encode()
	reqURL := u.String()

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

		var mr massiveContractsResp
		if err := json.Unmarshal(body, &mr); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}

		for _, c := range mr.Results {
			// parse expiration
			t, err := time.Parse("2006-01-02", c.ExpirationDate)
			if err != nil {
				// skip malformed
				continue
			}
			out = append(out, OptionContract{
				ExpirationDate: t,
				Strike:         c.StrikePrice,
				Type:           c.ContractType,
			})
		}

		reqURL = mr.NextURL
	}

	return out, nil
}

func (massiveDataProv *massiveDataProvider) GetDailyBars(symbol string, from, to time.Time) ([]Bar, error) {
	url := fmt.Sprintf(
		"%s/v2/aggs/ticker/%s/range/1/day/%s/%s?adjusted=true&sort=asc&limit=50000&apiKey=%s",
		massiveDataProv.BaseURL,
		symbol,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
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

func (massiveDataProv *massiveDataProvider) GetOptionMidPrice(symbol string, strike float64, expiry time.Time, optType string) (float64, error) {
	//TODO: implement option mid price fetching from Massive API
	return 0, fmt.Errorf("GetOptionMidPrice not implemented for MassiveDataProvider")
}

func (massiveDataProv *massiveDataProvider) getIntervals(underlying string) float64 {
	return 50.0 // TODO: implement proper intervals reading
}
