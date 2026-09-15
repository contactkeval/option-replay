package stage2_dxfeeddatadownloader

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// statDrops counts FEED_DATA items that could not be decoded as a Quote or
// Trade (used for diagnostics when the feed switches to an unexpected shape).
var statDrops atomic.Int64

// LiveEvent is a single Quote or Trade delivered by dxLink's steady-state
// FEED stream. Unused fields are zero for the other event kind.
type LiveEvent struct {
	Kind    string // "Quote" or "Trade"
	Symbol  string
	Bid     float64
	Ask     float64
	BidSize float64
	AskSize float64
	Price   float64
	Size    float64
	Seq     int64
	Time    time.Time
}

// streamFields caches the FEED_CONFIG field mapping so LIST-form FEED_DATA
// entries can be decoded by position when the server does not send objects.
type streamFields struct {
	mu      sync.RWMutex
	byType  map[string][]string
	byCount map[int]string
}

func (s *streamFields) set(eventFields map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byType = make(map[string][]string, len(eventFields))
	s.byCount = make(map[int]string, len(eventFields))
	for kind, fields := range eventFields {
		s.byType[kind] = append([]string(nil), fields...)
		s.byCount[len(fields)] = kind
	}
}

func (s *streamFields) fields(kind string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byType[kind]
}

func (s *streamFields) kindByFieldCount(n int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byCount[n]
}

// SubscribeEvents adds a Quote or Trade subscription for each symbol.
// eventType must be "Quote" or "Trade".
func (c *DXFeedClient) SubscribeEvents(symbols []string, eventType string) error {
	if len(symbols) == 0 {
		return nil
	}
	add := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		add = append(add, map[string]any{
			"symbol": symbol,
			"type":   eventType,
		})
	}
	return c.writeJSON(map[string]any{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"add":     add,
	})
}

// UnsubscribeEvents removes a Quote or Trade subscription for each symbol.
func (c *DXFeedClient) UnsubscribeEvents(symbols []string, eventType string) error {
	if len(symbols) == 0 {
		return nil
	}
	remove := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		remove = append(remove, map[string]any{
			"symbol": symbol,
			"type":   eventType,
		})
	}
	return c.writeJSON(map[string]any{
		"type":    "FEED_SUBSCRIPTION",
		"channel": 1,
		"remove":  remove,
	})
}

// ReadStream reads the FEED stream until ctx is cancelled or the server sends
// an ERROR or the connection breaks. Quote/Trade events are passed to handler.
func (c *DXFeedClient) ReadStream(ctx context.Context, handler func(LiveEvent) error) error {
	var fields streamFields

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		msgType, raw, err := c.readEnvelope(ctx)
		if err != nil {
			return err
		}

		switch msgType {
		case "KEEPALIVE":
			if err := c.writeJSON(map[string]any{
				"type":    "KEEPALIVE",
				"channel": 0,
			}); err != nil {
				return err
			}

		case "FEED_CONFIG":
			var cfg struct {
				EventFields map[string][]string `json:"eventFields"`
			}
			if err := json.Unmarshal(raw, &cfg); err == nil {
				fields.set(cfg.EventFields)
			}

		case "ERROR":
			return parseDXFeedError(raw)

		case "FEED_DATA":
			var msg struct {
				Data []json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("failed to unmarshal FEED_DATA: %w", err)
			}
			for _, item := range msg.Data {
				ev := decodeLiveEvent(item, &fields)
				if ev == nil {
					n := statDrops.Add(1)
					if n <= 5 || n%1000 == 0 {
						logger.Debugf("dropped undecodable FEED_DATA item #%d: %.80s", n, item)
					}
					continue
				}
				if err := handler(*ev); err != nil {
					return err
				}
			}
		}
	}
}

// decodeLiveEvent parses one FEED_DATA entry. Object-form entries (primary;
// this is what the pipeline's candle subscriptions receive) carry eventType /
// eventSymbol / named fields. LIST-form entries ([symbol, [values...]]) are
// decoded as a fallback using the cached FEED_CONFIG fields.
func decodeLiveEvent(item json.RawMessage, fields *streamFields) *LiveEvent {
	obj, err := parseObject(item)
	if err == nil {
		if ev := decodeObjectEvent(obj); ev != nil {
			return ev
		}
	}

	var list []json.RawMessage
	if err := json.Unmarshal(item, &list); err != nil || len(list) < 2 {
		return nil
	}
	symbol := strings.Trim(strVal(list[0]), `"'`)
	var values []json.RawMessage
	if err := json.Unmarshal(list[1], &values); err != nil {
		return nil
	}
	kind := fields.kindByFieldCount(len(values))
	if kind == "" {
		return nil
	}
	names := fields.fields(kind)
	ev := &LiveEvent{Kind: kind, Symbol: symbol}
	for i, name := range names {
		if i >= len(values) {
			break
		}
		switch name {
		case "bidPrice":
			ev.Bid = numVal(values[i])
		case "bidSize":
			ev.BidSize = numVal(values[i])
		case "askPrice":
			ev.Ask = numVal(values[i])
		case "askSize":
			ev.AskSize = numVal(values[i])
		case "price":
			ev.Price = numVal(values[i])
		case "size":
			ev.Size = numVal(values[i])
		case "sequence":
			ev.Seq = int64(numVal(values[i]))
		case "time":
			ev.Time = unixMsTime(numVal(values[i]))
		}
	}
	if ev.Kind == "" || ev.Symbol == "" {
		return nil
	}
	return ev
}

func parseObject(item json.RawMessage) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(item, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func decodeObjectEvent(m map[string]json.RawMessage) *LiveEvent {
	kind := strVal(m["eventType"])
	symbol := strVal(m["eventSymbol"])
	if (kind != "Quote" && kind != "Trade") || symbol == "" {
		return nil
	}
	ev := &LiveEvent{
		Kind:    kind,
		Symbol:  symbol,
		Bid:     numVal(m["bidPrice"]),
		Ask:     numVal(m["askPrice"]),
		BidSize: numVal(m["bidSize"]),
		AskSize: numVal(m["askSize"]),
		Price:   numVal(m["price"]),
		Size:    numVal(m["size"]),
		Seq:     int64(numVal(m["sequence"])),
	}
	if t := numVal(m["time"]); t != 0 {
		ev.Time = unixMsTime(t)
	} else {
		ev.Time = unixMsTime(numVal(m["eventTime"]))
	}
	return ev
}

func strVal(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

func numVal(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return finite(f)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return finite(v)
		}
	}
	return 0
}

// finite coerces dxLink's NaN/Infinity "no value" sentinels to 0 so that
// snapshots can always be JSON-marshaled and rendered.
func finite(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func unixMsTime(ms float64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC()
}
