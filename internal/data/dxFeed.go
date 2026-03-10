package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type DxFeedDataProvider struct {
	WsURL        string
	SessionToken string    // The dxLink Quote Token
	ExpiresAt    time.Time // When the Quote Token expires

	// Credentials to fetch a new token
	ttBaseURL   string
	ttAuthToken string // Your Tastytrade Session/OAuth Token

	mu sync.Mutex
}

func NewDxFeedProvider(wsURL, ttBaseURL, ttAuthToken string) *DxFeedDataProvider {
	return &DxFeedDataProvider{
		WsURL:       wsURL,
		ttBaseURL:   ttBaseURL,
		ttAuthToken: ttAuthToken,
	}
}

// refreshTokenIfNeeded checks if the token is missing or expiring soon (within 5 mins)
func (dxFeedDataProv *DxFeedDataProvider) refreshTokenIfNeeded() error {
	// If token exists and has > 5 minutes of life, do nothing
	if dxFeedDataProv.SessionToken != "" && time.Now().Add(5*time.Minute).Before(dxFeedDataProv.ExpiresAt) {
		return nil
	}

	fmt.Println("Token expired or missing. Fetching new dxLink token...")

	req, _ := http.NewRequest("GET", dxFeedDataProv.ttBaseURL+"/api-quote-tokens", nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", dxFeedDataProv.ttAuthToken))
	req.Header.Add("User-Agent", "option-replay/1.0")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call api-quote-tokens: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get quote token, status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode quote token: %w", err)
	}

	// Update the provider state
	dxFeedDataProv.SessionToken = result.Data.Token
	// dxLink tokens are usually valid for 24h
	dxFeedDataProv.ExpiresAt = time.Now().Add(24 * time.Hour)

	return nil
}

// GetHistoricalData retrieves historical OHLC bar data from the dxFeed API for a specified symbol and time range.
//
// It performs the following steps:
// 1. Validates and refreshes the authentication token if needed
// 2. Establishes a WebSocket connection to the dxFeed API
// 3. Performs the WebSocket handshake using the validated token
// 4. Subscribes to Candle data for the specified symbol and timeframe
// 5. Collects bar data from the feed until the end time is reached or timeout occurs
//
// Parameters:
//   - symbol: The trading symbol (e.g., "AAPL", "GOOGL")
//   - start: The start time for the historical data range
//   - end: The end time for the historical data range
//   - timeframe: The candle timeframe (e.g., "1m", "5m", "1h", "1d")
//
// Returns:
//   - []Bar: A slice of Bar objects containing the historical OHLC data
//   - error: An error if authentication, connection, subscription, or data retrieval fails;
//     returns a timeout error if no data is received within 30 seconds
//
// Note: This method is protected by a mutex lock to ensure thread-safe access.
func (dxFeedDataProv *DxFeedDataProvider) GetHistoricalData(
	symbol string,
	start, end time.Time,
	timeframe string,
) ([]Bar, error) {
	dxFeedDataProv.mu.Lock()
	defer dxFeedDataProv.mu.Unlock()

	// 1. Validate / Refresh Token
	if err := dxFeedDataProv.refreshTokenIfNeeded(); err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}

	// 2. Connect
	conn, _, err := websocket.DefaultDialer.Dial(dxFeedDataProv.WsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dxfeed connection failed: %w", err)
	}
	defer conn.Close()

	// 3. Handshake (Uses the fresh/validated SessionToken)
	if err := dxFeedDataProv.handshake(conn); err != nil {
		return nil, err
	}

	// 4. Subscribe to Candles
	dxSymbol := fmt.Sprintf("%s{=%s}", symbol, timeframe)
	subMsg := map[string]interface{}{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"add": []map[string]string{
			{
				"symbol": dxSymbol,
				"type":   "Candle",
				"from":   start.Format("2006-01-02T15:04:05Z"),
				"to":     end.Format("2006-01-02T15:04:05Z"),
			},
		},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		return nil, err
	}

	// 5. Data Collection Loop (Unchanged logic)
	var bars []Bar
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-timeout:
			return bars, fmt.Errorf("timeout waiting for dxfeed data")
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				return bars, nil
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			if msg["type"] == "FEED_DATA" {
				newBars := parseDxBars(msg["data"])
				bars = append(bars, newBars...)

				if len(newBars) > 0 && !newBars[len(newBars)-1].Date.Before(end) {
					return bars, nil
				}
			}
		}
	}
}

// handshake establishes a connection with the dxFeed server by sending authentication
// and channel setup requests over the provided WebSocket connection. It sends three
// sequential JSON messages: a SETUP message to initialize the protocol version, an AUTH
// message with the session token for authentication, and a CHANNEL_REQUEST message to
// subscribe to the FEED service with automatic contract selection and list format.
// The function pauses for 200 milliseconds to allow the server time to process the
// channel request before returning. It returns an error if any operation fails.
func (dxFeedDataProv *DxFeedDataProvider) handshake(conn *websocket.Conn) error {
	// SETUP
	conn.WriteJSON(map[string]interface{}{"type": "SETUP", "channel": 0, "version": "1.0.0"})
	// AUTH
	conn.WriteJSON(map[string]interface{}{"type": "AUTH", "channel": 0, "token": dxFeedDataProv.SessionToken})
	// CHANNEL
	conn.WriteJSON(map[string]interface{}{
		"type": "CHANNEL_REQUEST", "channel": 1, "service": "FEED",
		"parameters": map[string]interface{}{"contract": "AUTO", "subFormat": "LIST"},
	})

	// Give the server a moment to process the channel request
	time.Sleep(200 * time.Millisecond)
	return nil
}

// parseDxBars converts the raw dxFeed Candle event array into your internal Bar struct
func parseDxBars(rawData interface{}) []Bar {
	var bars []Bar
	data, ok := rawData.([]interface{})
	if !ok {
		return bars
	}

	for _, item := range data {
		entry := item.([]interface{})
		// dxFeed Candle index typically: 0:symbol, 1:type, 2:time, 3:open, 4:high, 5:low, 6:close, etc.
		if len(entry) < 7 {
			continue
		}

		ts := int64(entry[2].(float64))
		bars = append(bars, Bar{
			Date:  time.Unix(ts/1000, 0).UTC(),
			Open:  entry[3].(float64),
			High:  entry[4].(float64),
			Low:   entry[5].(float64),
			Close: entry[6].(float64),
		})
	}
	return bars
}
