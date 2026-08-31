package stage2_dxfeeddatadownloader

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func TestToDXFeedSymbol_Spot(t *testing.T) {
	got := ToDXFeedSymbol(db.Contract{
		Underlying: "SPY",
		Type:       db.ContractTypeSpot,
		Strike:     0,
		Expiry:     db.SpotContractExpiry,
	})
	if got != "SPY{=m}" {
		t.Fatalf("got %q", got)
	}
}

func TestGetContractsForRun_AddsSpotsWhenStale(t *testing.T) {
	database := openTestDB(t)
	config.AllowedUnderlyings = map[string]struct{}{
		"SPY": {},
		"QQQ": {},
		"IWM": {},
	}
	t.Cleanup(func() { config.AllowedUnderlyings = map[string]struct{}{} })

	runDate := date(2026, 8, 6)
	for i := int64(1); i <= 25; i++ {
		insertContract(t, database, i, "A", date(2026, 7, 1), int(i), date(2026, 1, 1))
	}
	for i := int64(100); i < 160; i++ {
		insertContract(t, database, i, "Y", date(2026, 12, 1), int(i), date(2026, 1, 1))
	}

	selected, err := GetContractsForRun(database, runDate)
	if err != nil {
		t.Fatal(err)
	}

	var spotCount int
	for _, c := range selected {
		if db.IsSpotContract(c) {
			spotCount++
		}
	}
	if spotCount != 3 {
		t.Fatalf("spotCount=%d want 3 selected=%d", spotCount, len(selected))
	}
}

func TestGetContractsForRun_SkipsSpotsWhenFresh(t *testing.T) {
	database := openTestDB(t)
	config.AllowedUnderlyings = map[string]struct{}{
		"SPY": {},
		"QQQ": {},
	}
	t.Cleanup(func() { config.AllowedUnderlyings = map[string]struct{}{} })

	if err := database.EnsureSpotContracts([]string{"SPY", "QQQ"}); err != nil {
		t.Fatal(err)
	}
	spots, err := database.ListSpotContracts()
	if err != nil {
		t.Fatal(err)
	}
	var spySerial int64
	for _, c := range spots {
		if c.Underlying == "SPY" {
			spySerial = c.SerialNo
		}
	}
	fresh := time.Now().UTC().Add(-time.Hour)
	if _, err := database.InsertCandleStaging(spySerial, config.Candle{
		Time: fresh.UnixMilli(), Open: 1, High: 1, Low: 1, Close: 1,
	}, 1, 1); err != nil {
		t.Fatal(err)
	}

	selected, err := GetContractsForRun(database, date(2026, 8, 6))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selected {
		if db.IsSpotContract(c) {
			t.Fatalf("did not expect spot contract when fresh: %+v", c)
		}
	}
}

func TestSelectContractsForFetch_SkipsNearExpiry(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	insertContract(t, database, 1, "A", date(2026, 7, 1), 10, date(2026, 1, 1))
	insertContract(t, database, 2, "A", date(2026, 8, 20), 5, date(2026, 1, 1))  // near
	insertContract(t, database, 3, "A", date(2026, 10, 1), 20, date(2026, 1, 1)) // far
	insertArchived(t, database, 4, "B", date(2026, 7, 2), 1)

	for i := int64(10); i < 60; i++ {
		insertContract(t, database, i, "X", date(2026, 6, 1), int(i), date(2026, 1, 1))
	}
	for i := int64(100); i < 160; i++ {
		insertContract(t, database, i, "Y", date(2026, 12, 1), int(i), date(2026, 1, 1))
	}

	selected, err := SelectContractsForFetch(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selected {
		if c.SerialNo == 2 {
			t.Fatal("near-expiry contract must not be selected")
		}
		if c.Archived {
			t.Fatal("archived contract must not be selected")
		}
		expiry := truncateDate(c.Expiry)
		if !expiry.Before(runDate) && !expiry.After(runDate.AddDate(0, 1, 0)) {
			t.Fatalf("near-expiry contract %d selected", c.SerialNo)
		}
	}
}

func TestSelectExpiredContracts_GapFromBeforeYesterday(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)
	yesterday := date(2026, 8, 5)

	for i := int64(1); i <= 25; i++ {
		expiry := date(2026, 7, 15)
		bar := int(i)
		if i <= 3 {
			expiry = yesterday
			bar = int(4 - i)
		}
		insertContract(t, database, i, "A", expiry, bar, date(2026, 1, 1))
	}
	insertContract(t, database, 90, "A", date(2026, 6, 1), 5, date(2026, 1, 1))
	insertContract(t, database, 91, "A", date(2026, 7, 1), 1000, date(2026, 1, 1))
	insertContract(t, database, 99, "A", runDate, 9999, date(2026, 1, 1)) // today — not in gap

	selected, err := selectExpiredContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 5 {
		t.Fatalf("expected batch size 5, got %d", len(selected))
	}

	wantYesterday := []int64{3, 2, 1}
	for i, serial := range wantYesterday {
		if selected[i].SerialNo != serial {
			t.Fatalf("pos %d: want yesterday serial %d, got %d", i, serial, selected[i].SerialNo)
		}
	}
	if selected[3].SerialNo != 90 {
		t.Fatalf("expected oldest-expiry gap serial 90, got %d", selected[3].SerialNo)
	}
	if selected[4].SerialNo != 91 {
		t.Fatalf("expected highest-bar gap serial 91, got %d", selected[4].SerialNo)
	}
}

