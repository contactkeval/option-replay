package backtest

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/pricing"
)

type Engine struct {
	cfg  *Config
	prov data.Provider
}

// Config struct
type Config struct {
	Underlying   string        `json:"underlying"`                // e.g. "AAPL"
	Entry        EntryRule     `json:"entry"`                     // entry rules
	DaysToExpiry int           `json:"dte,omitempty"`             // default DTE if not specified in legs
	MatchType    DateMatchType `json:"date_match_type,omitempty"` // date matching type, default "nearest"
	Strategy     []LegSpec     `json:"strategy"`                  // option legs
	Exit         ExitSpec      `json:"exit"`                      // exit rules
	MaxTrades    int           `json:"max_trades,omitempty"`      // max trades to execute, 0 = unlimited
	OutputDir    string        `json:"output_dir,omitempty"`      // output directory
	Seed         int64         `json:"seed,omitempty"`            // random seed for stochastic elements
	Verbosity    int           `json:"verbosity,omitempty"`       // 0=errors,1=info,2=debug
}

// Individual leg specification
type LegSpec struct {
	Side       string `json:"side,omitempty"`        // "buy" or "sell", defaults to "buy"
	OptionType string `json:"option_type,omitempty"` // "call" or "put", defaults to "call"
	StrikeRule string `json:"strike_rule"`           // "ATM", "ABS:100", "DELTA:0.3", etc.
	Qty        int    `json:"qty,omitempty"`         // used for ratio spreads, defaults to one
	Expiration string `json:"expiration,omitempty"`  // used for calendar spreads, defaults DTE from config
	LegName    string `json:"leg_name,omitempty"`    // used for dependent wings
}

// ExitSpec defines various exit rules for trades
type ExitSpec struct {
	ProfitTargetPct    *float64 `json:"profit_target_pct,omitempty"`      // e.g. 50.0 for 50%
	StopLossPct        *float64 `json:"stop_loss_pct,omitempty"`          // e.g. 30.0 for 30%
	UnderlyingMovePx   *float64 `json:"underlying_move_px,omitempty"`     // e.g. 5.0 for $5 move
	MaxDaysInTrade     *int     `json:"max_days_in_trade,omitempty"`      // e.g. 10 for 10 days
	ExitByDaysToExpiry *int     `json:"exit_by_days_to_expiry,omitempty"` // e.g. 5 for exit when any leg has ≤5 days to expiry
}

// Trade/TradeLeg/Bar types reused from original but simplified for internal use
type TradeLeg struct {
	Spec         LegSpec
	Strike       float64
	OptType      string
	Qty          int
	Expiration   time.Time
	OpenPremium  float64
	ClosePremium float64
}

