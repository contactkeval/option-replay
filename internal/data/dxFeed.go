package data

import (
	"context"
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

	// dxFeed data parsing helpers
	fieldMap  map[string]int
	recordLen int
}

func NewDxFeedProvider(baseURL, refresh, cID, cSecret string) *DxFeedDataProvider {
	return &DxFeedDataProvider{
		ttBaseURL:    baseURL,
		refreshToken: refresh,
		clientID:     cID,
		clientSecret: cSecret,
	}
}

// GetName returns the name of the provider.
func (*DxFeedDataProvider) GetName() string {
	return "dxFeed"
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

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("oauth network error: %w", err)
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
		return fmt.Errorf("failed to decode oauth response: %w", err)
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
		return fmt.Errorf("quote token network error: %w", err)
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
		return fmt.Errorf("failed to decode quote token response: %w", err)
	}

	dxFeedDataProv.SessionToken = result.Data.Token
	dxFeedDataProv.WsURL = result.Data.DxLinkURL
	dxFeedDataProv.dxLinkExpiry = time.Now().Add(24 * time.Hour) // dxLink tokens are robust

	return nil
}

// GetHistoricalData retrieves historical OHLC bar data for a given symbol within a specified time range.
// It establishes an authenticated connection to the dxFeed service, subscribes to candle data for the
// specified symbol and timeframe, and collects the bars until the end time is reached.
//
// Parameters:
//   - symbol: The trading symbol to fetch data for (e.g., "AAPL")
//   - start: The beginning of the time range for historical data
//   - end: The end of the time range for historical data
//   - timeframe: The candle timeframe (e.g., "1m", "5m", "1h", "1d")
//
// Returns:
//   - []Bar: A slice of Bar objects containing OHLC data points
//   - error: An error if the connection, subscription, or data collection fails
func (dxFeedDataProv *DxFeedDataProvider) GetHistoricalData(
	symbol string,
	start, end time.Time,
	timeframe string,
) ([]Bar, error) {
	// 1. Establish the authenticated connection
	conn, err := dxFeedDataProv.connectAndHandshake()
	if err != nil {
		return nil, fmt.Errorf("connection/handshake failed: %w", err)
	}
	defer conn.Close()

	// 2. Send the subscription request
	if err := dxFeedDataProv.subscribeCandles(conn, symbol, timeframe, start, end); err != nil {
		return nil, fmt.Errorf("candle subscription failed: %w", err)
	}

	// 3. Collect the bars
	return dxFeedDataProv.collectHistoricalBars(conn, end)
}

// connectAndHandshake establishes a WebSocket connection to the dxFeed service and performs
// the necessary handshake authentication. It first refreshes the authentication token if needed,
// then dials the WebSocket URL with a 10-second timeout. If the connection is successful,
// it performs the handshake protocol. Returns the established WebSocket connection or an error
// if any step fails (token refresh, dial, or handshake).
func (dxFeedDataProv *DxFeedDataProvider) connectAndHandshake() (*websocket.Conn, error) {
	dxFeedDataProv.mu.Lock()
	if err := dxFeedDataProv.refreshTokenIfNeeded(); err != nil {
		dxFeedDataProv.mu.Unlock()
		return nil, fmt.Errorf("token refresh error: %w", err)
	}
	wsURL := dxFeedDataProv.WsURL
	dxFeedDataProv.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed to %s: %w", wsURL, err)
	}

	if err := dxFeedDataProv.handshake(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	return conn, nil
}

// subscribeCandles subscribes to candle data for a given symbol and timeframe within a specified time range.
// It constructs a FEED_SUBSCRIPTION message with the formatted symbol (including timeframe specification),
// candle type, and start/end timestamps in milliseconds, then sends the subscription request via WebSocket.
// Parameters:
//   - conn: the WebSocket connection to send the subscription to
//   - symbol: the trading symbol (e.g., "AAPL")
//   - timeframe: the candle timeframe (e.g., "1m", "5m", "1h")
//   - start: the start time for the candle data range
//   - end: the end time for the candle data range
//
// Returns an error if the WebSocket message cannot be written.
func (dxFeedDataProv *DxFeedDataProvider) subscribeCandles(conn *websocket.Conn, symbol, timeframe string, start, end time.Time) error {
	dxSymbol := fmt.Sprintf("%s{=%s}", symbol, timeframe)
	subMsg := map[string]interface{}{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"add": []map[string]interface{}{
			{
				"symbol":   dxSymbol,
				"type":     "Candle",
				"fromTime": start.UnixMilli(),
				"toTime":   end.UnixMilli(),
			},
		},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscription JSON for %s: %w", symbol, err)
	}
	return nil
}

