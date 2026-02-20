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

// polygonDataProvider implements Data Provider using Polygon.io API.
type polygonDataProvider struct {
	apiKey    string
	client    *http.Client
	secondary Provider
}

func NewPolygonDataProvider(apiKey string) Provider {
	return &polygonDataProvider{apiKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

func (polygonDataProv *polygonDataProvider) GetSecondary() Provider {
	return polygonDataProv.secondary
}

func (polygonDataProv *polygonDataProvider) SetSecondary(secondary Provider) {
	polygonDataProv.secondary = secondary
}

func (polygonDataProv *polygonDataProvider) Set(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
	return polygonDataProv.GetATMOptionPrices(underlying, expiryDate, openDate, asOfPrice)
}

func (polygonDataProv *polygonDataProvider) GetATMOptionPrices(underlying string, expiryDate, openDate time.Time, asOfPrice float64) (strike, callPrice, putPrice float64, err error) {
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
	for _, s := range res.Options.Strikes {
		diff := math.Abs(s.Strike - strike)
		if diff < minDiff {
			minDiff = diff
			closestStrikeData = &s
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

func (polygonDataProv *polygonDataProvider) GetContracts(underlying string, strike float64, expiryDate, fromDate, toDate time.Time) ([]OptionContract, error) {
	// Polygon does not provide an endpoint to list option contracts by strike.
	// This method is not implemented.
	return nil, fmt.Errorf("GetContracts not implemented for PolygonProvider")
}

func (polygonDataProv *polygonDataProvider) GetBars(underlying string, fromDate, toDate time.Time, timespan int, multiplier string) ([]Bar, error) {
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
			Volume float64  `json:"v"`
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

func (polygonDataProv *polygonDataProvider) GetOptionPrice(underlying string, strike float64, expiryDate time.Time, optType string, openDate time.Time) (float64, error) {
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

func (polygonDataProv *polygonDataProvider) GetRelevantExpiries(ticker string, fromDate, toDate time.Time) ([]time.Time, error) {
	if polygonDataProv.secondary != nil {
		return polygonDataProv.secondary.GetRelevantExpiries(ticker, fromDate, toDate)
	}
	return nil, fmt.Errorf("GetRelevantExpiries not implemented for PolygonProvider")
}

func (polygonDataProv *polygonDataProvider) RoundToNearestStrike(underlying string, expiryDate, openDate time.Time, asOfPrice float64) float64 {
	intervals := polygonDataProv.getIntervals(underlying)
	return math.Round(asOfPrice/intervals) * intervals
}

// OptionSymbolFromParts: improved OCC-like formatter (best-effort)
func (polygonDataProv *polygonDataProvider) OptionSymbolFromParts(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	// OCC: <root><YYMMDD><C|P><strike*1000 padded to 8 digits>
	expDt := expiryDate.UTC().Format("060102")
	optType := "C"
	if opt := strings.ToLower(optionType); opt == "put" || opt == "p" {
		optType = "P"
	}
	strikeStr := fmt.Sprintf("%08d", int(math.Round(strike*1000)))
	return fmt.Sprintf("O:%s%s%s%s", strings.ToUpper(underlying), expDt, optType, strikeStr)
}

func (polygonDataProv *polygonDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
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

func (polygonDataProv *polygonDataProvider) getIntervals(underlying string) float64 {
	return 50.0 // TODO: implement proper intervals reading
}
