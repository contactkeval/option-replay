package stage2_dxfeeddatadownloader

import "testing"

func TestDxFeedToken_Empty(t *testing.T) {
	t.Setenv("dxlink_token", "")
	t.Setenv("DXLINK_TOKEN", "")
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")
	if _, err := dxFeedToken(); err == nil {
		t.Fatal("expected error when token env is empty")
	}
}

func TestDxFeedToken_Trimmed(t *testing.T) {
	t.Setenv("dxlink_token", "")
	t.Setenv("DXLINK_TOKEN", "")
	t.Setenv("dxFeed_Token", `  "abc"  `)
	token, err := dxFeedToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "abc" {
		t.Fatalf("want abc, got %q", token)
	}
}

func TestDxFeedToken_FromDxlinkToken(t *testing.T) {
	t.Setenv("dxFeed_Token", "")
	t.Setenv("DXFEED_TOKEN", "")
	t.Setenv("dxlink_token", "postman-token")
	token, err := dxFeedToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "postman-token" {
		t.Fatalf("got %q", token)
	}
}

func TestTastyAPIBase(t *testing.T) {
	if got := tastyAPIBase("wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime"); got != "https://api.tastytrade.com" {
		t.Fatalf("prod: %s", got)
	}
	if got := tastyAPIBase("wss://tasty-cert-openapi-dxlink-md.dxfeed.com/realtime"); got != "https://api.cert.tastyworks.com" {
		t.Fatalf("cert: %s", got)
	}
}

func TestDownloadPoolConstants(t *testing.T) {
	if MaxCandleSubscribe != 100 {
		t.Fatalf("MaxCandleSubscribe=%d", MaxCandleSubscribe)
	}
	if DownloadWorkers != 4 {
		t.Fatalf("DownloadWorkers=%d", DownloadWorkers)
	}
}

func TestChunkSymbols(t *testing.T) {
	symbols := []string{"a", "b", "c", "d", "e"}
	chunks := chunkSymbols(symbols, 2)
	if len(chunks) != 3 {
		t.Fatalf("chunks=%d", len(chunks))
	}
	if len(chunks[0]) != 2 || len(chunks[2]) != 1 {
		t.Fatalf("chunk sizes %v", chunks)
	}
}

func TestTokenPrefixSuffix(t *testing.T) {
	if got := tokenPrefix("abcdefghij", 6); got != "abcdef" {
		t.Fatalf("prefix: %q", got)
	}
	if got := tokenSuffix("abcdefghij", 6); got != "efghij" {
		t.Fatalf("suffix: %q", got)
	}
}
