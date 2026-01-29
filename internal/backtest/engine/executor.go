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

type Engine struct {
	cfg  *Config
	prov data.Provider
}

// Config struct
type Config struct {
	Underlying string          `json:"underlying"`           // e.g. "AAPL"
	Entry      sch.EntryRule   `json:"entry"`                // entry rules
	Strategy   st.StrategySpec `json:"strategy"`             // option legs
	Exit       ExitSpec        `json:"exit"`                 // exit rules
	ReportDir  string          `json:"report_dir,omitempty"` // report directory
	// TODO: implement logic for below properties
	MaxTrades int   `json:"max_trades,omitempty"` // max trades to execute, 0 = unlimited
	Seed      int64 `json:"seed,omitempty"`       // random seed for stochastic elements
	Verbosity int   `json:"verbosity,omitempty"`  // 0=errors,1=info,2=debug,3=trace
}

// ExitSpec defines various exit rules for trades
type ExitSpec struct {
	ExitByDaysToExpiry *int     `json:"exit_by_days_to_expiry,omitempty"` // e.g. 5 for exit when any leg has ≤5 days to expiry
	MaxDaysInTrade     *int     `json:"max_days_in_trade,omitempty"`      // e.g. 10 for 10 days
	UnderlyingMovePx   *float64 `json:"underlying_move_px,omitempty"`     // e.g. 5.0 for $5 move
	ProfitTargetPct    *float64 `json:"profit_target_pct,omitempty"`      // e.g. 50.0 for 50%
	StopLossPct        *float64 `json:"stop_loss_pct,omitempty"`          // e.g. 30.0 for 30%
}

type Trade struct {
	ID                int           // unique trade ID
	OpenDateTime      time.Time     // trade open date time
	CloseDateTime     *time.Time    // trade close date time
	UnderlyingAtOpen  float64       // underlying price at open
	UnderlyingAtClose float64       // underlying price at close
	Legs              []st.TradeLeg // trade legs (strategy)
	OpenPremium       float64       // total premium at open for entire strategy
	ClosePremium      float64       // total premium at close for entire strategy
	HighPremium       float64       // highest premium during trade
	LowPremium        float64       // lowest premium during trade
	ClosedBy          string        // reason for closing the trade
}

const (
	VerbosityError = iota // 0
	VerbosityInfo         // 1
	VerbosityDebug        // 2
	VerbosityTrace        // 3
)

// Result mirrors original
type Result struct {
	Trades []Trade `json:"trades"`
}

func NewEngine(cfg *Config, prov data.Provider) *Engine {
	return &Engine{cfg: cfg, prov: prov}
}

// Run executes the backtest
func (e *Engine) Run() (*Result, error) {
	cfg := e.cfg
	// fill defaults
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

	// fetch bars
	bars, err := e.prov.GetBars(cfg.Underlying, cfg.Entry.StartDate, cfg.Entry.EndDate, 1, "day")
	if err != nil || len(bars) == 0 {
		// fallback synthetic
		logger.Infof("provider bars error or empty: %v - generating synthetic", err)
		// bars = generateSyntheticSeries(cfg.Underlying, start, end)	/* 🔥 TODO: replaced with synthetic provider */
	}

	// build map
	barMap := make(map[string]data.Bar, len(bars))
	for _, b := range bars {
		k := b.Date.Format("2006-01-02")
		barMap[k] = b
	}

	// historical vol
	closes := extractCloses(bars)
	hv := AnnualizedVolatility(closes)
	logger.Infof("hist vol = %.2f%%", hv*100)

	// get list of expiryList for the underlying during backtest period
	expiryList, err := e.prov.GetRelevantExpiries(cfg.Underlying, cfg.Entry.StartDate, cfg.Entry.EndDate)
	if err != nil {
		return nil, fmt.Errorf("backtest scheduler error: get relevant expiries error, %w", err)
	}

	// schedule
	dates, err := sch.ScheduleDates(cfg.Entry, bars, expiryList)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule dates: %w", err)
	}
	if len(dates) == 0 {
		return nil, fmt.Errorf("no dates scheduled")
	}
	logger.Infof("%d schedule dates", len(dates))

	var trades []Trade
	id := 1
	for _, dt := range dates {
		// TODO: max trades limit
		// if cfg.MaxTrades > 0 && len(trades) >= cfg.MaxTrades {
		// 	break
		// }
		bk := dt.Format("2006-01-02")
		bar, ok := barMap[bk]
		if !ok {
			logger.Debugf("no bar for %s", bk)
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
		logger.Infof(
			"trade %d opened %s underlying=%.2f open premium=%.2f",
			trade.ID,
			dt.Format("2006-01-02"),
			openPrice,
			openPremium,
		)
		id++
		// simulate
		simCloseTrade(&trade, bars, hv, *cfg, e.prov)
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
	bars []data.Bar,
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
		// TODO: find earliest underlying move exit date and adjust closeByDateTime
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
	cfg Config,
) time.Time {

	if cfg.Exit.UnderlyingMovePx == nil {
		return time.Time{}
	}

	for _, dailyBar := range dailyBars {
		move := dailyBar.High - tr.UnderlyingAtOpen
		if tr.UnderlyingAtOpen-dailyBar.Low > move {
			move = tr.UnderlyingAtOpen - dailyBar.Low
		}
		if move >= *cfg.Exit.UnderlyingMovePx {
			// TODO: fetch minute bar for this bar date and find exact time
			return dailyBar.Date
		}
	}

	return time.Time{}
}

func extractCloses(bars []data.Bar) []float64 {
	var closes []float64
	for _, b := range bars {
		closes = append(closes, b.Close)
	}
	return closes
}
