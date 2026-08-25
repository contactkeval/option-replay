package stage2_dxfeeddatadownloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

const defaultTastyRESTBase = "https://api.tastyworks.com"

type dxLinkAuthCache struct {
	mu             sync.Mutex
	accessToken    string
	accessExpiry   time.Time
	quoteToken     string
	quoteURL       string
	quoteExpiry    time.Time
	oauthGrantDead bool // invalid_grant; needs a new TT_REFRESH_TOKEN
}

var tastyAuthCache dxLinkAuthCache

var tastyHTTPClient = &http.Client{Timeout: 30 * time.Second}

func tastyRESTBase() string {
	if v := firstEnv("TT_API_BASE", "TASTY_API_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if strings.Contains(strings.ToLower(dxFeedURL()), "cert") {
		return "https://api.cert.tastyworks.com"
	}
	return defaultTastyRESTBase
}

func canRefreshTastyOAuth() bool {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()
	return !tastyAuthCache.oauthGrantDead &&
		firstEnv("TT_REFRESH_TOKEN") != "" &&
		firstEnv("TT_CLIENT_SECRET") != ""
}

func invalidateDxLinkAuth() {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()
	tastyAuthCache.accessToken = ""
	tastyAuthCache.accessExpiry = time.Time{}
	tastyAuthCache.quoteToken = ""
	tastyAuthCache.quoteURL = ""
	tastyAuthCache.quoteExpiry = time.Time{}
}

// resetDxLinkAuthState clears cached tokens and the revoked-grant latch (tests).
func resetDxLinkAuthState() {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()
	tastyAuthCache.accessToken = ""
	tastyAuthCache.accessExpiry = time.Time{}
	tastyAuthCache.quoteToken = ""
	tastyAuthCache.quoteURL = ""
	tastyAuthCache.quoteExpiry = time.Time{}
	tastyAuthCache.oauthGrantDead = false
}

func oauthGrantIsDead() bool {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()
	return tastyAuthCache.oauthGrantDead
}

func resolveDxLinkAuth() (dxLinkAuth, error) {
	return resolveDxLinkAuthLocked(false)
}

func resolveDxLinkAuthLocked(force bool) (dxLinkAuth, error) {
	tastyAuthCache.mu.Lock()
	defer tastyAuthCache.mu.Unlock()

	if force {
		tastyAuthCache.quoteToken = ""
		tastyAuthCache.quoteExpiry = time.Time{}
	}

	wsURL := dxFeedURL()
	if tastyAuthCache.quoteToken != "" && time.Now().Add(10*time.Minute).Before(tastyAuthCache.quoteExpiry) {
		if tastyAuthCache.quoteURL != "" {
			wsURL = tastyAuthCache.quoteURL
		}
		logger.Debugf("DXFeed using cached quote token")
		return dxLinkAuth{token: tastyAuthCache.quoteToken, wsURL: wsURL}, nil
	}

	if tastyAuthCache.oauthGrantDead {
		return dxLinkAuth{}, revokedGrantError(nil)
	}

	if firstEnv("TT_REFRESH_TOKEN") != "" && firstEnv("TT_CLIENT_SECRET") != "" {
		accessToken, err := ensureTastyAccessTokenLocked()
		if err != nil {
			if isInvalidGrantErr(err) {
				tastyAuthCache.oauthGrantDead = true
				return dxLinkAuth{}, revokedGrantError(err)
			}
			return dxLinkAuth{}, err
		}

		quoteToken, quoteURL, quoteExpiry, err := fetchQuoteToken(accessToken)
		if err != nil {
			return dxLinkAuth{}, err
		}

		tastyAuthCache.quoteToken = quoteToken
		tastyAuthCache.quoteURL = quoteURL
		tastyAuthCache.quoteExpiry = quoteExpiry
		if quoteURL != "" {
			wsURL = quoteURL
		}
		logger.Infof(
			"DXFeed refreshed quote token length=%d prefix=%s suffix=%s expires=%s",
			len(quoteToken),
			tokenPrefix(quoteToken, 6),
			tokenSuffix(quoteToken, 6),
			quoteExpiry.Format(time.RFC3339),
		)
		return dxLinkAuth{token: quoteToken, wsURL: wsURL}, nil
	}

	auth, err := authFromEnvToken(wsURL)
	if err != nil {
		return dxLinkAuth{}, err
	}
	return auth, nil
}

