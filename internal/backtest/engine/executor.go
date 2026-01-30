package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	sch "github.com/contactkeval/option-replay/internal/backtest/scheduler"
	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pricing"
)

/*
Engine orchestrates the backtest execution.
It schedules trades, opens strategies, and simulates exits.
*/
type Engine struct {
	cfg  *Config
	prov data.Provider
}

// Config defines runtime parameters for a backtest.
type Config struct {
	Underlying string          `json:"underlying"`
	Entry      sch.EntryRule   `json:"entry"`
	Strategy   st.StrategySpec `json:"strategy"`
	Exit       ExitSpec        `json:"exit"`
	ReportDir  string          `json:"report_dir,omitempty"`
	MaxTrades  int             `json:"max_trades,omitempty"`
	Seed       int64           `json:"seed,omitempty"`
	Verbosity  int             `json:"verbosity,omitempty"`
}

// ExitSpec defines exit rules for a trade.
// The earliest satisfied rule closes the trade.
type ExitSpec struct {
	ExitByDaysToExpiry *int     `json:"exit_by_days_to_expiry,omitempty"`
	MaxDaysInTrade     *int     `json:"max_days_in_trade,omitempty"`
	UnderlyingMovePx   *float64 `json:"underlying_move_px,omitempty"`
	ProfitTargetPct    *float64 `json:"profit_target_pct,omitempty"`
	StopLossPct        *float64 `json:"stop_loss_pct,omitempty"`
}

// Trade represents a complete lifecycle of a strategy execution.
type Trade struct {
	ID                int
	OpenDateTime      time.Time
	CloseDateTime     *time.Time
	UnderlyingAtOpen  float64
	UnderlyingAtClose float64
	Legs              []st.TradeLeg
	OpenPremium       float64
	ClosePremium      float64
	HighPremium       float64
	LowPremium        float64
	ClosedBy          string
}

const (
	VerbosityError = iota // 0
	VerbosityInfo         // 1
	VerbosityDebug        // 2
	VerbosityTrace        // 3
)

// Result holds all completed trades.
type Result struct {
	Trades []Trade `json:"trades"`
}

// NewEngine constructs a new backtest engine.
func NewEngine(cfg *Config, prov data.Provider) *Engine {
	return &Engine{cfg: cfg, prov: prov}
}