// collectHistoricalBars retrieves historical candlestick data from the dxFeed WebSocket connection
// until the specified end time is reached or a timeout occurs.
//
// The function establishes a read loop on the WebSocket connection and processes incoming messages
// in the following sequence:
//   - FEED_CONFIG: Updates the internal field mapping for parsing Candle events
//   - FEED_DATA: Parses candlestick bars and appends them to the result slice, resetting the
//     activity timeout on each successful parse
//   - ERROR: Returns any dxFeed API errors encountered
//
// The function terminates when:
//   - A bar with a timestamp at or after the end time is received
//   - A timeout of 30 seconds occurs after the last FEED_DATA message (60 seconds initially)
//   - An error is encountered reading from the connection
//
// Parameters:
//   - conn: The WebSocket connection to the dxFeed API
//   - end: The target end time for historical data collection
//
// Returns:
//   - A slice of Bar structs containing the collected candlestick data
//   - An error if the connection fails, times out, or the server returns an error
//   - If bars were collected before an error occurs, returns the bars with nil error
func (dxFeedDataProv *DxFeedDataProvider) collectHistoricalBars(conn *websocket.Conn, end time.Time) ([]Bar, error) {
	var bars []Bar
	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()

	// Ensure mapping is fresh for this specific session
	dxFeedDataProv.mu.Lock()
	dxFeedDataProv.recordLen = 0
	dxFeedDataProv.mu.Unlock()

	for {
		// Set a read deadline so ReadMessage doesn't block forever if the server hangs
		if err := conn.SetReadDeadline(time.Now().Add(65 * time.Second)); err != nil {
			return bars, fmt.Errorf("failed to set read deadline: %w", err)
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			if len(bars) > 0 {
				return bars, nil
			}
			return nil, fmt.Errorf("websocket read error: %w", err)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(message, &raw); err != nil {
			continue
		}

		var msgType string
		_ = json.Unmarshal(raw["type"], &msgType)

		switch msgType {
		case "FEED_CONFIG":
			var config struct {
				EventFields map[string][]interface{} `json:"eventFields"`
			}
			if err := json.Unmarshal(message, &config); err == nil {
				if fields, ok := config.EventFields["Candle"]; ok {
					dxFeedDataProv.mu.Lock()
					dxFeedDataProv.updateMapping(fields)
					dxFeedDataProv.mu.Unlock()
				}
			}

		case "FEED_DATA":
			dxFeedDataProv.mu.Lock()
			hasMapping := dxFeedDataProv.recordLen > 0
			dxFeedDataProv.mu.Unlock()

			if !hasMapping {
				continue
			}

			// Activity detected: Reset the safety timeout
			if !timeout.Stop() {
				select {
				case <-timeout.C:
				default:
				}
			}
			timeout.Reset(30 * time.Second)

			newBars := dxFeedDataProv.parseDxBars(raw["data"])
			bars = append(bars, newBars...)

			// Exit condition: Have we reached the requested end time?
			if len(newBars) > 0 {
				if !newBars[len(newBars)-1].Date.Before(end) {
					return bars, nil
				}
			}

		case "ERROR":
			var errMsg struct {
				Err string `json:"error"`
			}
			_ = json.Unmarshal(message, &errMsg)
			return bars, fmt.Errorf("dxfeed error: %s", errMsg.Err)
		}

		// Check the timer
		select {
		case <-timeout.C:
			if len(bars) > 0 {
				return bars, nil
			}
			return nil, fmt.Errorf("collection timed out")
		default:
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
	err := conn.WriteJSON(map[string]interface{}{
		"type":    "CHANNEL_REQUEST",
		"channel": 1,
		"service": "FEED",
		"parameters": map[string]interface{}{
			"contract":  "AUTO",
			"subFormat": "LIST"},
	})
	if err != nil {
		return fmt.Errorf("failed channel request: %w", err)
	}

	// Give the server a moment to process the channel request
	time.Sleep(200 * time.Millisecond)
	return nil
}

// parseDxBars converts raw dxFeed data into a slice of Bar structs.
// It expects rawData to be a nested slice where the second element contains
// a flat array of bar records. Each record is parsed according to the fieldMap
// to extract OHLCV (Open, High, Low, Close, Volume) data and timestamp information.
// Returns an empty slice if rawData is malformed, has insufficient elements,
// or recordLen is not set. Timestamps are expected in milliseconds and are
// converted to UTC time.
func (dxFeedDataProv *DxFeedDataProvider) parseDxBars(rawData interface{}) []Bar {
	var bars []Bar
	topLevel, ok := rawData.([]interface{})
	if !ok || len(topLevel) < 2 || dxFeedDataProv.recordLen == 0 {
		return bars
	}

	flatData, ok := topLevel[1].([]interface{})
	if !ok {
		return bars
	}

	for i := 0; i+dxFeedDataProv.recordLen <= len(flatData); i += dxFeedDataProv.recordLen {
		record := flatData[i : i+dxFeedDataProv.recordLen]

		// Helper to get float safely by field name
		getF64 := func(fieldName string) float64 {
			idx, exists := dxFeedDataProv.fieldMap[fieldName]
			if !exists || idx >= len(record) {
				return 0
			}
			val, _ := record[idx].(float64)
			return val
		}

		tsMillis := getF64("time")

		bars = append(bars, Bar{
			Date:   time.UnixMilli(int64(tsMillis)).UTC(),
			Open:   getF64("open"),
			High:   getF64("high"),
			Low:    getF64("low"),
			Close:  getF64("close"),
			Volume: getF64("volume"),
		})
	}
	return bars
}

// updateMapping initializes the field mapping and record length based on the provided event fields.
// It creates a map that associates field names (strings) with their corresponding indices,
// enabling efficient field lookup by name. The record length is set to the total number of fields.
// If a field is not a string, it is skipped during the mapping process.
func (dxFeedDataProv *DxFeedDataProvider) updateMapping(eventFields []interface{}) {
	dxFeedDataProv.fieldMap = make(map[string]int)
	dxFeedDataProv.recordLen = len(eventFields)
	for i, field := range eventFields {
		if name, ok := field.(string); ok {
			dxFeedDataProv.fieldMap[name] = i
		}
	}
}
