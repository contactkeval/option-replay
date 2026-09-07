package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
	"github.com/parquet-go/parquet-go"
)

func TestParquetProviderReadsOptionBars(t *testing.T) {
	prov, symbol, expiry, ts := newTestParquetProvider(t)

	from := ts
	to := ts
	bars, err := prov.GetBars(symbol, from, to, 1, TimespanMinute)
	if err != nil {
		t.Fatalf("GetBars: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("GetBars bars=%d want 1", len(bars))
	}
	if bars[0].Close != 1.25 {
		t.Fatalf("GetBars close=%v want 1.25", bars[0].Close)
	}

	price, err := prov.GetOptionPrice("SPY", 580, expiry, "call", ts)
	if err != nil {
		t.Fatalf("GetOptionPrice: %v", err)
	}
	if price != 1.25 {
		t.Fatalf("GetOptionPrice=%v want 1.25", price)
	}

	contracts, err := prov.GetContracts("SPY", 0, expiry, expiry, true)
	if err != nil {
		t.Fatalf("GetContracts: %v", err)
	}
	if len(contracts) != 3 {
		t.Fatalf("GetContracts=%d want 3", len(contracts))
	}

	expiries, err := prov.GetRelevantExpiries("SPY", expiry.AddDate(0, 0, -1), expiry.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("GetRelevantExpiries: %v", err)
	}
	if len(expiries) != 1 || !sameDate(expiries[0], expiry) {
		t.Fatalf("GetRelevantExpiries=%v", expiries)
	}
}

func TestParquetTickers(t *testing.T) {
	got := parquetTickers("I:SPX")
	if len(got) != 2 || got[0] != "SPX" || got[1] != "SPXW" {
		t.Fatalf("parquetTickers(I:SPX)=%v want [SPX SPXW]", got)
	}
}

func TestParquetProviderDelegatesUnderlyingBars(t *testing.T) {
	prov, _, _, _ := newTestParquetProvider(t)
	want := []Bar{{
		Date:  time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC),
		Close: 520,
	}}
	prov.SetSecondary(&stubProvider{bars: want})

	got, err := prov.GetBars("SPY", want[0].Date, want[0].Date, 1, TimespanDay)
	if err != nil {
		t.Fatalf("underlying GetBars: %v", err)
	}
	if len(got) != 1 || got[0].Close != 520 {
		t.Fatalf("underlying GetBars=%v", got)
	}
}