// Run executes the backtest from scheduling to trade simulation.
func (e *Engine) Run() (*Result, error) {
	cfg := e.cfg

	if cfg.ReportDir == "" {
		cfg.ReportDir = "./out"
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}
	if cfg.Verbosity < VerbosityError || cfg.Verbosity > VerbosityTrace {
		cfg.Verbosity = VerbosityInfo
	}
	logger.SetVerbosity(cfg.Verbosity)

	// Fetch daily bars
	dailyBars, err := e.prov.GetBars(cfg.Underlying, cfg.Entry.StartDate, cfg.Entry.EndDate, 1, "day")
	if err != nil || len(dailyBars) == 0 {
		// fallback synthetic
		logger.Infof("provider bars error or empty: %v - generating synthetic", err)
		// bars = generateSyntheticSeries(cfg.Underlying, start, end)	/* 🔥 TODO: replaced with synthetic provider */
	}

	barMap := make(map[string]data.Bar, len(dailyBars))
	for _, b := range dailyBars {
		barMap[b.Date.Format("2006-01-02")] = b
	}

	// historical vol
	hv := AnnualizedVolatility(extractCloses(dailyBars))
	logger.Infof("hist vol = %.2f%%", hv*100)

	// get list of expiryList for the underlying during backtest period
	expiryList, err := e.prov.GetRelevantExpiries(cfg.Underlying, cfg.Entry.StartDate, cfg.Entry.EndDate)
	if err != nil {
		return nil, fmt.Errorf("backtest scheduler error: get relevant expiries error, %w", err)
	}

	// schedule
	dates, err := sch.ScheduleDates(cfg.Entry, dailyBars, expiryList)
	if err != nil || len(dates) == 0 {
		return nil, fmt.Errorf("failed to schedule dates: %w", err)
	}

	var trades []Trade
	id := 1
	for _, dt := range dates {
		// TODO: max trades limit
		// if cfg.MaxTrades > 0 && len(trades) >= cfg.MaxTrades {
		// 	break
		// }
		bar, ok := barMap[dt.Format("2006-01-02")]
		if !ok {
			logger.Debugf("no bar for %s", dt.Format("2006-01-02"))
			continue
		}
		// intentionally using close price of bars as open (picking bar at open time)
		openPrice := bar.Close

		// build legs
		var legs []st.TradeLeg
		legs, err = st.PlanStrategy(cfg.Strategy, dt, cfg.Underlying, openPrice, expiryList, e.prov)
		if err != nil {
			logger.Infof("error on trade date %s, skipped", dt.Format("2006-01-02"))
			logger.Debugf("skipping trade on %s: build legs error: %v", dt.Format("2006-01-02"), err)
			continue
		}

		// price legs
		openPremium := 0.0
		for _, leg := range legs {
			p, err := e.prov.GetOptionPrice(
				cfg.Underlying,
				leg.Strike,
				leg.Expiration,
				leg.Spec.OptionType,
				dt,
			)
			if err != nil {
				// fallback to BS
				logger.Debugf(
					"option price fallback BS %s %s K=%.2f exp=%s err=%v",
					cfg.Underlying,
					leg.Spec.OptionType,
					leg.Strike,
					leg.Expiration.Format("2006-01-02"),
					err,
				)
				p = pricing.BlackScholesPrice(
					openPrice,
					leg.Strike,
					(leg.Expiration.Sub(dt).Hours() / (24 * 365)),
					0.02,
					hv, // historical volatility
					strings.ToLower(leg.Spec.OptionType) == "call",
				)
			}
			side := strings.ToLower(leg.Spec.Side)
			sign := 1.0
			if side == "sell" {
				sign = -1.0
			}
			openPremium += sign * p * float64(leg.Spec.Qty) * 100.0
		}

		trade := Trade{
			ID:               id,
			OpenDateTime:     dt,
			UnderlyingAtOpen: openPrice,
			Legs:             legs,
			OpenPremium:      openPremium,
			HighPremium:      openPremium,
			LowPremium:       openPremium,
		}
		id++
		logger.Infof(
			"trade %d opened %s underlying=%.2f open premium=%.2f",
			trade.ID,
			dt.Format("2006-01-02"),
			openPrice,
			openPremium,
		)

		simCloseTrade(&trade, dailyBars, hv, *cfg, e.prov)
		trades = append(trades, trade)
		logger.Infof("trade %d closed_by=%s close premium=%.2f pnl=%.2f",
			trade.ID,
			trade.ClosedBy,
			trade.ClosePremium,
			trade.ClosePremium-trade.OpenPremium,
		)
	}

	// sort trades by ID (stable)
	sort.Slice(trades, func(i, j int) bool { return trades[i].ID < trades[j].ID })

	res := &Result{Trades: trades}
	return res, nil
}

// PriceOption uses provider price else BS
func PriceOption(
	prov data.Provider,
	underlying string,
	S, K float64,
	asOfDate time.Time,
	expiryDate time.Time,
	optType string,
	hv float64,
	overrideIV *float64,
) (float64, error) {
	if prov != nil {
		p, err := prov.GetOptionPrice(underlying, K, expiryDate, optType, asOfDate)
		if err == nil && p > 0 {
			return p, nil
		}
	}
	iv := hv // use historical vol
	if overrideIV != nil {
		iv = *overrideIV // override if provided
	}

	// TODO: risk-free rate from provider or config - using 2% fixed here
	return pricing.BlackScholesPrice(
		S, K,
		(expiryDate.Sub(asOfDate).Hours() / (24 * 365)),
		0.02,
		iv,
		strings.ToLower(optType) == "call",
	), nil
}

func simCloseTrade(
	tr *Trade,
	dailyBars []data.Bar,
	historicalVolatility float64,
	cfg Config,
	prov data.Provider,
) {
	closeByDateTime := tr.Legs[0].Expiration // default to first leg expiry date
	openDateTime := tr.OpenDateTime

	if cfg.Exit.ExitByDaysToExpiry != nil {
		if *cfg.Exit.ExitByDaysToExpiry <= 0 {
			// invalid config
		} else {
			closeByDateTime = closeByDateTime.AddDate(0, 0, -*cfg.Exit.ExitByDaysToExpiry)
		}
	}

	if cfg.Exit.MaxDaysInTrade != nil {
		if *cfg.Exit.MaxDaysInTrade <= 0 {
			// invalid config
		} else if openDateTime.AddDate(0, 0, *cfg.Exit.MaxDaysInTrade).Before(closeByDateTime) {
			closeByDateTime = openDateTime.AddDate(0, 0, *cfg.Exit.MaxDaysInTrade)
		}
	}

	if cfg.Exit.UnderlyingMovePx != nil {
		// check underlying move to adjust closeByDateTime
		exitByUnderlyingMove(tr, dailyBars, &closeByDateTime, cfg, prov)
	}

	if cfg.Exit.ProfitTargetPct == nil &&
		cfg.Exit.StopLossPct == nil {
		// TODO: no exit rules - just close the trade on closeByDateTime and exit
		return
	}
}

