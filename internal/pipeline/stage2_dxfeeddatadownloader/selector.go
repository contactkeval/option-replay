// Package stage2_dxfeeddatadownloader downloads option bar data via DXFeed.
//
// Contract selection (this file) decides which contracts enter each download run.
// Heavy filtering/ranking is done in SQL (see db/selection.go); Go only
// orchestrates multi-step rules and merges the ranked slices:
//
//  1. Archive any contract with downloadAttempts >= 3.
//
//  2. Pools by expiry relative to run date (eligible = not archived,
//     downloadAttempts < 3, not spot):
//     - expired:      expiry < today          (batch size = count/5)
//     - near expiry:  today <= expiry <= today+1 month  (never fetched)
//     - far expiry:   expiry > today+1 month, last fetch older than 15 days
//     (batch size = availableCount/5)
//
//  3. Expired selection:
//     - downloadAttempts < 1
//     - remaining slots split equally:
//     oldest lastDownloadedDate (record max), then highest barCount with
//     expiry < T-1 and lastDownloadedDate > that max
//
//  4. Far selection:
//     - downloadAttempts = 0
//     - remaining slots split equally:
//     oldest lastDownloadedDate (record max), then highest barCount with
//     lastDownloadedDate > that max
//
//  5. Merge, sort in Go, assign download groups of size < MaxGroupSize.
//
// Later slices do not need serialNo exclude lists: attempt thresholds and
// lastDownloadedDate > max naturally partition the pools.
//
// Future-dated contracts only add 0.001 per fetch so they are not archived quickly.
package stage2_dxfeeddatadownloader

