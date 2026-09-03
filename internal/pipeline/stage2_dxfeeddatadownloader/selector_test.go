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

func TestSelectExpiredContracts_UnderFetchedThenGap(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	// 25 eligible expired → batchSize 5.
	// Serials 1-3: already fetched after expiry (attempts >= 1).
	// Serials 4-25: under-fetched (attempts 0).
	for i := int64(1); i <= 3; i++ {
		insertContractWithAttempts(t, database, i, "A", date(2026, 8, 5), int(10-i), date(2026, 1, 1), 1)
	}
	for i := int64(4); i <= 25; i++ {
		insertContract(t, database, i, "A", date(2026, 7, 15), int(i), date(2026, 1, 1))
	}

	selected, err := selectExpiredContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 5 {
		t.Fatalf("expected batch size 5, got %d", len(selected))
	}

	// First fill: under-fetched lowest barCount → serials 4..8.
	for i, want := range []int64{4, 5, 6, 7, 8} {
		if selected[i].SerialNo != want {
			t.Fatalf("pos %d: want under-fetched serial %d, got %d", i, want, selected[i].SerialNo)
		}
	}
}

func TestSelectExpiredContracts_GapOldestFetchThenHighestAfterMax(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	// 25 eligible expired, only 2 under-fetched → remaining 3 from gap.
	for i := int64(1); i <= 23; i++ {
		insertContractWithAttempts(t, database, i, "A", date(2026, 7, 15), int(i), date(2026, 7, 1), 1)
	}
	insertContract(t, database, 24, "A", date(2026, 7, 15), 1, date(2026, 1, 1))
	insertContract(t, database, 25, "A", date(2026, 7, 15), 2, date(2026, 1, 1))
	// Oldest lastFetch among attempts>=1: serial 90 on 2026-06-01.
	insertContractWithAttempts(t, database, 90, "A", date(2026, 6, 1), 5, date(2026, 6, 1), 1)
	// Highest bar with expiry < T-1 and lastFetch > 2026-06-01.
	insertContractWithAttempts(t, database, 91, "A", date(2026, 7, 1), 1000, date(2026, 7, 10), 1)

	selected, err := selectExpiredContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 5 {
		t.Fatalf("expected batch size 5, got %d", len(selected))
	}
	if selected[0].SerialNo != 24 || selected[1].SerialNo != 25 {
		t.Fatalf("want under-fetched 24,25 first, got %d,%d", selected[0].SerialNo, selected[1].SerialNo)
	}
	if selected[2].SerialNo != 90 {
		t.Fatalf("expected oldest-lastFetch serial 90, got %d", selected[2].SerialNo)
	}
	if selected[3].SerialNo != 91 {
		t.Fatalf("expected highest-bar after maxFetch serial 91, got %d", selected[3].SerialNo)
	}
}

func TestSelectFarExpiryContracts_NeverDownloadedThenOldestAndHighest(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	// 15 stale far → available=15, batchSize 3.
	// lastFetch must be older than runDate-15d (before 2026-07-22).
	insertContract(t, database, 1, "A", date(2026, 12, 1), 50, date(2026, 6, 1)) // attempts 0
	insertContractWithAttempts(t, database, 2, "A", date(2026, 12, 1), 40, date(2026, 6, 1), 0.002)
	insertContractWithAttempts(t, database, 3, "A", date(2026, 12, 1), 100, date(2026, 7, 1), 0.002)
	insertContractWithAttempts(t, database, 4, "A", date(2026, 12, 1), 10, date(2026, 7, 10), 0.002)
	insertContractWithAttempts(t, database, 5, "A", date(2026, 12, 1), 200, date(2026, 7, 10), 0.002)
	insertContractWithAttempts(t, database, 6, "A", date(2026, 12, 1), 30, date(2026, 6, 2), 0.002)
	for i := int64(7); i <= 15; i++ {
		insertContractWithAttempts(t, database, i, "A", date(2026, 12, 1), 1, date(2026, 7, 1), 0.001)
	}
	// Fresh fetch within 15 days — must not count toward available pool.
	insertContractWithAttempts(t, database, 99, "A", date(2026, 12, 1), 999, date(2026, 8, 1), 0.001)

	selected, err := selectFarExpiryContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 3 {
		t.Fatalf("expected batch size 3, got %d", len(selected))
	}
	if selected[0].SerialNo != 1 {
		t.Fatalf("never-downloaded: want serial 1, got %d", selected[0].SerialNo)
	}
	if selected[1].SerialNo != 2 {
		t.Fatalf("oldest fetch: want serial 2, got %d", selected[1].SerialNo)
	}
	if selected[2].SerialNo != 5 {
		t.Fatalf("highest bar after max: want serial 5, got %d", selected[2].SerialNo)
	}
	for _, c := range selected {
		if c.SerialNo == 99 {
			t.Fatal("fresh lastDownload within 15 days must not be selected")
		}
	}
}

func TestSelectFarExpiryContracts_NeverFetchedFirst(t *testing.T) {
	database := openTestDB(t)
	runDate := date(2026, 8, 6)

	mustExec(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, downloadAttempts, archived
		) VALUES (1, 'A', '2026-12-01', 'call', 100, '2026-01-01', NULL, 10, 0, 0)
	`)
	for i := int64(2); i <= 30; i++ {
		insertContractWithAttempts(t, database, i, "A", date(2026, 12, 1), int(i), date(2026, 7, 1), 0.001)
	}

	selected, err := selectFarExpiryContracts(database, runDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) < 1 || selected[0].SerialNo != 1 {
		t.Fatalf("never-fetched contract should be selected first, got %v", selected)
	}
}

func TestGetContractsForRun_ArchivesHighAttempts(t *testing.T) {
	database := openTestDB(t)
	config.AllowedUnderlyings = map[string]struct{}{"SPY": {}}
	t.Cleanup(func() { config.AllowedUnderlyings = map[string]struct{}{} })

	insertContractWithAttempts(t, database, 1, "A", date(2026, 7, 1), 10, date(2026, 1, 1), 3)
	for i := int64(2); i <= 25; i++ {
		insertContract(t, database, i, "A", date(2026, 7, 1), int(i), date(2026, 1, 1))
	}
	for i := int64(100); i < 160; i++ {
		insertContract(t, database, i, "Y", date(2026, 12, 1), int(i), date(2026, 1, 1))
	}

	selected, err := GetContractsForRun(database, date(2026, 8, 6))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selected {
		if c.SerialNo == 1 {
			t.Fatal("downloadAttempts >= 3 should be archived and not selected")
		}
	}
	var archived int
	if err := database.QueryRow(`SELECT archived FROM contracts WHERE serialNo = 1`).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 1 {
		t.Fatal("expected serial 1 archived")
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
	insertContractWithAttempts(t, database, serial, underlying, expiry, barCount, lastFetch, 0)
}

func insertContractWithAttempts(
	t *testing.T,
	database *db.DB,
	serial int64,
	underlying string,
	expiry time.Time,
	barCount int,
	lastFetch time.Time,
	downloadAttempts float64,
) {
	t.Helper()
	mustExec(t, database, `
		INSERT INTO contracts (
			serialNo, underlying, expiry, type, strike,
			firstSeenDate, lastDownloadedDate, barCount, downloadAttempts, archived
		) VALUES (?, ?, ?, 'call', ?, ?, ?, ?, ?, 0)
	`,
		serial,
		underlying,
		expiry.Format("2006-01-02"),
		float64(serial),
		"2026-01-01",
		lastFetch.Format("2006-01-02"),
		barCount,
		downloadAttempts,
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
