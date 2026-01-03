package data

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
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

func (polygonDataProv *polygonDataProvider) Secondary() Provider {
	return polygonDataProv.secondary
}

func (polygonDataProv *polygonDataProvider) GetContracts(underlying string, strike float64, fromDate, toDate, expiryDt time.Time) ([]OptionContract, error) {
	// Polygon does not provide an endpoint to list option contracts by strike.
	// This method is not implemented.
	return nil, fmt.Errorf("GetContracts not implemented for PolygonProvider")
}

func (polygonDataProv *polygonDataProvider) GetDailyBars(underlying string, fromDate, toDate time.Time) ([]Bar, error) {
	base := "https://api.polygon.io"
	url := fmt.Sprintf("%s/v2/aggs/ticker/%s/range/1/day/%s/%s?adjusted=true&sort=asc&limit=50000&apiKey=%s",
		base, underlying, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"), polygonDataProv.apiKey)
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
			Time  int64   `json:"t"`
			Open  float64 `json:"o"`
			High  float64 `json:"h"`
			Low   float64 `json:"l"`
			Close float64 `json:"c"`
			Vol   float64 `json:"v"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Bar, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Bar{Date: time.UnixMilli(r.Time).UTC(), Open: r.Open, High: r.High, Low: r.Low, Close: r.Close, Vol: r.Vol})
	}
	return out, nil
}

func (polygonDataProv *polygonDataProvider) GetOptionMidPrice(underlying string, strike float64, expiryDate time.Time, optType string) (float64, error) {
	// Try snapshot v3; this requires that your plan supports option snapshot access.
	symbol := OptionSymbolFromParts(underlying, expiryDate, optType, strike)
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

func (polygonDataProv *polygonDataProvider) RoundToNearestStrike(underlying string, asOfPrice float64, openDate, expiryDate time.Time) float64 {
	intervals := polygonDataProv.getIntervals(underlying)
	return math.Round(asOfPrice/intervals) * intervals
}

func (polygonDataProv *polygonDataProvider) getIntervals(underlying string) float64 {
	return 50.0 // TODO: implement proper intervals reading
}