type Trade struct {
	ID                int
	OpenTime          time.Time
	CloseTime         *time.Time
	UnderlyingAtOpen  float64
	UnderlyingAtClose float64
	Legs              []TradeLeg
	OpenPremium       float64
	ClosePremium      float64
	HighPremium       float64
	LowPremium        float64
	ClosedBy          string
}

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
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./out"
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}
	if cfg.Verbosity < 0 || cfg.Verbosity > 2 {
		cfg.Verbosity = 1
	}

	// fetch bars
	bars, err := e.prov.GetDailyBars(cfg.Underlying, cfg.Entry.Start, cfg.Entry.End)
	if err != nil || len(bars) == 0 {
		// fallback synthetic
		log.Printf("[warn] provider bars error or empty: %v - generating synthetic", err)
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
	if cfg.Verbosity >= 1 {
		log.Printf("[info] hist vol = %.2f%%", hv*100)
	}

	// get list of expiries for the underlying during backtest period
	expiries, err := GetRelevantExpiries(cfg.Underlying, cfg.Entry.Start, cfg.Entry.End, e.prov)
	if err != nil {
		return nil, fmt.Errorf("backtest scheduler error: get relevant expiries error, %w", err)
	}

	// schedule
	dates, err := ResolveScheduleDates(cfg.Entry, bars, expiries)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve schedule dates: %w", err)
	}
	if len(dates) == 0 {
		return nil, fmt.Errorf("no schedule dates resolved")
	}
	if cfg.Verbosity == 1 {
		log.Printf("[info] %d schedule dates", len(dates))
	}

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
			if cfg.Verbosity >= 2 {
				log.Printf("[debug] no bar for %s", bk)
			}
			continue
		}
		openPrice := bar.Close

		// build legs
		var legs []TradeLeg
		okLegs := true
		for _, ls := range cfg.Strategy {
			exp := ResolveExpiration(dt, cfg.DaysToExpiry, expiries, cfg.Entry.DateMatchType)
			strike, err := ResolveStrike(ls.StrikeRule, cfg.Underlying, openPrice, dt, exp, legs)
			if err != nil {
				okLegs = false
				break
			}
			// TODO: OpenPremium pricing later
			legs = append(legs, TradeLeg{Spec: ls, Strike: strike, OptType: ls.OptionType, Qty: ls.Qty, Expiration: exp, OpenPremium: 0.0})
		}
		if !okLegs || len(legs) == 0 {
			continue
		}

		// price legs
		openPremium := 0.0
		for _, leg := range legs {
			p, err := e.prov.GetOptionMidPrice(cfg.Underlying, leg.Strike, leg.Expiration, leg.OptType)
			if err != nil {
				// fallback to BS
				p = pricing.BlackScholesPrice(openPrice, leg.Strike, 0.02, hv, leg.Expiration.Sub(dt), strings.ToLower(leg.OptType))
			}
			side := strings.ToLower(leg.Spec.Side)
			sign := 1.0
			if side == "sell" {
				sign = -1.0
			}
			openPremium += sign * p * float64(leg.Qty) * 100.0
		}

		tr := Trade{ID: id, OpenTime: dt, UnderlyingAtOpen: openPrice, Legs: legs, OpenPremium: openPremium, HighPremium: openPremium, LowPremium: openPremium}
		id++
		// simulate
		simCloseTrade(&tr, bars, barMap, *cfg, e.prov, hv)
		trades = append(trades, tr)
		if cfg.Verbosity >= 1 {
			log.Printf("[info] trade %d opened %s closed_by=%s pnl=%.2f", tr.ID, tr.OpenTime.Format("2006-01-02"), tr.ClosedBy, tr.ClosePremium-tr.OpenPremium)
		}
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
func PriceOption(prov data.Provider, underlying string, S, K float64, at time.Time, expiry time.Time, optType string, hv float64, overrideIV *float64) (float64, error) {
	if prov != nil {
		p, err := prov.GetOptionMidPrice(underlying, K, expiry, optType)
		if err == nil && p > 0 {
			return p, nil
		}
	}
	iv := hv
	if overrideIV != nil {
		iv = *overrideIV
	}
	return pricing.BlackScholesPrice(S, K, 0.02, iv, expiry.Sub(at), strings.ToLower(optType)), nil
}

// simCloseTrade: corrected expiration handling (per-leg) and exits
func simCloseTrade(tr *Trade, bars []data.Bar, barMap map[string]data.Bar, cfg Config, prov data.Provider, hv float64) {
	openKey := tr.OpenTime.Format("2006-01-02")
	idx := -1
	for i, b := range bars {
		if b.Date.Format("2006-01-02") == openKey {
			idx = i
			break
		}
	}
	if idx == -1 {
		now := time.Now().UTC()
		tr.CloseTime = &now
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
				if strings.ToLower(leg.OptType) == "call" {
					intr = math.Max(0.0, b.Close-leg.Strike)
				} else {
					intr = math.Max(0.0, leg.Strike-b.Close)
				}
				side := strings.ToLower(leg.Spec.Side)
				sign := 1.0
				if side == "sell" {
					sign = -1.0
				}
				total += sign * intr * float64(leg.Qty) * 100.0
				continue
			}
			// active leg -> price via provider else BS
			p, err := prov.GetOptionMidPrice(cfg.Underlying, leg.Strike, leg.Expiration, leg.OptType)
			if err != nil || p <= 0 {
				p = pricing.BlackScholesPrice(b.Close, leg.Strike, 0.02, hv, leg.Expiration.Sub(b.Date), strings.ToLower(leg.OptType))
			}
			side := strings.ToLower(leg.Spec.Side)
			sign := 1.0
			if side == "sell" {
				sign = -1.0
			}
			total += sign * p * float64(leg.Qty) * 100.0
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
			tr.ClosePremium = total
			tr.UnderlyingAtClose = b.Close
			t := b.Date
			tr.CloseTime = &t
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
			tr.CloseTime = &t
			tr.ClosedBy = "expired"
			return
		}
	}

	// end of data
	last := bars[len(bars)-1]
	tr.ClosePremium = tr.HighPremium
	tr.UnderlyingAtClose = last.Close
	t := last.Date
	tr.CloseTime = &t
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
func checkExits(tr *Trade, currPremium float64, bar data.Bar, cfg Config) string {
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
		days := int(bar.Date.Sub(tr.OpenTime).Hours() / 24.0)
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
