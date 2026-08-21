package stage1_dxfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type FeedData struct {
	Type    string          `json:"type"`
	Channel int             `json:"channel"`
	Data    []config.Candle `json:"data"`
}

type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func Connect(
	ctx context.Context,
	url string,
) (*Client, error) {

	conn, _, err := websocket.Dial(
		ctx,
		url,
		nil,
	)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(100 * 1024 * 1024)

	return &Client{
		conn: conn,
	}, nil
}

func (c *Client) Close() error {

	_, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	return c.conn.Close(
		websocket.StatusNormalClosure,
		"client shutdown",
	)
}

func (c *Client) writeJSON(v any) error {

	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	return c.conn.Write(
		ctx,
		websocket.MessageText,
		b,
	)
}

func (c *Client) Setup() error {

	return c.writeJSON(map[string]any{
		"type":                   "SETUP",
		"channel":                0,
		"keepaliveTimeout":       60,
		"acceptKeepaliveTimeout": 60,
		"version":                "1.0.0",
	})
}

func (c *Client) Auth(token string) error {

	return c.writeJSON(map[string]any{
		"type":    "AUTH",
		"channel": 0,
		"token":   token,
	})
}

func (c *Client) OpenFeedChannel() error {

	return c.writeJSON(map[string]any{
		"type":    "CHANNEL_REQUEST",
		"channel": 1,
		"service": "FEED",
		"parameters": map[string]any{
			"contract":  "AUTO",
			"subFormat": "LIST",
		},
	})
}

func (c *Client) SubscribeCandles(
	symbols []string,
	fromTime int64,
) error {

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

func (c *Client) StartKeepalive(
	ctx context.Context,
) {

	go func() {

		ticker := time.NewTicker(
			30 * time.Second,
		)

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
					logger.Warnf("Failed to send keepalive: %v", err)
				}
			}
		}
	}()
}

func (c *Client) WaitForAuth(
	ctx context.Context,
) error {

	for {

		_, raw, err := c.conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("failed to read from dxfeed: %w", err)
		}

		logger.Tracef("%s", raw)

		var msg struct {
			Type  string `json:"type"`
			State string `json:"state"`
		}

		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		if msg.Type == "AUTH_STATE" &&
			msg.State == "AUTHORIZED" {
			return nil
		}
	}
}

func (c *Client) WaitForChannel(
	ctx context.Context,
	channel int,
) error {

	for {

		_, raw, err := c.conn.Read(ctx)
		if err != nil {
			return err
		}

		logger.Tracef("%s", raw)

		var msg struct {
			Type    string `json:"type"`
			Channel int    `json:"channel"`
		}

		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		if msg.Type == "CHANNEL_OPENED" &&
			msg.Channel == channel {
			return nil
		}
	}
}

func (c *Client) ReadLoop(
	ctx context.Context,
	handler func(config.Candle) error,
) error {

	keepAliveCount := 0
	feedDataMessages := 0
	candleCount := 0

	for {

		_, raw, err := c.conn.Read(ctx)
		if err != nil {
			logger.Infof(
				"FINAL STATS: feedData=%d candles=%d",
				feedDataMessages,
				candleCount,
			)
			return fmt.Errorf("failed to read from dxfeed: %w", err)
		}
		// logger.Tracef("%s", raw) // TODO: remove this after debugging (TEMP)

		var envelope struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}

		switch envelope.Type {

		case "KEEPALIVE":
			keepAliveCount++
			if keepAliveCount >= 1 {
				return nil
			}
			continue

		case "ERROR":

			var e struct {
				Type    string `json:"type"`
				Error   string `json:"error"`
				Message string `json:"message"`
			}

			_ = json.Unmarshal(raw, &e)

			return fmt.Errorf(
				"dxfeed error: %s (%s)",
				e.Error,
				e.Message,
			)

		case "FEED_DATA":
			keepAliveCount = 0
			var msg FeedData

			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("failed to unmarshal feed data: %w", err)
			}

			for _, candle := range msg.Data {

				if err := handler(candle); err != nil {
					return fmt.Errorf("failed to handle candle: %w", err)
				}
			}
			feedDataMessages++
			candleCount += len(msg.Data)

			if feedDataMessages%100 == 0 {
				logger.Debugf(
					"feedData=%d candles=%d",
					feedDataMessages,
					candleCount,
				)
			}

		default:
			logger.Tracef(
				"UNHANDLED: %s",
				string(raw),
			)
		}
	}
}

func Run(cfg config.Config) error {
	return LoadContractsToSQLite(cfg)
}

func Run2() error {

	ctx := context.Background()

	dxLinkURL := "wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime"
	dxToken := os.Getenv("dxFeed_Token")

	fromTime := time.Now().
		Add(-24 * 120 * time.Hour).
		UnixMilli()

	client, err := Connect(
		ctx,
		dxLinkURL,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to dxfeed: %w", err)
	}

	defer client.Close()

	if err := client.Setup(); err != nil {
		return fmt.Errorf("failed to setup dxfeed client: %w", err)
	}

	if err := client.Auth(dxToken); err != nil {
		return fmt.Errorf("failed to authenticate with dxfeed: %w", err)
	}

	if err := client.WaitForAuth(ctx); err != nil {
		return fmt.Errorf("failed to wait for authentication: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := client.OpenFeedChannel(); err != nil {
		return fmt.Errorf("failed to open feed channel: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := client.WaitForChannel(ctx, 1); err != nil {
		return fmt.Errorf("failed to wait for channel: %w", err)
	}

	client.StartKeepalive(ctx)

	if err := client.SubscribeCandles(
		[]string{
			".SPY260731C735{=1d}",
			".SPY260630C735{=1d}",
			".SPY260529C735{=1d}",
			".SPY260430C735{=1d}",
		},
		fromTime,
	); err != nil {
		return fmt.Errorf("failed to subscribe to candles: %w", err)
	}

	err = client.ReadLoop(
		ctx,
		func(c config.Candle) error {

			logger.Tracef(
				"%s | %s | O=%.2f H=%.2f L=%.2f C=%.2f",
				time.UnixMilli(c.Time).Format(time.RFC3339),
				c.EventSymbol,
				c.Open,
				c.High,
				c.Low,
				c.Close,
			)

			return nil
		},
	)

	if err != nil {
		return fmt.Errorf("failed to read loop: %w", err)
	}

	return nil
}
