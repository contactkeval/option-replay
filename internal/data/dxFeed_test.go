package data

import (
	"fmt"
	"testing"
	"time"
)

func TestDxFeed_FetchSpecificWindow(t *testing.T) {
	// 1. Setup provider
	// In production/local testing, use Environment Variables for security
	// LIVE
	// wsURL := "wss://tasty-openapi-ws.dxfeed.com/realtime"
	// ttBaseURL := "https://api.tastyworks.com"
	// DEMO
	wsURL := "wss://tasty-demo-ws.dxfeed.com/delayed" // For testing, we can use the delayed feed which doesn't require a paid subscription
	ttBaseURL := "https://api.cert.tastyworks.com"

	// Get your Tastytrade Session/OAuth token from environment
	// ttAuthToken := os.Getenv("DXFEED_AUTH_TOKEN") // TODO
	ttAuthToken := "dGFzdHksZGVtbywsMTc3MzIyNTYzNiwxNzczMTM5MjM2LFUyN2M5NzA3ZS00ZWQ2LTQ4ZDQtYWU3Ny03MWMxNmY0NzBhNDE.k_n7DVedHqU_faN1HJPv9AMhUWDQwFbZ6JGwFXoigzk"
	if ttAuthToken == "" {
		t.Skip("Skipping test: DXFEED_AUTH_TOKEN not set in environment")
	}

	// Note the new signature: wsURL, ttBaseURL, ttAuthToken
	dxFeedDataProv := NewDxFeedProvider(wsURL, ttBaseURL, ttAuthToken)

	// 2. Define the EST Timezone
	est, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal("Could not load EST location:", err)
	}

	// 3. Create the start/end times in EST (Jan 02, 2025 was a Thursday)
	startEST := time.Date(2025, time.January, 2, 15, 0, 0, 0, est)
	endEST := time.Date(2025, time.January, 2, 16, 0, 0, 0, est)

	// 4. Convert to UTC
	startUTC := startEST.UTC()
	endUTC := endEST.UTC()

	t.Logf("Fetching AAPL from %s to %s (UTC)", startUTC, endUTC)

	// 5. Execute fetch
	// This will trigger refreshTokenIfNeeded() internally if dxFeedDataProv.SessionToken is empty
	bars, err := dxFeedDataProv.GetHistoricalData("AAPL", startUTC, endUTC, "1m")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// 6. Assertions
	if len(bars) == 0 {
		t.Errorf("No bars returned for AAPL on %s. Ensure market was open and token has permissions.", startEST.Format("2006-01-02"))
	}

	// 7. Verify the data is within the requested range
	fmt.Printf("Successfully retrieved %d bars.\n", len(bars))
	for i, bar := range bars {
		if i < 3 || i >= len(bars)-2 { // Print first 3 and last 2
			fmt.Printf("[%d] Time: %s | Close: %.2f\n", i, bar.Date.In(est), bar.Close)
		}
	}
}
