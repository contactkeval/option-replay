package stage2_dxfeeddatadownloader

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseOAuthToken(t *testing.T) {
	token, expiresIn, err := parseOAuthToken([]byte(`{"access_token":"abc","expires_in":900}`))
	if err != nil {
		t.Fatal(err)
	}
	if token != "abc" || expiresIn != 900 {
		t.Fatalf("got %q %d", token, expiresIn)
	}
}

func TestParseQuoteTokenWrapped(t *testing.T) {
	body := []byte(`{"data":{"token":"qt","dxlink-url":"wss://example/realtime","expires-at":"2026-08-18T01:00:00Z"}}`)
	token, wsURL, expiresAt, err := parseQuoteToken(body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "qt" || wsURL != "wss://example/realtime" {
		t.Fatalf("got %q %q", token, wsURL)
	}
	if expiresAt.UTC().Format(time.RFC3339) != "2026-08-18T01:00:00Z" {
		t.Fatalf("expires %s", expiresAt)
	}
}

func TestResolveDxLinkAuth_OAuthThenQuoteToken(t *testing.T) {
	invalidateDxLinkAuth()
	t.Cleanup(invalidateDxLinkAuth)

	var sawOAuth, sawQuote bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			sawOAuth = true
			if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
				t.Errorf("oauth content-type %q", ct)
			}
			body, _ := io.ReadAll(r.Body)
			form := string(body)
			if !strings.Contains(form, "grant_type=refresh_token") ||
				!strings.Contains(form, "refresh_token=refresh-xyz") ||
				!strings.Contains(form, "client_secret=secret-xyz") {
				t.Errorf("oauth form %q", form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-xyz","expires_in":900}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api-quote-tokens":
			sawQuote = true
			if got := r.Header.Get("Authorization"); got != "Bearer access-xyz" {
				t.Errorf("quote auth %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"token":"quote-xyz","dxlink-url":"wss://dxlink.example/realtime","expires-at":"2099-01-01T00:00:00Z"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	t.Setenv("TT_API_BASE", server.URL)
	t.Setenv("TT_REFRESH_TOKEN", "refresh-xyz")
	t.Setenv("TT_CLIENT_SECRET", "secret-xyz")
	t.Setenv("dxlink_token", "")
	t.Setenv("DXLINK_TOKEN", "")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")

	auth, err := resolveDxLinkAuth()
	if err != nil {
		t.Fatal(err)
	}
	if !sawOAuth || !sawQuote {
		t.Fatalf("sawOAuth=%v sawQuote=%v", sawOAuth, sawQuote)
	}
	if auth.token != "quote-xyz" {
		t.Fatalf("token %q", auth.token)
	}
	if auth.wsURL != "wss://dxlink.example/realtime" {
		t.Fatalf("wsURL %q", auth.wsURL)
	}

	sawOAuth, sawQuote = false, false
	auth, err = resolveDxLinkAuth()
	if err != nil {
		t.Fatal(err)
	}
	if sawOAuth || sawQuote {
		t.Fatal("expected cached quote token")
	}
	if auth.token != "quote-xyz" {
		t.Fatalf("cached token %q", auth.token)
	}
}

func TestFormatOAuthHTTPError_InvalidGrant(t *testing.T) {
	err := formatOAuthHTTPError(400, []byte(`{"error_code":"invalid_grant","error_description":"Grant revoked"}`))
	if !isInvalidGrantErr(err) {
		t.Fatalf("expected invalid_grant detection, got %v", err)
	}
}

func TestResolveDxLinkAuth_OAuthInvalidGrantFallsBackToEnvToken(t *testing.T) {
	invalidateDxLinkAuth()
	t.Cleanup(invalidateDxLinkAuth)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error_code":"invalid_grant","error_description":"Grant revoked"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	t.Setenv("TT_API_BASE", server.URL)
	t.Setenv("TT_REFRESH_TOKEN", "revoked")
	t.Setenv("TT_CLIENT_SECRET", "secret")
	t.Setenv("dxlink_token", "env-quote-token")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")

	auth, err := resolveDxLinkAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.token != "env-quote-token" {
		t.Fatalf("token %q", auth.token)
	}
}

func TestTastyRESTBase(t *testing.T) {
	t.Setenv("TT_API_BASE", "")
	t.Setenv("TASTY_API_BASE", "")
	t.Setenv("dxlink_url", "")
	t.Setenv("DXLINK_URL", "")
	t.Setenv("dxFeed_URL", "")
	t.Setenv("DXFEED_URL", "")
	if got := tastyRESTBase(); got != defaultTastyRESTBase {
		t.Fatalf("prod rest base %s", got)
	}

	t.Setenv("dxlink_url", "wss://tasty-cert-openapi-dxlink-md.dxfeed.com/realtime")
	if got := tastyRESTBase(); got != "https://api.cert.tastyworks.com" {
		t.Fatalf("cert rest base %s", got)
	}
}
