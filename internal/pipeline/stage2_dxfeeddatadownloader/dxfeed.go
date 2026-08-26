package stage2_dxfeeddatadownloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/gorilla/websocket"
)

const dxFeedWSURL = "wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime"

type FeedData struct {
	Type    string          `json:"type"`
	Channel int             `json:"channel"`
	Data    []config.Candle `json:"data"`
}

type DXFeedClient struct {
	conn  *websocket.Conn
	token string
	mu    sync.Mutex
}

func Connect(ctx context.Context) (*DXFeedClient, error) {
	auth, err := resolveDxLinkAuth()
	if err != nil {
		return nil, err
	}

	logger.Infof("DXFeed connecting %s", auth.wsURL)
	logger.Tracef(
		"DXFeed AUTH token length=%d prefix=%s suffix=%s",
		len(auth.token),
		tokenPrefix(auth.token, 6),
		tokenSuffix(auth.token, 6),
	)

	headers := http.Header{}
	headers.Set("User-Agent", "option-replay/1.0")
	headers.Set("Origin", tastyOrigin(auth.wsURL))

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, auth.wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to DXFeed: %w", err)
	}
	conn.SetReadLimit(100 * 1024 * 1024)

	return &DXFeedClient{conn: conn, token: auth.token}, nil
}

func (c *DXFeedClient) Close() error {
	_ = c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"),
	)
	return c.conn.Close()
}

func (c *DXFeedClient) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, b)
}

func (c *DXFeedClient) writeRaw(raw string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, []byte(raw))
}

func (c *DXFeedClient) Setup() error {
	const setupJSON = `{"type":"SETUP","channel":0,"keepaliveTimeout":60,"acceptKeepaliveTimeout":60,"version":"1.0.0"}`
	return c.writeRaw(setupJSON)
}

func (c *DXFeedClient) Auth(token string) error {
	payload, err := json.Marshal(struct {
		Type    string `json:"type"`
		Channel int    `json:"channel"`
		Token   string `json:"token"`
	}{
		Type:    "AUTH",
		Channel: 0,
		Token:   token,
	})
	if err != nil {
		return err
	}
	return c.writeRaw(string(payload))
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func dxFeedURL() string {
	url := firstEnv("dxlink_url", "DXLINK_URL", "dxFeed_URL", "DXFEED_URL")
	if url == "" {
		return dxFeedWSURL
	}
	return url
}

func dxFeedToken() (string, error) {
	token := firstEnv("dxlink_token", "DXLINK_TOKEN", "dxFeed_Token", "DXFEED_TOKEN")
	token = strings.Trim(token, `"'`)
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "\ufeff")
	if token == "" {
		return "", fmt.Errorf("environment variable dxlink_token (or dxFeed_Token) is empty")
	}
	return token, nil
}

func tastyAPIBase(wsURL string) string {
	if strings.Contains(strings.ToLower(wsURL), "cert") {
		return "https://api.cert.tastyworks.com"
	}
	return "https://api.tastytrade.com"
}

func tastyOrigin(wsURL string) string {
	if strings.Contains(strings.ToLower(wsURL), "cert") {
		return "https://api.cert.tastyworks.com"
	}
	return "https://api.tastytrade.com"
}

type dxLinkAuth struct {
	token string
	wsURL string
}

func parseDXFeedError(raw []byte) error {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Error == "" && e.Message == "" {
		return fmt.Errorf("dxfeed error: %s", raw)
	}
	return fmt.Errorf("dxfeed error: %s (%s)", e.Error, e.Message)
}

func (c *DXFeedClient) readEnvelope(ctx context.Context) (string, []byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetReadDeadline(deadline)
	}

	_, raw, err := c.conn.ReadMessage()
	if err != nil {
		return "", nil, fmt.Errorf("failed to read from DXFeed: %w", err)
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", raw, nil
	}
	return envelope.Type, raw, nil
}

// Handshake matches the Postman flow: SETUP (version 1.0.0), then AUTH.
func (c *DXFeedClient) Handshake(ctx context.Context) error {
	if c.token == "" {
		return fmt.Errorf("dxlink token is empty")
	}

	if err := c.Setup(); err != nil {
		return fmt.Errorf("failed to setup DXFeed client: %w", err)
	}
	logger.Debugf("DXFeed handshake >> SETUP")

	if err := c.waitUntilReadyForAuth(ctx); err != nil {
		return err
	}

	if err := c.Auth(c.token); err != nil {
		return fmt.Errorf("failed to authenticate with DXFeed: %w", err)
	}
	logger.Debugf("DXFeed handshake >> AUTH")

	return c.waitForAuthorized(ctx)
}

