package data

import (
	"os"
	"testing"
	"time"
)

func TestDxFeed_FetchSpecificWindow(t *testing.T) {
	// 1. Setup provider with the new OAuth-capable signature
	// It is highly recommended to use Environment Variables for these!

	ttBaseURL := "https://api.cert.tastyworks.com"

	// These should be your Sandbox/Certification credentials
	refreshToken := os.Getenv("TT_REFRESH_TOKEN")
	clientID := os.Getenv("TT_CLIENT_ID")
	clientSecret := os.Getenv("TT_CLIENT_SECRET")

	// Fallback for quick manual testing (Optional - use with caution)
	// if refreshToken == "" {
	// 	refreshToken = "YOUR_PERMANENT_REFRESH_TOKEN_HERE"
	// 	clientID = "YOUR_CLIENT_ID_HERE"
	// 	clientSecret = "YOUR_CLIENT_SECRET_HERE"
	// }

	if refreshToken == "YOUR_PERMANENT_REFRESH_TOKEN_HERE" {
		t.Skip("Skipping test: Credentials not set. Please update environment variables or placeholders.")
	}

	// Initialize the provider. Note: WsURL will be discovered automatically
	// by the provider during the first refresh call.
	dxFeedDataProv := NewDxFeedProvider(ttBaseURL, refreshToken, clientID, clientSecret)

	// 2. Define the EST Timezone
	est, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("Could not load EST location:", err)
	}

	// 3. Create the start/end times in EST
	// Note: January 5th, 2026 was a Thursday. Market was open.
	startEST := time.Date(2026, time.January, 5, 10, 0, 0, 0, est) // 10:00 AM EST
	endEST := time.Date(2026, time.January, 5, 11, 0, 0, 0, est)   // 11:00 AM EST

	// 5. Execute fetch
	// This will now:
	// a) Exchange Refresh Token for Access Token
	// b) Use Access Token to get dxLink Quote Token and WsURL
	// c) Connect to WebSocket and pull bars
	t.Logf("Fetching AAPL from %s to %s (EST)", startEST, endEST)
	bars, err := dxFeedDataProv.GetHistoricalData("AAPL", startEST.UTC(), endEST.UTC(), "1m")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// 6. Assertions
	if len(bars) == 0 {
		t.Errorf("No bars returned for AAPL. Ensure sandbox has an active account and market data permissions.")
	}

	// 7. Output results
	t.Logf("Successfully retrieved %d bars.", len(bars))
	for i, bar := range bars {
		// Print first 2 and last 2 for brevity
		if i < 2 || i >= len(bars)-2 {
			t.Logf("[%d] Time: %s | Close: %.2f", i, bar.Date.In(est).Format("15:04:05"), bar.Close)
		}
	}
}
