// Package engine provides the core simulation logic for backtesting option strategies.
// It handles trade scheduling, entry execution, and lifecycle management via price action and exit conditions.
package engine

import (
	"fmt"
	"strings"
	"time"

	sch "github.com/contactkeval/option-replay/internal/backtest/scheduler"
	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pricing"
)

// Engine orchestrates the backtest execution.
// It acts as the central coordinator between the data provider and strategy logic.
type Engine struct {
	cfg  *Config
	prov data.Provider
}

// Config defines the operational parameters for a single backtest run.
type Config struct {
	Underlying string          `json:"underlying"` // Ticker symbol of the asset
	Entry      sch.EntryRule   `json:"entry"`      // Rules governing when to open trades
	Strategy   st.StrategySpec `json:"strategy"`   // The option structure (e.g., Iron Condor)
	Exit       ExitSpec        `json:"exit"`       // Rules governing when to close trades
	ReportDir  string          `json:"report_dir,omitempty"`
	MaxTrades  int             `json:"max_trades,omitempty"`
	Seed       int64           `json:"seed,omitempty"`
	Verbosity  int             `json:"verbosity,omitempty"`
}

// ExitSpec defines the multi-condition exit logic for a trade.
// The engine checks these conditions in a specific order; the first condition met triggers the exit.
type ExitSpec struct {
	// ExitByDaysToExpiry forces an exit N days before the contract expires.
	ExitByDaysToExpiry *int `json:"exit_by_days_to_expiry,omitempty"`
	// MaxDaysInTrade limits the duration of the trade regardless of profit/loss.
	MaxDaysInTrade *int `json:"max_days_in_trade,omitempty"`
	// UnderlyingMovePx triggers an exit if the stock price moves by this absolute dollar amount.
	UnderlyingMovePx *float64 `json:"underlying_move_px,omitempty"`
	// ProfitTargetPct triggers an exit if the net premium gains this percentage.
	ProfitTargetPct *float64 `json:"profit_target_pct,omitempty"`
	// StopLossPct triggers an exit if the net premium loses this percentage.
	StopLossPct *float64 `json:"stop_loss_pct,omitempty"`
}

// Trade represents the full history and state of a single strategy execution.
type Trade struct {
	ID                int           // Unique trade identifier
	OpenDateTime      time.Time     // Entry timestamp
	CloseDateTime     *time.Time    // Exit timestamp (nil if trade is open)
	UnderlyingAtOpen  float64       // Stock price at entry
	UnderlyingAtClose float64       // Stock price at exit
	Legs              []st.TradeLeg // The individual option contracts involved
	OpenPremium       float64       // Net premium at entry (negative for debit, positive for credit)
	ClosePremium      float64       // Net premium at exit
	HighPremium       float64       // Maximum observed premium during trade life
	LowPremium        float64       // Minimum observed premium during trade life
	ClosedBy          string        // Label of the rule that triggered the exit
}

// Global log verbosity levels.
const (
	VerbosityError = iota // 0 - System crashes or critical data missing
	VerbosityWarn         // 1 - Non-fatal issues (e.g., using Black-Scholes instead of Market Data)
	VerbosityInfo         // 2 - High-level trade progress (Open/Close summaries)
	VerbosityDebug        // 3 - Logic flow (Why a specific exit was triggered)
	VerbosityTrace        // 4 - Minute-by-minute price data/granular math
)

// Result contains the collection of all trades generated during a backtest session.
type Result struct {
	Trades []Trade `json:"trades"`
}

// NewEngine initializes a new backtester with the provided configuration and data source.
func NewEngine(cfg *Config, prov data.Provider) *Engine {
	return &Engine{cfg: cfg, prov: prov}
}

// Run is the main entry point for the backtest. It performs data fetching,
// volatility calculation, trade scheduling, and execution loop.
func (e *Engine) Run() (*Result, error) {
	e.initConfiguration()
	logger.Infof("Starting backtest for %s | Range: %s to %s",
		e.cfg.Underlying, e.cfg.Entry.StartDate.Format("2006-01-02"), e.cfg.Entry.EndDate.Format("2006-01-02"))

	dailyBars, err := e.fetchDailyData()
	if err != nil {
		logger.Errorf("Failed to fetch underlying data: %v", err)
		return nil, err
	}

	hv := AnnualizedVolatility(extractCloses(dailyBars))
	logger.Infof("Calculated Historical Volatility: %.2f%%", hv*100)

	expiryList, err := e.prov.GetRelevantExpiries(e.cfg.Underlying, e.cfg.Entry.StartDate, e.cfg.Entry.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get expiries: %w", err)
	}

	// Scheduling
	entryDates, err := sch.ScheduleDates(e.cfg.Entry, dailyBars, expiryList)
	if err != nil || len(entryDates) == 0 {
		logger.Warnf("No entry dates scheduled for the given criteria.")
		return nil, fmt.Errorf("failed to schedule dates: %w", err)
	}

	logger.Infof("Scheduled %d potential trade entries", len(entryDates))
	trades := e.executeBacktest(entryDates, dailyBars, expiryList, hv)

	return &Result{Trades: trades}, nil
}

