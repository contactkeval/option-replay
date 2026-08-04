package stage2_dxfeeddatadownloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type FeedData struct {
	Type    string          `json:"type"`
	Channel int             `json:"channel"`
	Data    []config.Candle `json:"data"`
}

type DXFeedClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func Connect(
	ctx context.Context,
) (*DXFeedClient, error) {

	const wsURL = "wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime"
	conn, _, err := websocket.Dial(
		ctx,
		wsURL,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to DXFeed: %w", err)
	}

	conn.SetReadLimit(
		100 * 1024 * 1024,
	)

	return &DXFeedClient{
		conn: conn,
	}, nil
}

func (c *DXFeedClient) Close() error {

	return c.conn.Close(
		websocket.StatusNormalClosure,
		"client shutdown",
	)
}

func (c *DXFeedClient) writeJSON(
	v any,
) error {

	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
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

func (c *DXFeedClient) Setup() error {

	return c.writeJSON(map[string]any{
		"type":                   "SETUP",
		"channel":                0,
		"keepaliveTimeout":       60,
		"acceptKeepaliveTimeout": 60,
		"version":                "1.0.0",
	})
}

func (c *DXFeedClient) Auth() error {

	return c.writeJSON(map[string]any{
		"type":    "AUTH",
		"channel": 0,
		"token":   os.Getenv("dxFeed_Token"),
	})
}

func (c *DXFeedClient) OpenFeedChannel() error {

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

func (c *DXFeedClient) SubscribeCandles(
	symbols []string,
	fromTime int64,
) error {

	add :=
		make(
			[]map[string]any,
			0,
			len(symbols),
		)

	for _, symbol := range symbols {

		add = append(
			add,
			map[string]any{
				"symbol":   symbol,
				"type":     "Candle",
				"fromTime": fromTime,
			},
		)
	}

	return c.writeJSON(
		map[string]any{
			"type":    "FEED_SUBSCRIPTION",
			"channel": 1,
			"add":     add,
		},
	)
}

func (c *DXFeedClient) WaitForAuth(
	ctx context.Context,
) error {

	for {

		_, raw, err :=
			c.conn.Read(ctx)

		if err != nil {
			return fmt.Errorf("failed to read from DXFeed: %w", err)
		}

		var msg struct {
			Type  string `json:"type"`
			State string `json:"state"`
		}

		if json.Unmarshal(
			raw,
			&msg,
		) != nil {
			continue
		}

		if msg.Type == "AUTH_STATE" &&
			msg.State == "AUTHORIZED" {

			return nil
		}
	}
}

func (c *DXFeedClient) WaitForChannel(
	ctx context.Context,
) error {

	for {

		_, raw, err :=
			c.conn.Read(ctx)

		if err != nil {
			return fmt.Errorf("failed to read from DXFeed: %w", err)
		}

		var msg struct {
			Type    string `json:"type"`
			Channel int    `json:"channel"`
		}

		if json.Unmarshal(
			raw,
			&msg,
		) != nil {
			continue
		}

		if msg.Type == "CHANNEL_OPENED" &&
			msg.Channel == 1 {

			return nil
		}
	}
}

func (c *DXFeedClient) ReadLoop(
	ctx context.Context,
	handler func(config.Candle) error,
) error {

	for {

		_, raw, err :=
			c.conn.Read(ctx)

		if err != nil {
			return fmt.Errorf("failed to read from DXFeed: %w", err)
		}

		var envelope struct {
			Type string `json:"type"`
		}

		if json.Unmarshal(
			raw,
			&envelope,
		) != nil {

			continue
		}

		// fmt.Println("raw:", string(raw))
		// fmt.Printf(
		// 	"MESSAGE TYPE: %s\n",
		// 	envelope.Type,
		// )

		switch envelope.Type {

		case "KEEPALIVE":

			return nil

		case "ERROR":

			var e struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}

			_ = json.Unmarshal(
				raw,
				&e,
			)

			return fmt.Errorf(
				"dxfeed error: %s (%s)",
				e.Error,
				e.Message,
			)

		case "FEED_DATA":

			var msg FeedData

			if err := json.Unmarshal(
				raw,
				&msg,
			); err != nil {

				return fmt.Errorf("failed to unmarshal FEED_DATA: %w", err)
			}

			for _, candle := range msg.Data {

				if err :=
					handler(candle); err != nil {

					return fmt.Errorf("failed to handle candle: %w", err)
				}
			}
		}
	}
}

func ToDXFeedSymbol(
	contract db.Contract,
) string {

	optionType := "C"

	if strings.EqualFold(
		contract.Type,
		"put",
	) {
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
