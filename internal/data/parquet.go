package data

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
	"github.com/contactkeval/option-replay/internal/pipeline/util"
)

// ParquetDataProvider implements Provider using local parquet option bars.
type ParquetDataProvider struct {
	parquetRoot  string
	metadataPath string
	metadata     *db.DB
	secondary    Provider
}

func NewParquetDataProvider(
	parquetRoot, metadataPath string,
	secondary Provider,
) (*ParquetDataProvider, error) {
	if _, err := os.Stat(metadataPath); err != nil {
		return nil, fmt.Errorf("metadata db: %w", err)
	}

	metadata, err := db.Open(db.Options{
		Path:    metadataPath,
		Schemas: db.SchemaParquet,
	})
	if err != nil {
		return nil, fmt.Errorf("open metadata db: %w", err)
	}

	logger.Infof("initializing parquet data provider root=%s metadata=%s", parquetRoot, metadataPath)

	return &ParquetDataProvider{
		parquetRoot:  parquetRoot,
		metadataPath: metadataPath,
		metadata:     metadata,
		secondary:    secondary,
	}, nil
}

func NewParquetDataProviderFromConfig(secondary Provider) (*ParquetDataProvider, error) {
	cfg := config.Load()
	return NewParquetDataProvider(
		cfg.ParquetRoot,
		filepath.Join(cfg.MetadataRoot, "metadata.db"),
		secondary,
	)
}

func (*ParquetDataProvider) GetName() string {
	return "parquet"
}

func (p *ParquetDataProvider) GetSecondary() Provider {
	return p.secondary
}

func (p *ParquetDataProvider) SetSecondary(secondary Provider) {
	p.secondary = secondary
}

func (p *ParquetDataProvider) Close() error {
	if p.metadata == nil {
		return nil
	}
	return p.metadata.Close()
}

func (p *ParquetDataProvider) GetATMOptionPrices(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) (strike, callPrice, putPrice float64, err error) {
	expired := expiryDate.Before(time.Now().UTC().Truncate(24 * time.Hour))
	contracts, err := p.GetContracts(underlying, 0, expiryDate, expiryDate, expired)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(contracts) == 0 {
		return 0, 0, 0, fmt.Errorf("no contracts found")
	}

	minDiff := math.MaxFloat64
	for _, c := range contracts {
		diff := math.Abs(c.Strike - asOfPrice)
		if diff < minDiff {
			minDiff = diff
			strike = c.Strike
		}
	}

	callPrice, err = p.GetOptionPrice(underlying, strike, expiryDate, "call", openDate)
	if err != nil {
		return 0, 0, 0, err
	}
	putPrice, err = p.GetOptionPrice(underlying, strike, expiryDate, "put", openDate)
	if err != nil {
		return 0, 0, 0, err
	}
	return strike, callPrice, putPrice, nil
}