// executeBacktest iterates through scheduled dates to open, manage, and close trades.
func (e *Engine) executeBacktest(dates []time.Time, dailyBars []data.Bar, expiries []time.Time, hv float64) []Trade {
	var trades []Trade
	barMap := createBarMap(dailyBars)

	for i, dt := range dates {
		dateStr := dt.Format("2006-01-02")
		bar, ok := barMap[dateStr]
		if !ok {
			logger.Debugf("[%s] Skipping: No underlying bar data available", dateStr)
			continue
		}

		// Plan Strategy Legs
		legs, err := st.PlanStrategy(e.cfg.Strategy, dt, e.cfg.Underlying, bar.Close, expiries, e.prov)
		if err != nil {
			logger.Warnf("[%s] Entry Failed: Could not plan strategy legs: %v", dateStr, err)
			continue
		}

		trade := e.openTrade(i+1, dt, bar.Close, legs, hv)
		logger.Infof("[%s] OPEN Trade #%d | Spot: %.2f | Net Prem: %.2f", dateStr, trade.ID, bar.Close, trade.OpenPremium)

		// Simulate Lifecycle (Exit logic)
		simCloseTrade(&trade, dailyBars, hv, *e.cfg, e.prov)

		trades = append(trades, trade)
		e.logTradeSummary(trade)
	}
	return trades
}

// openTrade handles initial premium pricing and trade initialization.
func (e *Engine) openTrade(id int, dt time.Time, price float64, legs []st.TradeLeg, hv float64) Trade {
	openPremium := 0.0
	for _, leg := range legs {
		p := e.getOpeningPrice(leg, dt, price, hv)

		sign := 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			sign = -1.0
		}

		openPremium += sign * p * float64(leg.Spec.Qty)
	}

	return Trade{
		ID:               id,
		OpenDateTime:     dt,
		UnderlyingAtOpen: price,
		Legs:             legs,
		OpenPremium:      openPremium,
		HighPremium:      openPremium,
		LowPremium:       openPremium,
	}
}

// simCloseTrade determines the exit date and final trade value.
// It prioritizes time-based exits, then price-movement exits, and finally intraday PnL.
func simCloseTrade(tr *Trade, dailyBars []data.Bar, hv float64, cfg Config, prov data.Provider) {
	closeByDateTime := tr.Legs[0].Expiration // Default to expiration
	logger.Debugf("Trade #%d: Initial exit target set to Expiration: %s", tr.ID, closeByDateTime.Format("2006-01-02"))

	// Apply hard time limits
	if cfg.Exit.ExitByDaysToExpiry != nil && *cfg.Exit.ExitByDaysToExpiry > 0 {
		closeByDateTime = tr.Legs[0].Expiration.AddDate(0, 0, -*cfg.Exit.ExitByDaysToExpiry)
		tr.ClosedBy = "ExitByDaysToExpiry"
		logger.Debugf("Trade #%d: Exit adjusted to DTE limit: %s", tr.ID, closeByDateTime.Format("2006-01-02"))
	}

	if cfg.Exit.MaxDaysInTrade != nil && *cfg.Exit.MaxDaysInTrade > 0 {
		maxDate := tr.OpenDateTime.AddDate(0, 0, *cfg.Exit.MaxDaysInTrade)
		if maxDate.Before(closeByDateTime) {
			closeByDateTime = maxDate
			tr.ClosedBy = "ExitByMaxDaysInTrade"
			logger.Debugf("Trade #%d: Exit adjusted to Max Days limit: %s", tr.ID, closeByDateTime.Format("2006-01-02"))
		}
	}

	// Check underlying move (Coarse daily filter)
	if cfg.Exit.UnderlyingMovePx != nil {
		exitByUnderlyingMove(tr, dailyBars, &closeByDateTime, cfg)
	}

	// Check intraday price action (Fine minute filter)
	exitByPriceChange(tr, closeByDateTime, cfg, prov)
}

