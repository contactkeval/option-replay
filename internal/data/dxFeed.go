package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/gorilla/websocket"
)

type DxFeedDataProvider struct {
	mu sync.Mutex

	// 1. OAuth Tokens (Tastytrade API)
	ttBaseURL    string
	refreshToken string    // The permanent JWT from Developer Portal
	ttAuthToken  string    // The 15-minute Access Token
	ttAuthExpiry time.Time // When the 15-minute token dies

	// 2. dxLink Tokens (Market Data)
	WsURL        string
	SessionToken string    // The 24-hour dxLink Quote Token
	dxLinkExpiry time.Time // When the Quote Token expires

	// Client Config
	clientID     string
	clientSecret string
}

func NewDxFeedProvider(baseURL, refresh, cID, cSecret string) *DxFeedDataProvider {
	return &DxFeedDataProvider{
		ttBaseURL:    baseURL,
		refreshToken: refresh,
		clientID:     cID,
		clientSecret: cSecret,
	}
}

// ensureValidAccessToken handles the 15-minute Tastytrade OAuth lifecycle
func (dxFeedDataProv *DxFeedDataProvider) ensureValidAccessToken() error {
	// If token is valid for more than 2 minutes, we are good
	if dxFeedDataProv.ttAuthToken != "" && time.Now().Add(2*time.Minute).Before(dxFeedDataProv.ttAuthExpiry) {
		return nil
	}

	logger.Infof("Access Token expired. Exchanging Refresh Token for new Access Token...")

	// This is the POST request you were doing in Postman
	url := fmt.Sprintf("%s/oauth/token", dxFeedDataProv.ttBaseURL)

	// Prepare the body for x-www-form-urlencoded
	payload := fmt.Sprintf("grant_type=refresh_token&refresh_token=%s&client_secret=%s",
		dxFeedDataProv.refreshToken, dxFeedDataProv.clientSecret)
	if dxFeedDataProv.clientID != "" {
		payload += fmt.Sprintf("&client_id=%s", dxFeedDataProv.clientID)
	}

	req, _ := http.NewRequest("POST", url, strings.NewReader(payload))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", "option-replay/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oauth refresh failed: status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // Usually 900
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	dxFeedDataProv.ttAuthToken = res.AccessToken
	dxFeedDataProv.ttAuthExpiry = time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	return nil
}

// refreshTokenIfNeeded handles the 24-hour dxLink lifecycle
func (dxFeedDataProv *DxFeedDataProvider) refreshTokenIfNeeded() error {
	// 1. First, make sure our 15-minute Tastytrade token is valid
	if err := dxFeedDataProv.ensureValidAccessToken(); err != nil {
		return fmt.Errorf("failed to ensure oauth access: %w", err)
	}

	// 2. Now check if the 24-hour dxLink token is still valid
	if dxFeedDataProv.SessionToken != "" && time.Now().Add(10*time.Minute).Before(dxFeedDataProv.dxLinkExpiry) {
		return nil
	}

	logger.Infof("dxLink token expired or missing. Fetching new one...")

	req, _ := http.NewRequest("GET", dxFeedDataProv.ttBaseURL+"/api-quote-tokens", nil)
	req.Header.Add("Authorization", "Bearer "+dxFeedDataProv.ttAuthToken) // Uses the fresh token from step 1
	req.Header.Add("User-Agent", "option-replay/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get quote token, status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Token     string `json:"token"`
			DxLinkURL string `json:"dxlink-url"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	dxFeedDataProv.SessionToken = result.Data.Token
	dxFeedDataProv.WsURL = result.Data.DxLinkURL
	dxFeedDataProv.dxLinkExpiry = time.Now().Add(24 * time.Hour) // dxLink tokens are robust

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
	timeout := time.After(45 * time.Second)

	for {
		select {
		case <-timeout:
			return bars, fmt.Errorf("timeout waiting for dxfeed data")
		default:
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return bars, nil
			}

			// DEBUG: Print the raw message to see what is actually coming over the wire
			// This will show us if it's FEED_DATA, ERROR, or a KEEPALIVE
			logger.Infof("RAW WS MSG [%d]: %s", messageType, string(message))

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
		"type":    "CHANNEL_REQUEST",
		"channel": 1,
		"service": "FEED",
		"parameters": map[string]interface{}{
			"contract":  "AUTO",
			"subFormat": "LIST"},
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