func tokenPrefix(token string, n int) string {
	if len(token) <= n {
		return token
	}
	return token[:n]
}

func tokenSuffix(token string, n int) string {
	if len(token) <= n {
		return token
	}
	return token[len(token)-n:]
}

func (c *DXFeedClient) waitUntilReadyForAuth(ctx context.Context) error {
	for {
		msgType, raw, err := c.readEnvelope(ctx)
		if err != nil {
			return fmt.Errorf("failed to complete DXFeed handshake: %w", err)
		}
		logger.Tracef("DXFeed handshake << %s", raw)

		switch msgType {
		case "AUTH_STATE":
			var msg struct {
				State string `json:"state"`
			}
			if json.Unmarshal(raw, &msg) != nil {
				continue
			}
			if msg.State == "UNAUTHORIZED" || msg.State == "AUTHORIZED" {
				return nil
			}
		case "ERROR":
			var e struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(raw, &e)
			if e.Error == "UNAUTHORIZED" {
				continue
			}
			return parseDXFeedError(raw)
		}
	}
}

func (c *DXFeedClient) waitForAuthorized(ctx context.Context) error {
	for {
		msgType, raw, err := c.readEnvelope(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed (check dxFeed_Token): %w", err)
		}
		logger.Tracef("DXFeed handshake << %s", raw)

		switch msgType {
		case "ERROR":
			return parseDXFeedError(raw)
		case "AUTH":
			continue
		case "AUTH_STATE":
			var msg struct {
				State string `json:"state"`
			}
			if json.Unmarshal(raw, &msg) != nil {
				continue
			}
			if msg.State == "AUTHORIZED" {
				return nil
			}
			if msg.State == "UNAUTHORIZED" {
				return fmt.Errorf("dxfeed authentication failed: UNAUTHORIZED")
			}
		}
	}
}

func (c *DXFeedClient) StartKeepalive(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.writeJSON(map[string]any{
					"type":    "KEEPALIVE",
					"channel": 0,
				}); err != nil {
					return
				}
			}
		}
	}()
}

func (c *DXFeedClient) OpenFeedChannel() error {
	const channelJSON = `{"type":"CHANNEL_REQUEST","channel":1,"service":"FEED","parameters":{"contract":"AUTO","subFormat":"LIST"}}`
	return c.writeRaw(channelJSON)
}

func (c *DXFeedClient) SubscribeCandles(symbols []string, fromTime int64) error {
	add := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		add = append(add, map[string]any{
			"symbol":   symbol,
			"type":     "Candle",
			"fromTime": fromTime,
		})
	}

	return c.writeJSON(map[string]any{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"add":     add,
	})
}

func (c *DXFeedClient) UnsubscribeCandles(symbols []string) error {
	remove := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		remove = append(remove, map[string]any{
			"symbol": symbol,
			"type":   "Candle",
		})
	}

	return c.writeJSON(map[string]any{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"remove":  remove,
	})
}

func chunkSymbols(symbols []string, size int) [][]string {
	if size <= 0 {
		size = len(symbols)
	}
	if len(symbols) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(symbols)+size-1)/size)
	for i := 0; i < len(symbols); i += size {
		end := i + size
		if end > len(symbols) {
			end = len(symbols)
		}
		chunks = append(chunks, symbols[i:end])
	}
	return chunks
}

func (c *DXFeedClient) WaitForChannel(ctx context.Context) error {
	for {
		msgType, raw, err := c.readEnvelope(ctx)
		if err != nil {
			return err
		}

		switch msgType {
		case "ERROR":
			return parseDXFeedError(raw)
		case "CHANNEL_REQUEST":
			continue
		case "CHANNEL_OPENED":
			var msg struct {
				Channel int `json:"channel"`
			}
			if json.Unmarshal(raw, &msg) != nil {
				continue
			}
			if msg.Channel == 1 {
				return nil
			}
		}
	}
}

const (
	dxSnapshotEnd  = 0x08
	dxSnapshotSnip = 0x10
)