// exitByUnderlyingMove scans daily bars to find if the asset moved past a dollar threshold.
func exitByUnderlyingMove(trade *Trade, dailyBars []data.Bar, closeByDateTime *time.Time, cfg Config) {
	for _, bar := range dailyBars {
		if bar.Date.Before(trade.OpenDateTime) {
			continue
		}
		if bar.Date.After(*closeByDateTime) {
			break
		}

		move := max(bar.High-trade.UnderlyingAtOpen, trade.UnderlyingAtOpen-bar.Low)
		if move >= *cfg.Exit.UnderlyingMovePx {
			*closeByDateTime = bar.Date
			trade.ClosedBy = "ExitByUnderlyingMovePx"
			return
		}
	}
}

// exitByPriceChange performs high-fidelity minute-by-minute simulation for the trade exit.
func exitByPriceChange(trade *Trade, closeByDateTime time.Time, cfg Config, prov data.Provider) {
	bars, err := prov.GetBars(cfg.Underlying, trade.OpenDateTime, closeByDateTime, 1, "minute")
	if err != nil {
		logger.Errorf("Trade #%d: Data error fetching minute bars: %v", trade.ID, err)
	}

	// Precise underlying move check
	if cfg.Exit.UnderlyingMovePx != nil {
		if hitTime, movePrice, hit := checkUnderlyingMove(bars, trade, *cfg.Exit.UnderlyingMovePx); hit {
			closeByDateTime = hitTime
			trade.UnderlyingAtClose = movePrice
			trade.ClosedBy = "ExitByUnderlyingMovePx"
		}
	}

	// Profit/Stop Target checks
	if cfg.Exit.ProfitTargetPct != nil || cfg.Exit.StopLossPct != nil {
		minuteData := fetchAndAlignLegData(trade, bars, closeByDateTime, cfg, prov)
		if triggered := scanOptionExits(trade, minuteData, cfg, &closeByDateTime); triggered {
			logger.Debugf("Trade #%d: Exit triggered by PnL Target at %s", trade.ID, closeByDateTime.Format("2006-01-02 15:04"))
			trade.ClosedBy = "ExitByOptionPriceChange"
		}
	}

	if trade.ClosePremium == 0.0 {
		trade.ClosePremium = calculateFinalClosePremium(trade, closeByDateTime, cfg, prov)
	}
	trade.CloseDateTime = &closeByDateTime
}

// scanOptionExits simulates the PnL of the entire strategy at every minute to check for target triggers.
func scanOptionExits(trade *Trade, minuteData map[time.Time][]data.Bar, cfg Config, closeDate *time.Time) bool {
	legsCount := len(trade.Legs)
	legSigns := make([]float64, legsCount)
	lastPrices := make([]float64, legsCount)

	for i, leg := range trade.Legs {
		lastPrices[i] = leg.OpenPremium
		legSigns[i] = 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			legSigns[i] = -1.0
		}
	}

	validators := getValidators(cfg, trade.OpenPremium)

	// Sort keys or iterate carefully if order matters,
	// though map iteration is usually fine for "first hit" in backtests
	for ts, bars := range minuteData {
		currentTotal := 0.0
		for i := 0; i < legsCount; i++ {
			bar := bars[i+1]
			if !bar.Date.IsZero() {
				lastPrices[i] = bar.Close
			}
			currentTotal += legSigns[i] * lastPrices[i] * float64(trade.Legs[i].Spec.Qty)
		}

		for _, check := range validators {
			if check(currentTotal) {
				*closeDate = ts
				trade.ClosePremium = currentTotal
				for i := 0; i < legsCount; i++ {
					trade.Legs[i].ClosePremium = lastPrices[i]
				}
				return true
			}
		}
	}
	return false
}

// calculateFinalClosePremium determines the final value of the trade if no specific exit was triggered.
func calculateFinalClosePremium(trade *Trade, closeByDateTime time.Time, cfg Config, prov data.Provider) float64 {
	totalPremium := 0.0
	for _, leg := range trade.Legs {
		premium, err := prov.GetOptionPrice(cfg.Underlying, leg.Strike, leg.Expiration, leg.Spec.OptionType, closeByDateTime)

		if err != nil {
			logger.Debugf("fallback to Black-Scholes for %s %s at %s", cfg.Underlying, leg.Spec.OptionType, closeByDateTime)
			premium = pricing.BlackScholesPrice(trade.UnderlyingAtClose, leg.Strike,
				(leg.Expiration.Sub(closeByDateTime).Hours() / (24 * 365)), 0.02, 0.2,
				strings.ToLower(leg.Spec.OptionType) == "call")
		}

		sign := 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			sign = -1.0
		}
		totalPremium += sign * premium * float64(leg.Spec.Qty)
	}
	return totalPremium
}