func revokedGrantError(cause error) error {
	msg := "TT_REFRESH_TOKEN grant is revoked/invalid; create a new personal OAuth grant at https://developer.tastytrade.com and update TT_REFRESH_TOKEN (keep TT_CLIENT_SECRET). A stale dxlink_token cannot be refreshed without a valid grant."
	if cause == nil {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("%w; %s", cause, msg)
}

func authFromEnvToken(wsURL string) (dxLinkAuth, error) {
	token, err := dxFeedToken()
	if err != nil {
		return dxLinkAuth{}, fmt.Errorf(
			"%w; set TT_REFRESH_TOKEN and TT_CLIENT_SECRET to refresh dxLink quote tokens",
			err,
		)
	}
	logger.Warnf("DXFeed using env token as AUTH token (no OAuth refresh credentials)")
	return dxLinkAuth{token: token, wsURL: wsURL}, nil
}

func ensureTastyAccessTokenLocked() (string, error) {
	if tastyAuthCache.accessToken != "" && time.Now().Add(2*time.Minute).Before(tastyAuthCache.accessExpiry) {
		return tastyAuthCache.accessToken, nil
	}

	accessToken, expiresAt, err := refreshTastyAccessToken()
	if err != nil {
		return "", err
	}

	tastyAuthCache.accessToken = accessToken
	tastyAuthCache.accessExpiry = expiresAt
	logger.Infof("DXFeed refreshed Tastyworks access token expires=%s", expiresAt.Format(time.RFC3339))
	return accessToken, nil
}

func refreshTastyAccessToken() (string, time.Time, error) {
	refreshToken := firstEnv("TT_REFRESH_TOKEN")
	clientSecret := firstEnv("TT_CLIENT_SECRET")
	clientID := firstEnv("TT_CLIENT_ID")

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_secret", clientSecret)
	if clientID != "" {
		form.Set("client_id", clientID)
	}

	req, err := http.NewRequest(http.MethodPost, tastyRESTBase()+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "option-replay/1.0")

	resp, err := tastyHTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oauth/token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("%w", formatOAuthHTTPError(resp.StatusCode, body))
	}

	accessToken, expiresIn, err := parseOAuthToken(body)
	if err != nil {
		return "", time.Time{}, err
	}
	if expiresIn <= 0 {
		expiresIn = 900
	}
	return accessToken, time.Now().Add(time.Duration(expiresIn) * time.Second), nil
}

func parseOAuthToken(body []byte) (string, int, error) {
	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Data        struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", 0, fmt.Errorf("oauth/token decode: %w", err)
	}
	token := strings.TrimSpace(res.AccessToken)
	expiresIn := res.ExpiresIn
	if token == "" {
		token = strings.TrimSpace(res.Data.AccessToken)
		expiresIn = res.Data.ExpiresIn
	}
	if token == "" {
		return "", 0, fmt.Errorf("oauth/token: empty access_token")
	}
	return token, expiresIn, nil
}

func fetchQuoteToken(accessToken string) (string, string, time.Time, error) {
	req, err := http.NewRequest(http.MethodGet, tastyRESTBase()+"/api-quote-tokens", nil)
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "option-replay/1.0")

	resp, err := tastyHTTPClient.Do(req)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("api-quote-tokens: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", time.Time{}, fmt.Errorf("api-quote-tokens HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	token, dxLinkURL, expiresAt, err := parseQuoteToken(body)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(24 * time.Hour)
	}
	return token, dxLinkURL, expiresAt, nil
}

func parseQuoteToken(body []byte) (string, string, time.Time, error) {
	var result struct {
		Token     string `json:"token"`
		DxLinkURL string `json:"dxlink-url"`
		ExpiresAt string `json:"expires-at"`
		Data      struct {
			Token     string `json:"token"`
			DxLinkURL string `json:"dxlink-url"`
			ExpiresAt string `json:"expires-at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", time.Time{}, fmt.Errorf("api-quote-tokens decode: %w", err)
	}

	token := strings.TrimSpace(result.Data.Token)
	dxLinkURL := result.Data.DxLinkURL
	expiresRaw := result.Data.ExpiresAt
	if token == "" {
		token = strings.TrimSpace(result.Token)
		dxLinkURL = result.DxLinkURL
		expiresRaw = result.ExpiresAt
	}
	if token == "" {
		return "", "", time.Time{}, fmt.Errorf("api-quote-tokens: empty token")
	}

	var expiresAt time.Time
	if expiresRaw != "" {
		if t, err := time.Parse(time.RFC3339, expiresRaw); err == nil {
			expiresAt = t
		}
	}
	return token, dxLinkURL, expiresAt, nil
}

func formatOAuthHTTPError(status int, body []byte) error {
	msg := truncateBody(body)
	var payload struct {
		Error            string `json:"error"`
		ErrorCode        string `json:"error_code"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &payload)
	code := strings.TrimSpace(payload.ErrorCode)
	if code == "" {
		code = strings.TrimSpace(payload.Error)
	}
	if code == "invalid_grant" || strings.Contains(strings.ToLower(payload.ErrorDescription), "grant revoked") {
		return fmt.Errorf(
			"oauth/token HTTP %d: invalid_grant (refresh token revoked or expired): %s",
			status,
			msg,
		)
	}
	return fmt.Errorf("oauth/token HTTP %d: %s", status, msg)
}

func isInvalidGrantErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid_grant") || strings.Contains(msg, "grant revoked")
}

func truncateBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

func isUnauthorizedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized")
}
