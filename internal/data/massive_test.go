package data

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMassiveProvider_GetDailyBars_HTTPError(t *testing.T) {
	// fake server returning 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer srv.Close()

	p := &massiveDataProvider{
		APIKey:  "test",
		Client:  srv.Client(),
		BaseURL: srv.URL, // IMPORTANT
	}

	start := time.Now().AddDate(0, 0, -5)
	end := time.Now()

	_, err := p.GetDailyBars("AAPL", start, end)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMassiveProvider_Pagination(t *testing.T) {
	callCount := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			w.Write([]byte(`{
				"data": [
					{"t": 1735689600000, "o":1,"h":1,"l":1,"c":1,"v":100}
				],
				"next_url": "` + srv.URL + `/page2"
			}`))
			return
		}

		w.Write([]byte(`{
				"data": [
					{"t": 1735776000000, "o":1,"h":1,"l":1,"c":1,"v":100}
				]
			}`))
	}))
	defer srv.Close()

	p := &massiveDataProvider{
		APIKey:  "test",
		Client:  srv.Client(),
		BaseURL: srv.URL,
	}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)

	bars, err := p.GetDailyBars("AAPL", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
}

func TestMassiveRoundToNearestStrike(t *testing.T) {
	prov := NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))
	actual := prov.RoundToNearestStrike("SPY", 581.39, time.Date(2025, 1, 14, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC))
	expected := 581.0
	if actual != expected {
		t.Fatalf("expected %f, got %f", expected, actual)
	}
}