func (c *DXFeedClient) ReadLoop(
	ctx context.Context,
	symbols []string,
	handler func(config.Candle) error,
) (alive bool, pendingLeft []string, err error) {
	const noDataWait = 60 * time.Second
	const idleAfterData = 60 * time.Second

	pending := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		pending[symbol] = struct{}{}
	}

	received := false
	var feedMessages, candleCount int

	if err := c.conn.SetReadDeadline(time.Now().Add(noDataWait)); err != nil {
		return false, nil, err
	}

	for {
		if err := ctx.Err(); err != nil {
			return false, pendingSymbols(pending), err
		}

		msgType, raw, err := c.readEnvelope(ctx)
		if err != nil {
			left := pendingSymbols(pending)
			if received && isReadTimeout(err) {
				if len(pending) > 0 {
					logger.Warnf(
						"DXFeed chunk idle with %d symbols pending after %d feed messages, %d candles",
						len(pending),
						feedMessages,
						candleCount,
					)
					return false, left, fmt.Errorf(
						"chunk incomplete: idle with %d symbols pending after %d candles",
						len(pending),
						candleCount,
					)
				}
				logger.Warnf(
					"DXFeed chunk idle after %d feed messages, %d candles",
					feedMessages,
					candleCount,
				)
				return false, nil, nil
			}
			if isReadTimeout(err) {
				return false, left, fmt.Errorf("no candle data received after %s", noDataWait)
			}
			if received {
				logger.Warnf(
					"DXFeed chunk ended after %d feed messages, %d candles: %v",
					feedMessages,
					candleCount,
					err,
				)
				if len(pending) > 0 {
					return false, left, fmt.Errorf(
						"chunk incomplete: connection ended with %d symbols pending: %w",
						len(pending),
						err,
					)
				}
				return false, nil, nil
			}
			return false, left, err
		}

		switch msgType {
		case "KEEPALIVE":
			_ = c.writeJSON(map[string]any{
				"type":    "KEEPALIVE",
				"channel": 0,
			})

		case "FEED_CONFIG":
			logger.Tracef("DXFeed << FEED_CONFIG %s", raw)

		case "ERROR":
			return false, pendingSymbols(pending), parseDXFeedError(raw)

		case "FEED_DATA":
			var msg FeedData
			if err := json.Unmarshal(raw, &msg); err != nil {
				return false, pendingSymbols(pending), fmt.Errorf("failed to unmarshal FEED_DATA: %w", err)
			}

			received = true
			if err := c.conn.SetReadDeadline(time.Now().Add(idleAfterData)); err != nil {
				return false, pendingSymbols(pending), err
			}
			feedMessages++
			for _, candle := range msg.Data {
				if err := handler(candle); err != nil {
					return false, pendingSymbols(pending), fmt.Errorf("failed to handle candle: %w", err)
				}
				candleCount++
				if candle.EventFlags&(dxSnapshotEnd|dxSnapshotSnip) != 0 {
					delete(pending, candle.EventSymbol)
				}
			}
			if feedMessages == 1 || feedMessages%50 == 0 {
				logger.Debugf(
					"DXFeed FEED_DATA messages=%d candles=%d pending=%d last=%s",
					feedMessages,
					candleCount,
					len(pending),
					candleEventSymbol(msg),
				)
			}
			if len(pending) == 0 {
				logger.Debugf(
					"DXFeed chunk snapshot complete messages=%d candles=%d",
					feedMessages,
					candleCount,
				)
				_ = c.conn.SetReadDeadline(time.Time{})
				return true, nil, nil
			}

		default:
			if msgType != "" {
				logger.Tracef("DXFeed << %s", msgType)
			}
		}
	}
}

func pendingSymbols(pending map[string]struct{}) []string {
	if len(pending) == 0 {
		return nil
	}
	out := make([]string, 0, len(pending))
	for symbol := range pending {
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}

// excludeSymbols returns symbols with every entry in skip removed.
func excludeSymbols(symbols, skip []string) []string {
	if len(symbols) == 0 || len(skip) == 0 {
		return append([]string(nil), symbols...)
	}
	drop := make(map[string]struct{}, len(skip))
	for _, symbol := range skip {
		drop[symbol] = struct{}{}
	}
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if _, bad := drop[symbol]; bad {
			continue
		}
		out = append(out, symbol)
	}
	return out
}

func candleEventSymbol(msg FeedData) string {
	if len(msg.Data) == 0 {
		return ""
	}
	return msg.Data[0].EventSymbol
}

func isReadTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline")
}

func ToDXFeedSymbol(contract db.Contract) string {
	if db.IsSpotContract(contract) {
		return fmt.Sprintf("%s{=m}", contract.Underlying)
	}

	optionType := "C"
	if strings.EqualFold(contract.Type, "put") {
		optionType = "P"
	}

	return fmt.Sprintf(
		".%s%s%s%g{=m}",
		contract.Underlying,
		contract.Expiry.Format("060102"),
		optionType,
		contract.Strike,
	)
}