func TestSelectFarExpiryContracts_ThreeCategoriesWithFetchWindow(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)
	staleCutoff := runDate.AddDate(0, 0, -StaleFetchDays)

	insertContract(t, database, 1, "A", date(2026, 12, 1), 50, date(2026, 6, 1))
	insertContract(t, database, 2, "A", date(2026, 12, 1), 40, date(2026, 6, 1))
	insertContract(t, database, 3, "A", date(2026, 12, 1), 100, date(2026, 7, 1))
	insertContract(t, database, 4, "A", date(2026, 12, 1), 10, date(2026, 7, 10))
	insertContract(t, database, 5, "A", date(2026, 12, 1), 20, date(2026, 7, 10))
	insertContract(t, database, 6, "A", date(2026, 12, 1), 30, date(2026, 6, 1))
	insertContract(t, database, 7, "A", date(2026, 12, 1), 500, date(2026, 8, 1)) // recent
	for i := int64(8); i <= 45; i++ {
		insertContract(t, database, i, "A", date(2026, 12, 1), 1, date(2026, 8, 1))
	}

	selected, err := selectFarExpiryContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 3 {
		t.Fatalf("expected batch size 3, got %d", len(selected))
	}
	if selected[0].SerialNo != 1 {
		t.Fatalf("cat1: want serial 1, got %d", selected[0].SerialNo)
	}
	if selected[1].SerialNo != 3 {
		t.Fatalf("cat2: want serial 3, got %d", selected[1].SerialNo)
	}
	if selected[2].SerialNo != 4 {
		t.Fatalf("cat3: want serial 4, got %d", selected[2].SerialNo)
	}

	for _, c := range selected {
		if c.SerialNo == 6 {
			t.Fatal("serial 6 shares cat1 max lastFetch and must not fill cat 2/3")
		}
		if c.SerialNo == 7 {
			t.Fatal("recently fetched must not be selected")
		}
		if !c.LastDownloadedDate.IsZero() && !c.LastDownloadedDate.Before(staleCutoff) {
			t.Fatalf("serial %d lastFetch not before stale cutoff", c.SerialNo)
		}
	}
}

func TestSelectFarExpiryContracts_NeverFetchedIsStale(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	// Insert with NULL lastDownloadedDate by raw SQL (bypass insert helper seed).
	mustExec(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES (1, 'A', '2026-12-01', 'call', 100, '2026-01-01', NULL, 10, 0)
	`)
	for i := int64(2); i <= 30; i++ {
		insertContract(t, database, i, "A", date(2026, 12, 1), int(i), date(2026, 8, 1))
	}

	selected, err := selectFarExpiryContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 stale selection, got %d", len(selected))
	}
	if selected[0].SerialNo != 1 {
		t.Fatal("never-fetched contract should be eligible for far bucket")
	}
}

func TestGroupCountAndCreateBatches(t *testing.T) {
	if got := GroupCount(0); got != 0 {
		t.Fatalf("GroupCount(0)=%d", got)
	}
	if got := GroupCount(99); got != 1 {
		t.Fatalf("GroupCount(99)=%d want 1", got)
	}
	if got := GroupCount(100); got != 2 {
		t.Fatalf("GroupCount(100)=%d want 2", got)
	}
	if got := GroupCount(200); got != 3 {
		t.Fatalf("GroupCount(200)=%d want 3", got)
	}

	contracts := make([]db.Contract, 0, 101)
	for i := int64(1); i <= 101; i++ {
		contracts = append(contracts, db.Contract{
			SerialNo:   i,
			Underlying: "A",
			Expiry:     date(2026, 1, 1),
			BarCount:   int(i),
			Strike:     float64(i),
		})
	}
	SortContractsForGrouping(contracts)
	batches := CreateBatches(contracts)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	for _, b := range batches {
		if len(b.Contracts) > MaxGroupSize {
			t.Fatalf("batch %d has %d contracts (> %d)", b.BatchNo, len(b.Contracts), MaxGroupSize)
		}
	}
}

func TestSortContractsForGrouping(t *testing.T) {
	contracts := []db.Contract{
		{SerialNo: 1, Underlying: "B", Expiry: date(2026, 2, 1), Strike: 100, BarCount: 10},
		{SerialNo: 2, Underlying: "A", Expiry: date(2026, 1, 1), Strike: 90, BarCount: 10},
		{SerialNo: 3, Underlying: "A", Expiry: date(2026, 1, 1), Strike: 80, BarCount: 20},
	}
	SortContractsForGrouping(contracts)
	if contracts[0].SerialNo != 3 {
		t.Fatalf("want highest bar first, got %d", contracts[0].SerialNo)
	}
	if contracts[1].SerialNo != 2 {
		t.Fatalf("want lower strike next, got %d", contracts[1].SerialNo)
	}
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(db.Options{
		Path:    path,
		Schemas: db.SchemaContracts | db.SchemaDownload,
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func insertContract(
	t *testing.T,
	database *db.DB,
	serial int64,
	underlying string,
	expiry time.Time,
	barCount int,
	lastFetch time.Time,
) {
	t.Helper()
	mustExec(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES (?, ?, ?, 'call', ?, ?, ?, ?, 0)
	`,
		serial,
		underlying,
		expiry.Format("2006-01-02"),
		float64(serial),
		"2026-01-01",
		lastFetch.Format("2006-01-02"),
		barCount,
	)
}

func insertArchived(
	t *testing.T,
	database *db.DB,
	serial int64,
	underlying string,
	expiry time.Time,
	barCount int,
) {
	t.Helper()
	mustExec(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, archived
		) VALUES (?, ?, ?, 'call', 1, '2026-01-01', '2026-01-01', ?, 1)
	`, serial, underlying, expiry.Format("2006-01-02"), barCount)
}

func mustExec(t *testing.T, database *db.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec: %v\nquery: %s", err, query)
	}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
