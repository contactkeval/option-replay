package stage1_dxfeed

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type FeedData struct {
	Type    string          `json:"type"`
	Channel int             `json:"channel"`
	Data    []config.Candle `json:"data"`
}

type Client struct {
	conn *websocket.Conn

	mu sync.Mutex
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

	return &Client{
		conn: conn,
	}, nil
}

func (c *Client) writeJSON(v any) error {

	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.conn.Write(
		context.Background(),
		websocket.MessageText,
		b,
	)
}

func (c *Client) Setup() error {

	return c.writeJSON(map[string]any{
		"type": "SETUP",

		"channel": 0,

		"keepaliveTimeout":       60,
		"acceptKeepaliveTimeout": 60,

		"version": "1.0.0",
	})
}

func (c *Client) Auth(
	token string,
) error {

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
			"subFormat": "COMPACT", // other option is "LIST"
		},
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

				_ = c.writeJSON(map[string]any{
					"type":    "KEEPALIVE",
					"channel": 0,
				})
			}
		}
	}()
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

func (c *Client) ReadLoop(
	ctx context.Context,
	handler func(config.Candle) error,
) error {

	for {

		_, raw, err := c.conn.Read(ctx)
		if err != nil {
			return err
		}

		var envelope struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}

		switch envelope.Type {

		case "KEEPALIVE":
			continue

		case "FEED_DATA":

			var msg FeedData

			if err := json.Unmarshal(raw, &msg); err != nil {
				return err
			}

			for _, candle := range msg.Data {

				if err := handler(candle); err != nil {
					return err
				}
			}
		}
	}
}
