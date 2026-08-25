package stage2_dxfeeddatadownloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func seedExpiredQuoteCache(token, wsURL string) {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()
	tastyAuthCache.quoteToken = token
	tastyAuthCache.quoteURL = wsURL
	// Within the 10-minute skew so resolveDxLinkAuth must refresh.
	tastyAuthCache.quoteExpiry = time.Now().Add(5 * time.Minute)
	tastyAuthCache.accessToken = "stale-access"
	tastyAuthCache.accessExpiry = time.Now().Add(-time.Minute)
	tastyAuthCache.oauthGrantDead = false
}

func TestResolveDxLinkAuth_RefreshesExpiredQuoteToken(t *testing.T) {
	resetDxLinkAuthState()
	t.Cleanup(resetDxLinkAuthState)

	var oauthHits, quoteHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			oauthHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-fresh","expires_in":900}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api-quote-tokens":
			quoteHits.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer access-fresh" {
				t.Errorf("quote auth %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"token":"quote-fresh","dxlink-url":"wss://dxlink.example/realtime","expires-at":"2099-01-01T00:00:00Z"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	t.Setenv("TT_API_BASE", server.URL)
	t.Setenv("TT_REFRESH_TOKEN", "refresh-xyz")
	t.Setenv("TT_CLIENT_SECRET", "secret-xyz")
	t.Setenv("dxlink_token", "")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")

	seedExpiredQuoteCache("quote-stale", "wss://stale.example/realtime")

	auth, err := resolveDxLinkAuth()
	if err != nil {
		t.Fatal(err)
	}
	if oauthHits.Load() != 1 || quoteHits.Load() != 1 {
		t.Fatalf("oauthHits=%d quoteHits=%d want 1 each", oauthHits.Load(), quoteHits.Load())
	}
	if auth.token != "quote-fresh" {
		t.Fatalf("token %q", auth.token)
	}
	if auth.wsURL != "wss://dxlink.example/realtime" {
		t.Fatalf("wsURL %q", auth.wsURL)
	}
}

func TestDXFeed_RefreshExpiredTokenThenHandshake(t *testing.T) {
	resetDxLinkAuthState()
	t.Cleanup(resetDxLinkAuthState)

	var oauthHits, quoteHits atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var wsServer *httptest.Server
	wsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			oauthHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-fresh","expires_in":900}`))
			return
		case "/api-quote-tokens":
			quoteHits.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer access-fresh" {
				t.Errorf("quote auth %q", got)
			}
			dxURL := "ws" + strings.TrimPrefix(wsServer.URL, "http") + "/dxlink"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"token":"quote-fresh","dxlink-url":"` + dxURL + `","expires-at":"2099-01-01T00:00:00Z"}}`))
			return
		case "/dxlink":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("upgrade: %v", err)
				return
			}
			defer conn.Close()
			serveMockDxLinkHandshake(t, conn)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(wsServer.Close)

	t.Setenv("TT_API_BASE", wsServer.URL)
	t.Setenv("TT_REFRESH_TOKEN", "refresh-xyz")
	t.Setenv("TT_CLIENT_SECRET", "secret-xyz")
	t.Setenv("dxlink_token", "")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")

	seedExpiredQuoteCache("quote-stale", "ws://stale.example/realtime")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := Connect(ctx)
	if err != nil {
		t.Fatalf("connect after refresh: %v", err)
	}
	defer client.Close()

	if oauthHits.Load() != 1 || quoteHits.Load() != 1 {
		t.Fatalf("oauthHits=%d quoteHits=%d want 1 each (expired cache must refresh)", oauthHits.Load(), quoteHits.Load())
	}
	if client.token != "quote-fresh" {
		t.Fatalf("client token %q", client.token)
	}

	if err := client.Handshake(ctx); err != nil {
		t.Fatalf("handshake after refresh: %v", err)
	}
}

func serveMockDxLinkHandshake(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Errorf("read SETUP: %v", err)
		return
	}
	var setup struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &setup); err != nil || setup.Type != "SETUP" {
		t.Errorf("want SETUP, got %s", raw)
		return
	}

	_ = conn.WriteJSON(map[string]any{
		"type":    "AUTH_STATE",
		"channel": 0,
		"state":   "UNAUTHORIZED",
	})

	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Errorf("read AUTH: %v", err)
		return
	}
	var auth struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &auth); err != nil || auth.Type != "AUTH" {
		t.Errorf("want AUTH, got %s", raw)
		return
	}
	if auth.Token != "quote-fresh" {
		t.Errorf("AUTH token %q", auth.Token)
		return
	}

	_ = conn.WriteJSON(map[string]any{
		"type":    "AUTH_STATE",
		"channel": 0,
		"state":   "AUTHORIZED",
	})
}

// TestDXFeed_LiveRefreshAndHandshake refreshes via Tastyworks OAuth and completes
// a real dxLink WSS handshake. Requires TT_REFRESH_TOKEN and TT_CLIENT_SECRET.
func TestDXFeed_LiveRefreshAndHandshake(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live dxLink handshake in short mode")
	}

	refresh := strings.TrimSpace(os.Getenv("TT_REFRESH_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("TT_CLIENT_SECRET"))
	if refresh == "" || secret == "" {
		t.Skip("set TT_REFRESH_TOKEN and TT_CLIENT_SECRET to run live dxLink handshake test")
	}

	resetDxLinkAuthState()
	t.Cleanup(resetDxLinkAuthState)

	// Force a refresh path even if a prior run left a cached token.
	seedExpiredQuoteCache("stale-live-token", dxFeedWSURL)

	// Do not use a stale env quote token for this test.
	t.Setenv("dxlink_token", "")
	t.Setenv("DXLINK_TOKEN", "")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client, err := Connect(ctx)
	if err != nil {
		if isInvalidGrantErr(err) {
			t.Skipf("TT_REFRESH_TOKEN grant revoked/invalid: %v", err)
		}
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if client.token == "" || client.token == "stale-live-token" {
		t.Fatalf("expected refreshed quote token, got %q", tokenPrefix(client.token, 12))
	}

	if err := client.Handshake(ctx); err != nil {
		t.Fatalf("live dxLink handshake failed after refresh: %v", err)
	}
	t.Logf("live handshake OK ws=%s tokenPrefix=%s", dxFeedURL(), tokenPrefix(client.token, 8))
}