func exitByUnderlyingMove(
	tr *Trade,
	dailyBars []data.Bar,
	closeByDateTime *time.Time,
	cfg Config,
	prov data.Provider,
) {

	if cfg.Exit.UnderlyingMovePx == nil {
		return
	}

	for _, dailyBar := range dailyBars {
		if dailyBar.Date.After(*closeByDateTime) {
			break
		}

		// find max move and check if exceeded threshold to exit trade early
		move := dailyBar.High - tr.UnderlyingAtOpen
		if tr.UnderlyingAtOpen-dailyBar.Low > move {
			move = tr.UnderlyingAtOpen - dailyBar.Low
		}

		if move >= *cfg.Exit.UnderlyingMovePx {
			// TODO: remove hardcoded timespan and multiplier
			minuteBars, err := prov.GetBars(cfg.Underlying, dailyBar.Date, dailyBar.Date, 1, "minute")
			if err != nil || len(minuteBars) == 0 {
				// fallback synthetic
				logger.Infof("provider bars error or empty: %v - generating synthetic", err)
				// bars = generateSyntheticSeries(cfg.Underlying, start, end)	/* 🔥 TODO: replaced with synthetic provider */
			}
			for _, minuteBar := range minuteBars {
				move := minuteBar.High - tr.UnderlyingAtOpen
				if tr.UnderlyingAtOpen-minuteBar.Low > move {
					move = tr.UnderlyingAtOpen - minuteBar.Low
				}
				if move >= *cfg.Exit.UnderlyingMovePx {
					*closeByDateTime = minuteBar.Date
					return
				}
			}
		}
	}
}

// exitByPnL scans minute-by-minute option prices and exits
// immediately when profit target or stop loss is hit.

// Returns true if trade was closed.
func exitByPnL(
	trade *Trade,
	closeByDateTime time.Time,
	cfg Config,
	prov data.Provider,
) {
	one := 1
	minute := "minute"

	legsData := make([][]data.Bar, len(trade.Legs)+1)

	// fetch minute bars for all option legs
	for i, leg := range trade.Legs {
		symbol := data.OptionSymbolFromParts(cfg.Underlying, leg.Expiration, leg.Spec.OptionType, leg.Strike)
		bars, err := prov.GetBars(symbol, trade.OpenDateTime, closeByDateTime, one, minute)
		if err != nil {
			logger.Errorf("failed to get minute bars for leg %s (%d): %v", symbol, i, err)
			continue
		}
		legsData[i+1] = bars
	}

	// fetch minute bars for underlying
	bars, err := prov.GetBars(cfg.Underlying, trade.OpenDateTime, closeByDateTime, one, minute)
	if err != nil {
		logger.Errorf("failed to get minute bars for underlying %s: %v", cfg.Underlying, err)
	}
	legsData[0] = bars

	// scan minute by minute
	minuteData := make(map[time.Time][]data.Bar)
	for _, bars := range legsData {
		for _, b := range bars {
			minuteData[b.Date] = append(minuteData[b.Date], b)
		}
	}

}

// priceLegsAt prices all legs at a given timestamp.
func priceLegsAt(
	prov data.Provider,
	underlying string,
	legs []st.TradeLeg,
	spot float64,
	asOf time.Time,
	hv float64,
) float64 {
	total := 0.0

	for _, leg := range legs {
		p, err := prov.GetOptionPrice(
			underlying,
			leg.Strike,
			leg.Expiration,
			leg.Spec.OptionType,
			asOf,
		)
		if err != nil || p <= 0 {
			p = pricing.BlackScholesPrice(
				spot,
				leg.Strike,
				leg.Expiration.Sub(asOf).Hours()/(24*365),
				0.02,
				hv,
				strings.ToLower(leg.Spec.OptionType) == "call",
			)
		}

		sign := 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			sign = -1
		}
		total += sign * p * float64(leg.Spec.Qty) * 100
	}
	return total
}

func AnnualizedVolatility(closes []float64) float64 {
	if len(closes) < 2 {
		return 0.30
	}
	var rets []float64
	for i := 1; i < len(closes); i++ {
		rets = append(rets, math.Log(closes[i]/closes[i-1]))
	}
	mean := 0.0
	for _, v := range rets {
		mean += v
	}
	mean /= float64(len(rets))
	sd := 0.0
	for _, v := range rets {
		sd += (v - mean) * (v - mean)
	}
	sd = math.Sqrt(sd / float64(len(rets)-1))
	return sd * math.Sqrt(252.0)
}

func extractCloses(bars []data.Bar) []float64 {
	var closes []float64
	for _, b := range bars {
		closes = append(closes, b.Close)
	}
	return closes
}