func (p *ParquetDataProvider) GetContracts(
	underlying string,
	strike float64,
	fromDate, toDate time.Time,
	expired bool,
) ([]OptionContract, error) {
	ticker := normalizeUnderlying(underlying)
	filter := parquetFilter{
		fromExpiry: fromDate,
		toExpiry:   toDate,
	}
	if !fromDate.IsZero() && fromDate.Equal(toDate) {
		exp := util.EncodeExpiryDate(fromDate)
		filter.expiry = &exp
	}
	if strike > 0 {
		s := util.StrikeToUint32(strike)
		filter.strike = &s
	}

	seen := make(map[string]OptionContract)
	err := p.scanRowsAny(parquetTickers(underlying), filter, func(row config.ParquetRow) error {
		expiry := util.DecodeExpiryDate(row.ExpiryDate)
		if expiry.IsZero() || isSpotMetadataExpiry(expiry) {
			return nil
		}
		if skipExpired(expiry, expired) {
			return nil
		}
		if !fromDate.IsZero() && expiry.Before(dateOnly(fromDate)) && (toDate.IsZero() || !fromDate.Equal(toDate)) {
			return nil
		}
		if !toDate.IsZero() && expiry.After(dateOnly(toDate)) && (fromDate.IsZero() || !fromDate.Equal(toDate)) {
			return nil
		}

		contract := OptionContract{
			ExpiryDate: expiry,
			Strike:     util.StrikeFromUint32(row.Strike),
			Type:       optionTypeToString(row.OptionType),
		}
		key := fmt.Sprintf("%s|%.8g|%s", expiry.Format("2006-01-02"), contract.Strike, contract.Type)
		seen[key] = contract
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(seen) == 0 {
		if p.secondary != nil {
			return p.secondary.GetContracts(underlying, strike, fromDate, toDate, expired)
		}
		return nil, fmt.Errorf("%w: %s", ErrNoDataFound, ticker)
	}

	out := make([]OptionContract, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ExpiryDate.Equal(out[j].ExpiryDate) {
			return out[i].ExpiryDate.Before(out[j].ExpiryDate)
		}
		if out[i].Strike != out[j].Strike {
			return out[i].Strike < out[j].Strike
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func (p *ParquetDataProvider) GetBars(
	symbol string,
	fromDate, toDate time.Time,
	multiplier int,
	timespan string,
) ([]Bar, error) {
	var (
		out []Bar
		err error
	)
	if isOptionSymbol(symbol) {
		out, err = p.getOptionBars(symbol, fromDate, toDate)
	} else {
		out, err = p.getUnderlyingBars(symbol, fromDate, toDate)
	}
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return p.fallbackGetBars(symbol, fromDate, toDate, multiplier, timespan)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	if strings.EqualFold(timespan, TimespanDay) {
		out = aggregateDailyBars(out)
	}
	return out, nil
}

func (p *ParquetDataProvider) getOptionBars(symbol string, fromDate, toDate time.Time) ([]Bar, error) {
	parsed, err := parseOptionSymbol(symbol)
	if err != nil {
		return nil, fmt.Errorf("parse option symbol %s: %w", symbol, err)
	}

	expiry := util.EncodeExpiryDate(parsed.ExpiryDate)
	filter := parquetFilter{
		expiry:     &expiry,
		strike:     &parsed.Strike,
		optionType: &parsed.OptionType,
		fromTime:   fromDate,
		toTime:     toDate,
	}

	var out []Bar
	err = p.scanRowsAny(parquetTickers(parsed.Underlying), filter, func(row config.ParquetRow) error {
		appendParquetBar(&out, row, fromDate, toDate)
		return nil
	})
	return out, err
}

func (p *ParquetDataProvider) getUnderlyingBars(symbol string, fromDate, toDate time.Time) ([]Bar, error) {
	filter := parquetFilter{
		fromTime: fromDate,
		toTime:   toDate,
	}

	var out []Bar
	for _, ticker := range parquetTickers(symbol) {
		sources, err := p.spotSources(ticker)
		if err != nil {
			return nil, err
		}
		for _, src := range sources {
			if err := p.scanParquetFile(src, filter, func(row config.ParquetRow) error {
				appendParquetBar(&out, row, fromDate, toDate)
				return nil
			}); err != nil {
				return nil, err
			}
		}
		if len(out) > 0 {
			logger.Debugf("parquet spot bars ticker=%s count=%d", ticker, len(out))
			return out, nil
		}
	}
	return out, nil
}

func (p *ParquetDataProvider) fallbackGetBars(
	symbol string,
	fromDate, toDate time.Time,
	multiplier int,
	timespan string,
) ([]Bar, error) {
	if p.secondary != nil {
		return p.secondary.GetBars(symbol, fromDate, toDate, multiplier, timespan)
	}
	return nil, fmt.Errorf("%w: %s", ErrNoDataFound, symbol)
}

func appendParquetBar(out *[]Bar, row config.ParquetRow, fromDate, toDate time.Time) {
	ts := time.Unix(int64(row.WindowStart), 0).UTC()
	if !fromDate.IsZero() && ts.Before(fromDate) {
		return
	}
	if !toDate.IsZero() && ts.After(toDate) {
		return
	}
	*out = append(*out, Bar{
		Date:   ts,
		Open:   util.PriceFromUint32(row.Open),
		High:   util.PriceFromUint32(row.High),
		Low:    util.PriceFromUint32(row.Low),
		Close:  util.PriceFromUint32(row.Close),
		Volume: float64(row.Volume),
		Count:  row.Transactions,
	})
}

func (p *ParquetDataProvider) GetOptionPrice(
	underlying string,
	strike float64,
	expiryDate time.Time,
	optionType string,
	openDate time.Time,
) (float64, error) {
	symbol := p.OptionSymbolFromParts(underlying, expiryDate, optionType, strike)

	bars, err := p.GetBars(symbol, openDate.Add(-5*time.Minute), openDate, MultiplierOne, TimespanMinute)
	if err != nil && !errors.Is(err, ErrNoDataFound) {
		return 0, fmt.Errorf("error while search back for option price: %w", err)
	}
	if len(bars) != 0 {
		return bars[len(bars)-1].Close, nil
	}

	bars, err = p.GetBars(symbol, openDate, openDate.Add(5*time.Minute), MultiplierOne, TimespanMinute)
	if err != nil {
		if errors.Is(err, ErrNoDataFound) {
			return 0, err
		}
		return 0, fmt.Errorf("error while search forward for option price: %w", err)
	}
	if len(bars) != 0 {
		return bars[0].Open, nil
	}

	return 0, fmt.Errorf("no parquet price for %s at %s", symbol, openDate.Format("2006-01-02 15:04"))
}

func (p *ParquetDataProvider) GetRelevantExpiries(
	underlying string,
	fromDate, toDate time.Time,
) ([]time.Time, error) {
	seen := make(map[string]time.Time)
	for _, ticker := range parquetTickers(underlying) {
		expiries, err := p.metadata.ListTickerExpiries(ticker, fromDate, toDate)
		if err != nil {
			return nil, err
		}
		for _, expiry := range expiries {
			if isSpotMetadataExpiry(expiry) {
				continue
			}
			key := expiry.UTC().Format("2006-01-02")
			seen[key] = expiry
		}
	}

	if len(seen) == 0 {
		if p.secondary != nil {
			return p.secondary.GetRelevantExpiries(underlying, fromDate, toDate)
		}
		return nil, fmt.Errorf("%w: %s", ErrNoDataFound, normalizeUnderlying(underlying))
	}

	out := make([]time.Time, 0, len(seen))
	for _, expiry := range seen {
		out = append(out, expiry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})
	return out, nil
}

func (p *ParquetDataProvider) RoundToNearestStrike(
	underlying string,
	expiryDate, openDate time.Time,
	asOfPrice float64,
) float64 {
	intervals := p.GetStrikeIntervals(underlying, expiryDate)
	if len(intervals) == 0 || intervals[0] == 0 {
		if p.secondary != nil {
			return p.secondary.RoundToNearestStrike(underlying, expiryDate, openDate, asOfPrice)
		}
		return asOfPrice
	}
	return math.Round(asOfPrice/intervals[0]) * intervals[0]
}

func (p *ParquetDataProvider) OptionSymbolFromParts(
	underlying string,
	expiryDate time.Time,
	optionType string,
	strike float64,
) string {
	return formatOptionSymbol(underlying, expiryDate, optionType, strike)
}

func (p *ParquetDataProvider) parseExpiryFromSymbol(symbol string) time.Time {
	parsed, err := parseOptionSymbol(symbol)
	if err != nil {
		logger.Errorf("invalid option symbol length: %s", symbol)
		return time.Time{}
	}
	return parsed.ExpiryDate
}

func (p *ParquetDataProvider) GetStrikeIntervals(underlying string, expiryDate time.Time) []float64 {
	expired := expiryDate.Before(time.Now().UTC().Truncate(24 * time.Hour))
	contractList, err := p.GetContracts(underlying, 0, expiryDate, expiryDate, expired)
	if err != nil || len(contractList) == 0 {
		if p.secondary != nil {
			return p.secondary.GetStrikeIntervals(underlying, expiryDate)
		}
		return nil
	}

	strikeList := make([]float64, 0, len(contractList))
	for _, c := range contractList {
		strikeList = append(strikeList, c.Strike)
	}
	sort.Float64s(strikeList)

	uniqueStrikes := make([]float64, 0, len(strikeList))
	for i, s := range strikeList {
		if i == 0 || s != strikeList[i-1] {
			uniqueStrikes = append(uniqueStrikes, s)
		}
	}

	intervalMap := make(map[float64]struct{})
	for i := 0; i < len(uniqueStrikes)-1; i++ {
		diff := uniqueStrikes[i+1] - uniqueStrikes[i]
		if diff > 0 {
			intervalMap[diff] = struct{}{}
		}
	}

	intervalList := make([]float64, 0, len(intervalMap))
	for interval := range intervalMap {
		intervalList = append(intervalList, interval)
	}
	sort.Float64s(intervalList)
	return intervalList
}

func (p *ParquetDataProvider) scanRowsAny(
	tickers []string,
	filter parquetFilter,
	fn func(config.ParquetRow) error,
) error {
	for _, ticker := range tickers {
		if err := p.scanRows(ticker, filter, fn); err != nil {
			return err
		}
	}
	return nil
}

func (p *ParquetDataProvider) spotSources(ticker string) ([]db.ParquetSource, error) {
	sources := make([]db.ParquetSource, 0, 2)
	seen := make(map[string]struct{})

	add := func(src db.ParquetSource) {
		if src.ParquetPath == "" {
			return
		}
		if _, ok := seen[src.ParquetPath]; ok {
			return
		}
		seen[src.ParquetPath] = struct{}{}
		sources = append(sources, src)
	}

	path := filepath.Join(p.parquetRoot, ticker, ticker+"_spot.parquet")
	if _, err := os.Stat(path); err == nil {
		add(db.ParquetSource{
			Ticker:      ticker,
			ParquetPath: path,
		})
	}

	indexed, err := p.metadata.LookupParquetSources(ticker, db.SpotContractExpiry, db.SpotContractExpiry)
	if err != nil {
		return nil, err
	}
	for _, src := range indexed {
		add(src)
	}
	return sources, nil
}

func isSpotMetadataExpiry(expiry time.Time) bool {
	if expiry.IsZero() {
		return true
	}
	return expiry.UTC().Year() >= db.SpotContractExpiry.Year()
}

func skipExpired(expiry time.Time, expired bool) bool {
	today := time.Now().UTC()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	isExpired := expiry.Before(today)
	return expired != isExpired
}

func dateOnly(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func aggregateDailyBars(bars []Bar) []Bar {
	if len(bars) == 0 {
		return bars
	}

	type dayAgg struct {
		bar   Bar
		count uint32
	}
	byDay := make(map[string]*dayAgg)
	order := make([]string, 0)

	for _, b := range bars {
		key := b.Date.UTC().Format("2006-01-02")
		agg, ok := byDay[key]
		if !ok {
			dayStart := time.Date(b.Date.Year(), b.Date.Month(), b.Date.Day(), 0, 0, 0, 0, time.UTC)
			byDay[key] = &dayAgg{
				bar: Bar{
					Date:   dayStart,
					Open:   b.Open,
					High:   b.High,
					Low:    b.Low,
					Close:  b.Close,
					Volume: b.Volume,
					Count:  b.Count,
				},
			}
			order = append(order, key)
			continue
		}
		if b.High > agg.bar.High {
			agg.bar.High = b.High
		}
		if b.Low < agg.bar.Low {
			agg.bar.Low = b.Low
		}
		agg.bar.Close = b.Close
		agg.bar.Volume += b.Volume
		agg.bar.Count += b.Count
	}

	out := make([]Bar, 0, len(order))
	for _, key := range order {
		out = append(out, byDay[key].bar)
	}
	return out
}