import (
	"fmt"
	"sort"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

// MaxGroupSize is the soft upper bound used to size download groups.
// Group count is (selected/MaxGroupSize)+1 so each group stays at most this size.
const MaxGroupSize = 100

// MaxCandleSubscribe is the max symbols per DXLink Candle FEED_SUBSCRIPTION.
const MaxCandleSubscribe = 100

// DownloadWorkers is the number of parallel DXLink sessions.
const DownloadWorkers = 4

// ConnectAttempts is how many times a worker retries opening a DXLink session
// before failing the whole download pool.
const ConnectAttempts = 5

// ChunkAttempts is how many times a worker retries an incomplete/idle chunk.
const ChunkAttempts = 3

// DownloadWaves is how many connection cycles to split a run's batches into.
// Batches are divided as evenly as possible across these waves.
const DownloadWaves = 3

// WaveCooldown is how long to wait after finishing a wave before reconnecting
// for the next set of batches.
const WaveCooldown = 30 * time.Second

// ArchiveDownloadAttempts archives contracts once downloadAttempts reaches this.
const ArchiveDownloadAttempts = 3

// SpotFreshnessProbe is the underlying used to decide whether spot minutes
// need a full refresh for all allowed underlyings.
const SpotFreshnessProbe = "SPY"

// SpotFreshnessMaxAge triggers a full allowed-underlying spot download when the
// newest SPY spot bar in candle_staging is older than this duration.
const SpotFreshnessMaxAge = 30 * 24 * time.Hour

// SelectContractsForFetch builds the combined fetch set using SQL pool counts
// and ranked LIMIT slices. Near-expiry contracts are never selected.
func SelectContractsForFetch(
	database *db.DB,
	runDate time.Time,
) ([]db.Contract, error) {
	runDate = truncateDate(runDate)

	expired, err := selectExpiredContracts(database, runDate)
	if err != nil {
		return nil, err
	}

	far, err := selectFarExpiryContracts(database, runDate)
	if err != nil {
		return nil, err
	}

	return append(expired, far...), nil
}

// selectExpiredContracts builds the expired fetch batch.
//
// Available pool: expiry < runDate, archived = 0, downloadAttempts < 3.
// Batch size is poolSize/5. Selection:
//  1. downloadAttempts < 1
//  2. remaining slots split equally:
//     a. oldest lastDownloadedDate among attempts >= 1 (record max date)
//     b. highest barCount with expiry < T-1 and lastDownloadedDate > that max
func selectExpiredContracts(
	database *db.DB,
	runDate time.Time,
) ([]db.Contract, error) {
	expiredTotal, err := database.CountExpiredContracts(runDate)
	if err != nil {
		return nil, fmt.Errorf("count expired: %w", err)
	}

	batchSize := expiredTotal / 5
	if batchSize <= 0 {
		logger.Infof(
			"expired selection: total=%d batchSize=0 (skipped)",
			expiredTotal,
		)
		return nil, nil
	}

	tMinus1 := runDate.AddDate(0, 0, -1)

	underFetched, err := database.SelectExpiredUnderFetched(runDate, batchSize)
	if err != nil {
		return nil, err
	}

	selected := append([]db.Contract(nil), underFetched...)
	remaining := batchSize - len(selected)
	if remaining <= 0 {
		logger.Infof(
			"expired selection: total=%d batchSize=%d selectedUnderFetched=%d gap=none minBarCount=%d",
			expiredTotal,
			batchSize,
			len(underFetched),
			minBarCount(selected),
		)
		return selected, nil
	}

	half := remaining / 2
	other := remaining - half

	oldestFetch, err := database.SelectExpiredOldestLastFetch(runDate, half)
	if err != nil {
		return nil, err
	}
	selected = append(selected, oldestFetch...)
	maxLastFetch := maxLastFetchDate(oldestFetch)

	var highest []db.Contract
	if !maxLastFetch.IsZero() {
		highest, err = database.SelectExpiredHighestBarAfterFetch(
			runDate,
			tMinus1,
			maxLastFetch,
			other,
		)
		if err != nil {
			return nil, err
		}
		selected = append(selected, highest...)
	}

	logger.Infof(
		"expired selection: total=%d batchSize=%d selectedUnderFetched=%d maxLastFetch=%s selectedByOldestFetch=%d selectedByHighestBar=%d minBarCount=%d",
		expiredTotal,
		batchSize,
		len(underFetched),
		formatDateOrDash(maxLastFetch),
		len(oldestFetch),
		len(highest),
		minBarCount(selected),
	)

	return selected, nil
}

// selectFarExpiryContracts builds the far-expiry fetch batch.
//
// Available pool: expiry > runDate+1 month, archived = 0, downloadAttempts < 3,
// and lastDownloadedDate missing or older than FarStaleFetchDays.
// Batch size is poolSize/5. Selection:
//  1. downloadAttempts = 0
//  2. remaining slots split equally:
//     a. oldest lastDownloadedDate among attempts > 0 (record max date)
//     b. highest barCount with lastDownloadedDate > that max
func selectFarExpiryContracts(
	database *db.DB,
	runDate time.Time,
) ([]db.Contract, error) {
	farTotal, err := database.CountFarExpiryAvailableContracts(runDate)
	if err != nil {
		return nil, fmt.Errorf("count far available: %w", err)
	}

	batchSize := farTotal / 5
	if batchSize <= 0 {
		logger.Infof(
			"far selection: available=%d batchSize=0 (skipped)",
			farTotal,
		)
		return nil, nil
	}

	neverDownloaded, err := database.SelectFarNeverDownloaded(runDate, batchSize)
	if err != nil {
		return nil, err
	}

	selected := append([]db.Contract(nil), neverDownloaded...)
	remaining := batchSize - len(selected)
	if remaining <= 0 {
		logger.Infof(
			"far selection: available=%d batchSize=%d selectedNeverDownloaded=%d gap=none minBarCount=%d",
			farTotal,
			batchSize,
			len(neverDownloaded),
			minBarCount(selected),
		)
		return selected, nil
	}

	half := remaining / 2
	other := remaining - half

	oldestFetch, err := database.SelectFarOldestLastFetch(runDate, half)
	if err != nil {
		return nil, err
	}
	selected = append(selected, oldestFetch...)
	maxLastFetch := maxLastFetchDate(oldestFetch)

	var highest []db.Contract
	if !maxLastFetch.IsZero() {
		highest, err = database.SelectFarHighestBarAfterFetch(
			runDate,
			maxLastFetch,
			other,
		)
		if err != nil {
			return nil, err
		}
		selected = append(selected, highest...)
	}

	logger.Infof(
		"far selection: available=%d batchSize=%d selectedNeverDownloaded=%d maxLastFetch=%s selectedByOldestFetch=%d selectedByHighestBar=%d minBarCount=%d",
		farTotal,
		batchSize,
		len(neverDownloaded),
		formatDateOrDash(maxLastFetch),
		len(oldestFetch),
		len(highest),
		minBarCount(selected),
	)

	return selected, nil
}

// SortContractsForGrouping orders selected contracts for group assignment:
// barCount DESC, strike ASC, expiry ASC, underlying ASC.
func SortContractsForGrouping(contracts []db.Contract) {
	sort.SliceStable(contracts, func(i, j int) bool {
		a, b := contracts[i], contracts[j]

		if a.BarCount != b.BarCount {
			return a.BarCount > b.BarCount
		}
		if a.Strike != b.Strike {
			return a.Strike < b.Strike
		}
		ae, be := truncateDate(a.Expiry), truncateDate(b.Expiry)
		if !ae.Equal(be) {
			return ae.Before(be)
		}
		if a.Underlying != b.Underlying {
			return a.Underlying < b.Underlying
		}
		return a.SerialNo < b.SerialNo
	})
}

// GroupCount returns (selected/MaxGroupSize)+1 so each group stays <= MaxGroupSize.
func GroupCount(selectedCount int) int {
	if selectedCount <= 0 {
		return 0
	}
	return (selectedCount / MaxGroupSize) + 1
}

// GetContractsForRun selects contracts for the given run date using SQL-backed
// pool queries and ranked slices. Contracts with downloadAttempts >= 3 are
// archived first. When SPY spot bars in candle_staging are missing or older
// than SpotFreshnessMaxAge, all allowed underlyings are added as spot contracts
// so dxFeed downloads them in the same run batches.
func GetContractsForRun(database *db.DB, runDate time.Time) ([]db.Contract, error) {
	archived, err := database.ArchiveContractsByDownloadAttempts(ArchiveDownloadAttempts)
	if err != nil {
		return nil, fmt.Errorf("archive by downloadAttempts: %w", err)
	}
	if archived > 0 {
		logger.Infof(
			"archived %d contracts with downloadAttempts >= %g",
			archived,
			float64(ArchiveDownloadAttempts),
		)
	}

	selected, err := SelectContractsForFetch(database, runDate)
	if err != nil {
		return nil, err
	}

	selected, err = appendSpotContractsIfStale(database, selected)
	if err != nil {
		return nil, err
	}

	logger.Infof("contract pools: selected=%d", len(selected))
	return selected, nil
}

func appendSpotContractsIfStale(
	database *db.DB,
	selected []db.Contract,
) ([]db.Contract, error) {
	symbols := make([]string, 0, len(config.AllowedUnderlyings))
	for symbol := range config.AllowedUnderlyings {
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		logger.Warnf("no allowed underlyings loaded; skipping spot freshness check")
		return selected, nil
	}

	if err := database.EnsureSpotContracts(symbols); err != nil {
		return nil, fmt.Errorf("ensure spot contracts: %w", err)
	}

	stale, last, err := database.SpotBarsStale(SpotFreshnessProbe, SpotFreshnessMaxAge)
	if err != nil {
		return nil, fmt.Errorf("spot freshness check: %w", err)
	}
	if !stale {
		logger.Infof(
			"spot minutes fresh: %s last=%s (within 1 month)",
			SpotFreshnessProbe,
			last.Format(time.RFC3339),
		)
		return selected, nil
	}

	if last.IsZero() {
		logger.Infof(
			"spot minutes missing for %s; adding %d allowed underlyings to run",
			SpotFreshnessProbe,
			len(symbols),
		)
	} else {
		logger.Infof(
			"spot minutes stale: %s last=%s; adding %d allowed underlyings to run",
			SpotFreshnessProbe,
			last.Format(time.RFC3339),
			len(symbols),
		)
	}

	spots, err := database.ListSpotContracts()
	if err != nil {
		return nil, fmt.Errorf("list spot contracts: %w", err)
	}
	if len(spots) == 0 {
		return selected, fmt.Errorf("no spot contracts available after ensure")
	}

	return append(selected, spots...), nil
}

// truncateDate returns the UTC calendar date of t with time set to midnight.
func truncateDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// maxLastFetchDate returns the latest lastFetch among contracts.
// Never-fetched contracts contribute a zero time.
func maxLastFetchDate(contracts []db.Contract) time.Time {
	var max time.Time
	for _, c := range contracts {
		if c.LastDownloadedDate.IsZero() {
			continue
		}
		lf := truncateDate(c.LastDownloadedDate)
		if lf.After(max) {
			max = lf
		}
	}
	return max
}

// minBarCount returns the smallest BarCount among contracts, or 0 if empty.
func minBarCount(contracts []db.Contract) int {
	if len(contracts) == 0 {
		return 0
	}
	min := contracts[0].BarCount
	for _, c := range contracts[1:] {
		if c.BarCount < min {
			min = c.BarCount
		}
	}
	return min
}

// formatDateOrDash formats t as yyyy-mm-dd, or "-" when t is zero.
func formatDateOrDash(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}
