// Package stage2_dxfeeddatadownloader downloads option bar data via DXFeed.
//
// Contract selection (this file) decides which contracts enter each download run.
// Heavy filtering/ranking is done in SQL (see db/selection.go); Go only
// orchestrates multi-step rules and merges the ranked slices:
//
//  1. Pools by expiry relative to run date (SQL counts + WHERE clauses):
//     - expired:      expiry < today
//     - near expiry:  today <= expiry <= today+1 month  (never fetched)
//     - far expiry:   expiry > today+1 month
//
//  2. Expired batch (size = expiredCount/5):
//     - SQL: yesterday expiries ORDER BY barCount ASC LIMIT n
//     - Gap from expiry < yesterday:
//     oldest-expiry slice, then highest-bar slice in the date band
//     (maxOldestExpiry+1 … yesterday-1)
//
//  3. Far batch (size = farCount/15) from stale lastFetch:
//     - SQL: oldest lastFetch slice; record max lastFetch
//     - SQL: highest / least bar slices with lastFetch after that max
//
//  4. Merge, sort in Go, assign download groups of size < MaxGroupSize.
//
// Expired contracts are archived after DownloadAttempts reaches 3 (see db.RecordContractFetch).
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
const WaveCooldown = 10 * time.Minute

// StaleFetchDays is how old lastFetchDate must be for far-expiry eligibility.
const StaleFetchDays = 15

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

// selectExpiredContracts builds the expired fetch batch via SQL ranked slices.
//
// Batch size is expiredCount/5. Selection order:
//  1. Contracts that expired yesterday, ordered by barCount ascending.
//  2. If slots remain, split the remainder in half:
//     a. oldest expiry (expiry < yesterday), barCount descending
//     b. highest barCount in the date band after (a)'s max expiry through
//     yesterday-1 (inclusive) — date-band partitioning, no serial excludes
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
			"expired selection: total=%d previousDay=0 batchSize=0 (skipped)",
			expiredTotal,
		)
		return nil, nil
	}

	yesterday := runDate.AddDate(0, 0, -1)

	previousDayTotal, err := database.CountExpiredOnDate(yesterday)
	if err != nil {
		return nil, fmt.Errorf("count previous day expired: %w", err)
	}

	previousDay, err := database.SelectExpiredPreviousDay(yesterday, batchSize)
	if err != nil {
		return nil, err
	}

	selected := append([]db.Contract(nil), previousDay...)

	remaining := batchSize - len(selected)
	if remaining <= 0 {
		logger.Infof(
			"expired selection: total=%d previousDay=%d selectedPreviousDay=%d batchSize=%d gap=none minBarCount=%d",
			expiredTotal,
			previousDayTotal,
			len(previousDay),
			batchSize,
			minBarCount(selected),
		)
		return selected, nil
	}

	half := remaining / 2
	other := remaining - half

	oldest, err := database.SelectExpiredOldestExpiry(yesterday, half)
	if err != nil {
		return nil, err
	}
	selected = append(selected, oldest...)
	maxExpiry := maxExpiryDate(oldest)

	var highest []db.Contract
	if !maxExpiry.IsZero() {
		// Date band: day after oldest-slice max … day before yesterday.
		bandFrom := maxExpiry.AddDate(0, 0, 1)
		bandTo := yesterday.AddDate(0, 0, -1)
		highest, err = database.SelectExpiredHighestBar(bandFrom, bandTo, other)
		if err != nil {
			return nil, err
		}
		selected = append(selected, highest...)
	}

	logger.Infof(
		"expired selection: total=%d previousDay=%d selectedPreviousDay=%d batchSize=%d gapMaxExpiry=%s selectedByOldestExpiry=%d selectedByHighestBar=%d minBarCount=%d",
		expiredTotal,
		previousDayTotal,
		len(previousDay),
		batchSize,
		formatDateOrDash(maxExpiry),
		len(oldest),
		len(highest),
		minBarCount(selected),
	)

	return selected, nil
}

// selectFarExpiryContracts builds the far-expiry fetch batch via SQL ranked slices.
//
// Batch size is farCount/15. Only stale lastFetch rows are eligible. The batch is
// split into three parts (remainder distributed to earlier parts):
//  1. lastFetch ascending, barCount descending; record max lastFetch among picks
//  2. highest barCount where lastFetch is after that max and still stale
//  3. least barCount (lastFetch ascending) from the same remaining window
func selectFarExpiryContracts(
	database *db.DB,
	runDate time.Time,
) ([]db.Contract, error) {
	farTotal, err := database.CountFarExpiryContracts(runDate)
	if err != nil {
		return nil, fmt.Errorf("count far: %w", err)
	}

	batchSize := farTotal / 15
	if batchSize <= 0 {
		logger.Infof(
			"far selection: far=%d batchSize=0 (skipped)",
			farTotal,
		)
		return nil, nil
	}

	staleBefore := runDate.AddDate(0, 0, -StaleFetchDays)

	part := batchSize / 3
	rem := batchSize % 3
	parts := [3]int{part, part, part}
	for i := 0; i < rem; i++ {
		parts[i]++
	}

	cat1, err := database.SelectFarOldestFetch(runDate, staleBefore, parts[0])
	if err != nil {
		return nil, err
	}

	maxLastFetch := maxLastFetchDate(cat1)
	exclude := serialNos(cat1)

	var cat2, cat3 []db.Contract
	if !maxLastFetch.IsZero() {
		cat2, err = database.SelectFarHighestBarAfterFetch(
			runDate,
			staleBefore,
			maxLastFetch,
			exclude,
			parts[1],
		)
		if err != nil {
			return nil, err
		}
		exclude = append(exclude, serialNos(cat2)...)

		cat3, err = database.SelectFarLeastBarAfterFetch(
			runDate,
			staleBefore,
			maxLastFetch,
			exclude,
			parts[2],
		)
		if err != nil {
			return nil, err
		}
	}

	selected := append(append(cat1, cat2...), cat3...)

	logger.Infof(
		"far selection: far=%d batchSize=%d selectedByOldestFetch=%d maxLastFetch=%s selectedByHighestBar=%d selectedByLeastBar=%d minBarCount=%d",
		farTotal,
		batchSize,
		len(cat1),
		formatDateOrDash(maxLastFetch),
		len(cat2),
		len(cat3),
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
// pool queries and ranked slices. When SPY spot bars in candle_staging are
// missing or older than SpotFreshnessMaxAge, all allowed underlyings are added
// as spot contracts so dxFeed downloads them in the same run batches.
func GetContractsForRun(database *db.DB, runDate time.Time) ([]db.Contract, error) {
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

// serialNos returns SerialNo values for exclude NOT IN clauses.
func serialNos(contracts []db.Contract) []int64 {
	out := make([]int64, len(contracts))
	for i, c := range contracts {
		out[i] = c.SerialNo
	}
	return out
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

// maxExpiryDate returns the latest expiry among contracts.
func maxExpiryDate(contracts []db.Contract) time.Time {
	var max time.Time
	for _, c := range contracts {
		exp := truncateDate(c.Expiry)
		if exp.After(max) {
			max = exp
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