func TestParquetProviderReadsUnderlyingSpotBars(t *testing.T) {
	prov, _, _, ts := newTestParquetProvider(t)
	spotDir := filepath.Join(prov.parquetRoot, "SPX")
	if err := os.MkdirAll(spotDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeSpotParquet(filepath.Join(spotDir, "SPX_spot.parquet"), ts, 5123.25); err != nil {
		t.Fatalf("write spot parquet: %v", err)
	}

	secondary := &stubProvider{bars: []Bar{{Close: 1}}}
	prov.SetSecondary(secondary)

	got, err := prov.GetBars("I:SPX", ts.Add(-time.Minute), ts.Add(time.Minute), 1, TimespanMinute)
	if err != nil {
		t.Fatalf("spot GetBars: %v", err)
	}
	if len(got) != 1 || got[0].Close != 5123.25 {
		t.Fatalf("spot GetBars=%v", got)
	}
	if secondary.getBarsCalls != 0 {
		t.Fatalf("secondary GetBars called %d times, want 0", secondary.getBarsCalls)
	}
}

func TestParquetProviderExpiriesUseOptionRootAlias(t *testing.T) {
	prov, _, expiry, _ := newTestParquetProvider(t)
	metadata, err := db.Open(db.Options{
		Path:    prov.metadataPath,
		Schemas: db.SchemaParquet,
	})
	if err != nil {
		t.Fatalf("open metadata: %v", err)
	}
	if err := metadata.InsertActiveRow(config.ActiveMetadataRow{
		Ticker:     "SPXW",
		ExpiryDate: expiry,
		RowCount:   1,
		Status:     "created",
	}); err != nil {
		t.Fatalf("insert SPXW: %v", err)
	}
	tx, err := metadata.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := metadata.UpdateActiveProcessed(tx, "SPXW", expiry, filepath.Join(prov.parquetRoot, "SPXW.parquet"), 1); err != nil {
		t.Fatalf("update SPXW: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	_ = metadata.Close()

	secondary := &stubProvider{}
	prov.SetSecondary(secondary)

	expiries, err := prov.GetRelevantExpiries("I:SPX", expiry.AddDate(0, 0, -1), expiry.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("GetRelevantExpiries: %v", err)
	}
	if len(expiries) != 1 || !sameDate(expiries[0], expiry) {
		t.Fatalf("GetRelevantExpiries=%v", expiries)
	}
	if secondary.getExpiriesCalls != 0 {
		t.Fatalf("secondary GetRelevantExpiries called %d times, want 0", secondary.getExpiriesCalls)
	}
}

func TestParquetLiveSPXUsesLocalFiles(t *testing.T) {
	cfg := config.Load()
	spot := filepath.Join(cfg.ParquetRoot, "SPX", "SPX_spot.parquet")
	if _, err := os.Stat(spot); err != nil {
		t.Skip("live SPX spot parquet not present")
	}

	secondary := &stubProvider{bars: []Bar{{Close: 1}}}
	prov, err := NewParquetDataProviderFromConfig(secondary)
	if err != nil {
		t.Fatalf("NewParquetDataProviderFromConfig: %v", err)
	}
	t.Cleanup(func() { _ = prov.Close() })

	from := time.Date(2025, 1, 2, 14, 30, 0, 0, time.UTC)
	to := time.Date(2025, 1, 2, 21, 0, 0, 0, time.UTC)
	bars, err := prov.GetBars("I:SPX", from, to, 1, TimespanDay)
	if err != nil {
		t.Fatalf("GetBars: %v", err)
	}
	if len(bars) == 0 {
		t.Fatal("expected SPX daily bars from parquet")
	}
	if secondary.getBarsCalls != 0 {
		t.Fatalf("secondary GetBars called %d times, want 0", secondary.getBarsCalls)
	}

	expiries, err := prov.GetRelevantExpiries(
		"I:SPX",
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("GetRelevantExpiries: %v", err)
	}
	if len(expiries) == 0 {
		t.Fatal("expected SPX/SPXW expiries from parquet metadata")
	}
	if secondary.getExpiriesCalls != 0 {
		t.Fatalf("secondary GetRelevantExpiries called %d times, want 0", secondary.getExpiriesCalls)
	}
}

func TestParquetProviderAggregatesDailyBars(t *testing.T) {
	prov, symbol, _, ts := newTestParquetProvider(t)

	bars, err := prov.GetBars(symbol, ts.Add(-2*time.Hour), ts.Add(2*time.Hour), 1, TimespanDay)
	if err != nil {
		t.Fatalf("daily GetBars: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("daily bars=%d want 1", len(bars))
	}
	if bars[0].Open != 1.20 || bars[0].Close != 1.25 || bars[0].High != 1.30 || bars[0].Low != 1.10 {
		t.Fatalf("daily OHLC=%+v", bars[0])
	}
	if bars[0].Volume != 15 {
		t.Fatalf("daily volume=%v want 15", bars[0].Volume)
	}
}

func newTestParquetProvider(t *testing.T) (*ParquetDataProvider, string, time.Time, time.Time) {
	t.Helper()

	dir := t.TempDir()
	expiry := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC)
	ts := time.Date(2024, 5, 10, 14, 30, 0, 0, time.UTC)
	parquetPath := filepath.Join(dir, "SPY_20240513.parquet")

	if err := writeTestParquet(parquetPath, expiry, ts); err != nil {
		t.Fatalf("write parquet: %v", err)
	}

	metadataPath := filepath.Join(dir, "metadata.db")
	metadata, err := db.Open(db.Options{
		Path:    metadataPath,
		Schemas: db.SchemaParquet,
	})
	if err != nil {
		t.Fatalf("open metadata: %v", err)
	}
	if err := metadata.InsertActiveRow(config.ActiveMetadataRow{
		Ticker:     "SPY",
		ExpiryDate: expiry,
		RowCount:   4,
		Status:     "created",
	}); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}
	tx, err := metadata.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := metadata.UpdateActiveProcessed(tx, "SPY", expiry, parquetPath, 3); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	_ = metadata.Close()

	prov, err := NewParquetDataProvider(dir, metadataPath, nil)
	if err != nil {
		t.Fatalf("NewParquetDataProvider: %v", err)
	}
	t.Cleanup(func() { _ = prov.Close() })

	symbol := formatOptionSymbol("SPY", expiry, "call", 580)
	return prov, symbol, expiry, ts
}

func writeTestParquet(path string, expiry, ts time.Time) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[config.ParquetRow](file)
	defer writer.Close()

	exp := util.EncodeExpiryDate(expiry)
	unix := uint32(ts.Unix())
	earlier := uint32(ts.Add(-time.Minute).Unix())

	groups := [][]config.ParquetRow{
		{
			testRow(exp, 580000, true, earlier, 1.20, 1.22, 1.10, 1.21, 5),
			testRow(exp, 580000, true, unix, 1.21, 1.30, 1.15, 1.25, 10),
		},
		{
			testRow(exp, 580000, false, unix, 1.40, 1.45, 1.35, 1.42, 8),
		},
		{
			testRow(exp, 585000, true, unix, 0.90, 0.95, 0.85, 0.92, 4),
		},
	}

	for _, rows := range groups {
		if _, err := writer.Write(rows); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func writeSpotParquet(path string, ts time.Time, closePx float64) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[config.ParquetRow](file)
	defer writer.Close()

	unix := uint32(ts.Unix())
	if _, err := writer.Write([]config.ParquetRow{
		testRow(0, 0, false, unix, closePx, closePx, closePx, closePx, 0),
	}); err != nil {
		return err
	}
	return writer.Flush()
}

func testRow(
	expiry, strike uint32,
	isCall bool,
	window uint32,
	open, high, low, close float64,
	volume uint32,
) config.ParquetRow {
	return config.ParquetRow{
		ExpiryDate:   expiry,
		Strike:       strike,
		OptionType:   isCall,
		WindowStart:  window,
		Open:         util.PriceToUint32(open),
		High:         util.PriceToUint32(high),
		Low:          util.PriceToUint32(low),
		Close:        util.PriceToUint32(close),
		Volume:       volume,
		Transactions: 1,
	}
}

type stubProvider struct {
	bars             []Bar
	getBarsCalls     int
	getExpiriesCalls int
}

func (*stubProvider) GetName() string        { return "stub" }
func (*stubProvider) GetSecondary() Provider { return nil }
func (*stubProvider) SetSecondary(Provider)  {}
func (*stubProvider) parseExpiryFromSymbol(string) time.Time {
	return time.Time{}
}
func (*stubProvider) OptionSymbolFromParts(string, time.Time, string, float64) string {
	return ""
}
func (*stubProvider) GetStrikeIntervals(string, time.Time) []float64 { return nil }
func (*stubProvider) RoundToNearestStrike(string, time.Time, time.Time, float64) float64 {
	return 0
}
func (*stubProvider) GetATMOptionPrices(string, time.Time, time.Time, float64) (float64, float64, float64, error) {
	return 0, 0, 0, nil
}
func (*stubProvider) GetContracts(string, float64, time.Time, time.Time, bool) ([]OptionContract, error) {
	return nil, ErrNoDataFound
}
func (s *stubProvider) GetBars(string, time.Time, time.Time, int, string) ([]Bar, error) {
	s.getBarsCalls++
	return s.bars, nil
}
func (*stubProvider) GetOptionPrice(string, float64, time.Time, string, time.Time) (float64, error) {
	return 0, ErrNoDataFound
}
func (s *stubProvider) GetRelevantExpiries(string, time.Time, time.Time) ([]time.Time, error) {
	s.getExpiriesCalls++
	return nil, ErrNoDataFound
}
