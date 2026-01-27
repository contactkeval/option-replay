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
	MaxTrades  int             `json:"max_trades,omitempty"` // max trades to execute, 0 = unlimited
	ReportDir  string          `json:"report_dir,omitempty"` // report directory
	Seed       int64           `json:"seed,omitempty"`       // random seed for stochastic elements
	Verbosity  int             `json:"verbosity,omitempty"`  // 0=errors,1=info,2=debug,3=trace
}

// ExitSpec defines various exit rules for trades
type ExitSpec struct {
	ProfitTargetPct    *float64 `json:"profit_target_pct,omitempty"`      // e.g. 50.0 for 50%
	StopLossPct        *float64 `json:"stop_loss_pct,omitempty"`          // e.g. 30.0 for 30%
	UnderlyingMovePx   *float64 `json:"underlying_move_px,omitempty"`     // e.g. 5.0 for $5 move
	MaxDaysInTrade     *int     `json:"max_days_in_trade,omitempty"`      // e.g. 10 for 10 days
	ExitByDaysToExpiry *int     `json:"exit_by_days_to_expiry,omitempty"` // e.g. 5 for exit when any leg has ≤5 days to expiry
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

		tr := Trade{
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
			tr.ID,
			dt.Format("2006-01-02"),
			openPrice,
			openPremium,
		)
		id++
		// simulate
		simCloseTrade(&tr, bars, barMap, hv, *cfg, e.prov)
		trades = append(trades, tr)
		logger.Infof("trade %d closed_by=%s close premium=%.2f pnl=%.2f",
			tr.ID,
			tr.ClosedBy,
			tr.ClosePremium,
			tr.ClosePremium-tr.OpenPremium,
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

// simCloseTrade simulates the closing of a trade by iterating through historical bar data
// to determine when and how the trade exits. It updates the trade's close details including
// the close date, close premium, underlying price at close, and the reason for closure.
//
// The function searches for the bar corresponding to the trade's open date. If no data exists
// for that date, it closes the trade immediately with no price change and marks it as "no_data".
//
// For each subsequent bar, it calculates the total premium of all trade legs:
//   - If a leg has expired, it uses the intrinsic value (payoff at expiration)
//   - If a leg is still active, it fetches the option price from the provider or falls back
//     to Black-Scholes pricing if the provider returns no data
//
// The function tracks the high and low premiums reached during the trade's life. It then
// checks for exit conditions (stop loss, profit target, etc.) via checkExits. If an exit
// condition is met, the trade closes with that reason. If all legs expire naturally, the
// trade closes with reason "expired". If the bar data ends without an explicit exit, the
// trade closes at the last available bar with reason "data_end".
//
// Parameters:
//   - tr: pointer to the Trade being simulated
//   - bars: slice of historical bar data sorted chronologically
//   - barMap: map of bar data by key (currently unused in function)
//   - historicalVolatility: volatility used for Black-Scholes fallback pricing
//   - cfg: configuration containing the underlying symbol and exit parameters
//   - prov: data provider for fetching option prices
func simCloseTrade(
	tr *Trade,
	bars []data.Bar,
	barMap map[string]data.Bar,
	historicalVolatility float64,
	cfg Config,
	prov data.Provider,
) {

	// openKey := tr.OpenDateTime.Format("2006-01-02")
	// idx := -1
	// for i, b := range bars {
	// 	if b.Date.Format("2006-01-02") == openKey {
	// 		idx = i
	// 		break
	// 	}
	// }
	// // If no bar found at or after open date
	// if idx == -1 {

	// Efficiently find the starting bar using binary search instead of string formatting
	idx := sort.Search(len(bars), func(i int) bool {
		return !bars[i].Date.Before(tr.OpenDateTime)
	})

	// If no bar found at or after open date
	if idx == len(bars) {
		now := time.Now().UTC()
		tr.CloseDateTime = &now
		tr.ClosePremium = tr.OpenPremium
		tr.ClosedBy = "no_data"
		return
	}

	for i := idx; i < len(bars); i++ {
		b := bars[i]
		// compute premium
		total := 0.0
		for _, leg := range tr.Legs {
			// if leg already expired before this date, use intrinsic
			if !b.Date.Before(leg.Expiration) {
				// at or after expiration -> intrinsic
				intr := 0.0
				if strings.ToLower(leg.Spec.OptionType) == "call" {
					intr = math.Max(0.0, b.Close-leg.Strike)
				} else {
					intr = math.Max(0.0, leg.Strike-b.Close)
				}
				side := strings.ToLower(leg.Spec.Side)
				sign := 1.0
				if side == "sell" {
					sign = -1.0
				}
				total += sign * intr * float64(leg.Spec.Qty) * 100.0
				continue
			}
			// active leg -> price via provider else BS
			p, err := prov.GetOptionPrice(cfg.Underlying, leg.Strike, leg.Expiration, leg.Spec.OptionType, b.Date)
			if err != nil || p <= 0 {
				//TODO: risk-free rate from provider or config - using 2% fixed here
				logger.Debugf(
					"option price fallback BS %s %s K=%.2f exp=%s err=%v",
					cfg.Underlying,
					leg.Spec.OptionType,
					leg.Strike,
					leg.Expiration.Format("2006-01-02"),
					err,
				)
				p = pricing.BlackScholesPrice(
					b.Close,
					leg.Strike,
					(leg.Expiration.Sub(b.Date).Hours() / (24 * 365)),
					0.02,
					historicalVolatility,
					strings.ToLower(leg.Spec.OptionType) == "call",
				)
			}
			side := strings.ToLower(leg.Spec.Side)
			sign := 1.0
			if side == "sell" {
				sign = -1.0
			}
			total += sign * p * float64(leg.Spec.Qty) * 100.0
		}

		if total > tr.HighPremium {
			tr.HighPremium = total
		}
		if total < tr.LowPremium {
			tr.LowPremium = total
		}

		// check exits
		reason := checkExits(tr, total, b, cfg)
		if reason != "" {
			logger.Debugf(
				"trade %d exit %s on %s premium=%.2f underlying=%.2f",
				tr.ID,
				reason,
				b.Date.Format("2006-01-02"),
				total,
				b.Close,
			)
			tr.ClosePremium = total
			tr.UnderlyingAtClose = b.Close
			t := b.Date
			tr.CloseDateTime = &t
			tr.ClosedBy = reason
			return
		}

		// if all legs are expired now -> trade expired
		allExpired := true
		for _, leg := range tr.Legs {
			if b.Date.Before(leg.Expiration) {
				allExpired = false
				break
			}
		}
		if allExpired {
			// compute intrinsic for all legs (already handled in loop but ensure close)
			tr.ClosePremium = total
			tr.UnderlyingAtClose = b.Close
			t := b.Date
			tr.CloseDateTime = &t
			tr.ClosedBy = "expired"
			return
		}
	}

	// end of data
	last := bars[len(bars)-1]
	tr.ClosePremium = tr.HighPremium
	tr.UnderlyingAtClose = last.Close
	t := last.Date
	tr.CloseDateTime = &t
	tr.ClosedBy = "data_end"
}

// checkExits evaluates whether a trade should be exited based on configured exit rules.
// It checks the current premium against the opening premium and applies the following exit conditions in order:
// - ProfitTargetPct: exits if the trade has gained the specified percentage
// - StopLossPct: exits if the trade has lost the specified percentage
// - UnderlyingMovePx: exits if the underlying price has moved by the specified amount
// - MaxDaysInTrade: exits if the trade has been open for the specified number of days
// - ExitDaysBeforeExpiry: exits if any leg is within the specified number of days before expiration
//
// For percent-based calculations, the change is computed relative to the absolute value of the opening premium.
// For credits (negative open premium), profit occurs when the premium increases toward zero.
// For debits (positive open premium), profit occurs when the premium increases.
//
// Parameters:
//   - tr: the Trade to evaluate
//   - currPremium: the current premium price
//   - bar: the current market data bar
//   - cfg: the backtest configuration containing exit rules
//
// Returns:
// A string describing the exit reason if any exit condition is met, or an empty string if no exits are triggered.
func checkExits(
	tr *Trade,
	currPremium float64,
	bar data.Bar,
	cfg Config,
) string {

	open := tr.OpenPremium
	// p/l change measured as (current - open)
	change := currPremium - open
	// for percent-based rules compute relative to notional magnitude (abs(open) or 1 if zero)
	base := math.Abs(open)
	if base < 1e-9 {
		base = 1.0
	}
	changePct := change / base * 100.0

	if cfg.Exit.ProfitTargetPct != nil {
		// interpretation:
		// - for credits (open < 0): profit occurs when curr moves toward 0 (i.e., change is positive)
		// - for debits (open > 0): profit occurs when curr increases (change positive)
		pct := *cfg.Exit.ProfitTargetPct
		if pct >= 0 {
			if open < 0 {
				// credit: want currPremium to increase by pct of |open| (toward zero)
				if changePct >= pct {
					return fmt.Sprintf("profit_target_%.2f%%", pct)
				}
			} else {
				if changePct >= pct {
					return fmt.Sprintf("profit_target_%.2f%%", pct)
				}
			}
		}
	}

	if cfg.Exit.StopLossPct != nil {
		pct := *cfg.Exit.StopLossPct
		if changePct <= -pct {
			return fmt.Sprintf("stop_loss_%.2f%%", pct)
		}
	}

	if cfg.Exit.UnderlyingMovePx != nil {
		if math.Abs(bar.Close-tr.UnderlyingAtOpen) >= *cfg.Exit.UnderlyingMovePx {
			return fmt.Sprintf("underlying_move_%.2f", *cfg.Exit.UnderlyingMovePx)
		}
	}

	if cfg.Exit.MaxDaysInTrade != nil {
		days := int(math.Floor(bar.Date.Sub(tr.OpenDateTime).Hours() / 24))
		if days >= *cfg.Exit.MaxDaysInTrade {
			return fmt.Sprintf("max_days_%d", *cfg.Exit.MaxDaysInTrade)
		}
	}

	if cfg.Exit.ExitByDaysToExpiry != nil {
		minDays := math.MaxInt32
		for _, leg := range tr.Legs {
			d := int(math.Ceil(leg.Expiration.Sub(bar.Date).Hours() / 24.0))
			if d < minDays {
				minDays = d
			}
		}
		if minDays <= *cfg.Exit.ExitByDaysToExpiry {
			return fmt.Sprintf("exit_%ddays_before_expiry", *cfg.Exit.ExitByDaysToExpiry)
		}
	}

	return ""
}

func extractCloses(bars []data.Bar) []float64 {
	var closes []float64
	for _, b := range bars {
		closes = append(closes, b.Close)
	}
	return closes
}
