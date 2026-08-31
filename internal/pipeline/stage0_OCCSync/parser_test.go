package stage0_occ

import (
	"strings"
	"testing"
	"time"
)

func TestParseRecord_CallSPY(t *testing.T) {
	line := "CBOEA07/30/26State Street SPDR S&P 500 ETF SPY   Aug13202600820000C P 07/31/26"

	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}

	if rec.Action != ActionAdd {
		t.Fatalf("Action=%q", rec.Action)
	}
	if rec.Exchange != "CBOE" {
		t.Fatalf("Exchange=%q", rec.Exchange)
	}
	if rec.Underlying != "SPY" {
		t.Fatalf("Underlying=%q", rec.Underlying)
	}
	if rec.Type != "call" {
		t.Fatalf("Type=%q", rec.Type)
	}
	if rec.Strike != 820 {
		t.Fatalf("Strike=%v", rec.Strike)
	}
	wantExpiry := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	if !rec.ExpiryDate.Equal(wantExpiry) {
		t.Fatalf("ExpiryDate=%v", rec.ExpiryDate)
	}
	wantActivity := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	if !rec.ActivityDate.Equal(wantActivity) {
		t.Fatalf("ActivityDate=%v", rec.ActivityDate)
	}
}

func TestParseRecord_PutWithBlankCallSlot(t *testing.T) {
	line := "CBOEA08/04/26AMERIPRISE FINANCIAL INC (EURO2AMP  Jun25202700500000  P 08/04/26"

	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}

	if rec.Underlying != "AMP" {
		t.Fatalf("Underlying=%q (want AMP after stripping leading digit)", rec.Underlying)
	}
	if rec.Type != "put" {
		t.Fatalf("Type=%q", rec.Type)
	}
	if rec.Strike != 500 {
		t.Fatalf("Strike=%v", rec.Strike)
	}
}

func TestParseRecord_DeleteShortLine(t *testing.T) {
	line := "CBOED07/30/26                              AVD   Sep18202600010   C P "

	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}

	if rec.Action != ActionDelete {
		t.Fatalf("Action=%q", rec.Action)
	}
	if rec.Underlying != "AVD" {
		t.Fatalf("Underlying=%q", rec.Underlying)
	}
	if rec.Type != "call" {
		t.Fatalf("Type=%q", rec.Type)
	}
	if rec.Strike != 10 {
		t.Fatalf("Strike=%v", rec.Strike)
	}

	// No activity date on delete rows → report date.
	wantActivity := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	if !rec.ActivityDate.Equal(wantActivity) {
		t.Fatalf("ActivityDate=%v", rec.ActivityDate)
	}
}

func TestParseRecord_FractionalStrike(t *testing.T) {
	line := "CBOEA07/30/26Invesco S&P 500 Equal Weight E4RSP  Aug25202600217260C   07/30/26"

	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord: %v", err)
	}

	if rec.Underlying != "RSP" {
		t.Fatalf("Underlying=%q", rec.Underlying)
	}
	if rec.Strike != 217.26 {
		t.Fatalf("Strike=%v", rec.Strike)
	}
	if rec.Type != "call" {
		t.Fatalf("Type=%q", rec.Type)
	}
}

func TestNormalizeUnderlying(t *testing.T) {
	cases := map[string]string{
		"SPY   ": "SPY",
		"2AMP  ": "AMP",
		"4RSP  ": "RSP",
		"AAPL  ": "AAPL",
		"  ":     "",
	}

	for in, want := range cases {
		got := normalizeUnderlying(in)
		if got != want {
			t.Fatalf("normalizeUnderlying(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParseRecord_TooShort(t *testing.T) {
	_, err := ParseRecord(strings.Repeat("x", 20))
	if err == nil {
		t.Fatal("expected error")
	}
}
