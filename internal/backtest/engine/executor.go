// Package engine provides the core simulation logic for backtesting option strategies.
// It handles trade scheduling, entry execution, and lifecycle management via price action and exit conditions.
package engine

import (
	"fmt"
	"strings"
	"time"

	sch "github.com/contactkeval/option-replay/internal/backtest/sequence"
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

// Result contains the collection of all trades generated during a backtest session.
type Result struct {
	Trades []Trade `json:"trades"`
}

// MinuteRow represents a single "slice" of market time across all instruments.
type MinuteRow struct {
	Timestamp time.Time
	// LegBars stores bars for all legs.
	// Indices 0 to N-1 are option legs, Index N is the underlying.
	LegBars []data.Bar
}

// Global log verbosity levels.
const (
	VerbosityError = iota // 0 - System crashes or critical data missing
	VerbosityWarn         // 1 - Non-fatal issues (e.g., using Black-Scholes instead of Market Data)
	VerbosityInfo         // 2 - High-level trade progress (Open/Close summaries)
	VerbosityDebug        // 3 - Logic flow (Why a specific exit was triggered)
	VerbosityTrace        // 4 - Minute-by-minute price data/granular math

	CloseByDTE        = "ExitByDaysToExpiry"
	CloseByMaxDays    = "ExitByMaxDaysInTrade"
	CloseByUnderlying = "ExitByUnderlyingMovePx"
	CloseByPnL        = "ExitByOptionPriceChange"

	multiplierOne  = 1
	timespanDay    = "day"
	timespanMinute = "minute"
)

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

	histVol := data.AnnualizedVolatility(data.ExtractCloses(dailyBars))
	logger.Infof("Calculated Historical Volatility: %.2f%%", histVol*100)

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
	trades := e.executeBacktest(entryDates, dailyBars, expiryList, histVol)

	return &Result{Trades: trades}, nil
}

// executeBacktest iterates through scheduled dates to open, manage, and close trades.
func (e *Engine) executeBacktest(
	entryDates []time.Time,
	dailyBars []data.Bar,
	expiryList []time.Time,
	histVol float64,
) []Trade {
	var trades []Trade
	barMap := createBarMap(dailyBars)

	for i, entryDate := range entryDates {
		entryDateStr := entryDate.Format("2006-01-02")
		bar, ok := barMap[entryDateStr]
		if !ok {
			logger.Debugf("[%s] Skipping: No underlying bar data available", entryDateStr)
			continue
		}

		// Plan Strategy Legs
		legs, err := st.PlanStrategy(e.cfg.Strategy, entryDate, e.cfg.Underlying, bar.Close, expiryList, e.prov)
		if err != nil {
			logger.Warnf("[%s] Entry Failed: Could not plan strategy legs: %v", entryDateStr, err)
			continue
		}

		trade := e.openTrade(i+1, entryDate, bar.Close, legs, histVol)
		logger.Infof("[%s] OPEN Trade #%d | Spot: %.2f | Net Prem: %.2f", entryDateStr, trade.ID, bar.Close, trade.OpenPremium)

		// Simulate Lifecycle (Exit logic)
		simulatedCloseTrade(&trade, dailyBars, *e.cfg, e.prov)

		trades = append(trades, trade)
		e.logTradeSummary(trade)
	}
	return trades
}

// openTrade handles initial premium pricing and trade initialization.
func (e *Engine) openTrade(
	id int,
	entryDate time.Time,
	price float64,
	legs []st.TradeLeg,
	histVol float64,
) Trade {
	openPremium := 0.0
	for _, leg := range legs {
		p := e.getOpeningPrice(leg, entryDate, price, histVol)

		sign := 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			sign = -1.0
		}

		openPremium += sign * p * float64(leg.Spec.Qty)
	}

	return Trade{
		ID:               id,
		OpenDateTime:     entryDate,
		UnderlyingAtOpen: price,
		Legs:             legs,
		OpenPremium:      openPremium,
		HighPremium:      openPremium,
		LowPremium:       openPremium,
	}
}

// simulatedCloseTrade determines the exit date and final trade value.
// It prioritizes time-based exits, then price-movement exits, and finally intraday PnL.
func simulatedCloseTrade(
	trade *Trade,
	dailyBars []data.Bar,
	cfg Config,
	prov data.Provider,
) {
	closeByDateTime := trade.Legs[0].Expiration // Default to expiration
	logger.Debugf("Trade #%d: Initial exit target set to Expiration: %s", trade.ID, closeByDateTime.Format("2006-01-02"))

	// Apply hard time limits
	if cfg.Exit.ExitByDaysToExpiry != nil && *cfg.Exit.ExitByDaysToExpiry > 0 {
		closeByDateTime = trade.Legs[0].Expiration.AddDate(0, 0, -*cfg.Exit.ExitByDaysToExpiry)
		trade.ClosedBy = CloseByDTE
		logger.Debugf("Trade #%d: Exit adjusted to DTE limit: %s", trade.ID, closeByDateTime.Format("2006-01-02"))
	}

	if cfg.Exit.MaxDaysInTrade != nil && *cfg.Exit.MaxDaysInTrade > 0 {
		maxDate := trade.OpenDateTime.AddDate(0, 0, *cfg.Exit.MaxDaysInTrade)
		if maxDate.Before(closeByDateTime) {
			closeByDateTime = maxDate
			trade.ClosedBy = CloseByMaxDays
			logger.Debugf("Trade #%d: Exit adjusted to Max Days limit: %s", trade.ID, closeByDateTime.Format("2006-01-02"))
		}
	}

	// Check underlying move (Coarse daily filter)
	if cfg.Exit.UnderlyingMovePx != nil {
		exitByUnderlyingMove(trade, dailyBars, &closeByDateTime, cfg)
	}

	// Check intraday price action (Fine minute filter)
	exitByPriceChange(trade, closeByDateTime, cfg, prov)
}

// exitByUnderlyingMove scans daily bars to find if the asset moved past a dollar threshold.
func exitByUnderlyingMove(
	trade *Trade,
	dailyBars []data.Bar,
	closeByDateTime *time.Time,
	cfg Config,
) {
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
			trade.ClosedBy = CloseByUnderlying
			return
		}
	}
}

// exitByPriceChange performs high-fidelity minute-by-minute simulation for the trade exit.
func exitByPriceChange(
	trade *Trade,
	closeByDateTime time.Time,
	cfg Config,
	prov data.Provider,
) {
	underlyingBars, err := prov.GetBars(cfg.Underlying, trade.OpenDateTime, closeByDateTime, multiplierOne, timespanMinute)
	if err != nil {
		logger.Errorf("Trade #%d: Data error fetching minute bars: %v", trade.ID, err)
	}

	// Precise underlying move check
	if cfg.Exit.UnderlyingMovePx != nil {
		if hitTime, movePrice, hit := checkUnderlyingMove(underlyingBars, trade, *cfg.Exit.UnderlyingMovePx); hit {
			closeByDateTime = hitTime
			trade.UnderlyingAtClose = movePrice
			trade.ClosedBy = CloseByUnderlying
		}
	}

	// Profit/Stop Target checks
	if cfg.Exit.ProfitTargetPct != nil || cfg.Exit.StopLossPct != nil {
		minuteData := fetchAndAlignLegData(trade, underlyingBars, closeByDateTime, prov, cfg)
		if triggered := scanOptionExits(trade, minuteData, &closeByDateTime, cfg); triggered {
			logger.Debugf("Trade #%d: Exit triggered by PnL Target at %s", trade.ID, closeByDateTime.Format("2006-01-02 15:04"))
			trade.ClosedBy = CloseByPnL
		}
	}

	// If scanOptionExits didn't set a close premium, calculate final value at closeByDateTime
	if trade.ClosePremium == 0.0 {
		trade.CloseDateTime = &closeByDateTime
		trade.ClosePremium = calculateFinalClosePremium(trade, closeByDateTime, cfg, prov)
		if trade.UnderlyingAtClose == 0 {
			trade.UnderlyingAtClose = underlyingBars[len(underlyingBars)-1].Close // fallback to last known price
		}
	}
}

// scanOptionExits simulates the PnL of the entire strategy minute-by-minute.
// It returns true and updates the trade if an exit trigger (SL/TP) is hit.
func scanOptionExits(
	trade *Trade,
	minuteData []MinuteRow, // Changed from map to slice for chronological (sorted) order
	closeByDateTime *time.Time,
	cfg Config,
) bool {
	legsCount := len(trade.Legs)
	legSigns := make([]float64, legsCount)
	lastPrices := make([]float64, legsCount)

	// Pre-calculate signs and initialize prices with entry premiums
	for i, leg := range trade.Legs {
		lastPrices[i] = leg.OpenPremium
		legSigns[i] = 1.0
		if strings.EqualFold(leg.Spec.Side, "sell") {
			legSigns[i] = -1.0
		}
	}

	validators := getValidators(cfg, trade.OpenPremium)

	// Process the timeline in linear order (critical for path-dependent exits)
	for _, row := range minuteData {
		currentTotal := 0.0

		for i := 0; i < legsCount; i++ {
			// Gap Filling: If the bar for this minute exists, update the last price.
			// If row.LegBars[i].Date is Zero, we carry forward lastPrices[i] (no-op).
			if !row.LegBars[i].Date.IsZero() {
				lastPrices[i] = row.LegBars[i].Close
			}

			// Calculate contribution to strategy premium
			currentTotal += legSigns[i] * lastPrices[i] * float64(trade.Legs[i].Spec.Qty)
		}

		// Check all exit conditions (Target, Stop Loss, etc.)
		for _, validator := range validators {
			if validator(currentTotal) {
				// Record the exit event
				*closeByDateTime = row.Timestamp
				trade.CloseDateTime = &row.Timestamp
				trade.ClosePremium = currentTotal

				// Underlying bar is stored at the last index (N)
				trade.UnderlyingAtClose = row.LegBars[legsCount].Close

				// Update individual leg close premiums for audit/reporting
				for i := 0; i < legsCount; i++ {
					trade.Legs[i].ClosePremium = lastPrices[i]
				}

				logger.Infof("Exit triggered | time=%s | premium=%.2f",
					row.Timestamp.Format("2006-01-02 15:04"), currentTotal)

				return true
			}
		}
	}

	return false
}

// calculateFinalClosePremium determines the final value of the trade if no specific exit was triggered.
func calculateFinalClosePremium(
	trade *Trade,
	closeByDateTime time.Time,
	cfg Config,
	prov data.Provider,
) float64 {
	totalPremium := 0.0

	for i, leg := range trade.Legs {
		premium, err := prov.GetOptionPrice(cfg.Underlying, leg.Strike, leg.Expiration, leg.Spec.OptionType, closeByDateTime)
		if err != nil {
			logger.Debugf("fallback to Black-Scholes for %s %s at %s", cfg.Underlying, leg.Spec.OptionType, closeByDateTime)
			premium = pricing.BlackScholesPrice(trade.UnderlyingAtClose, leg.Strike,
				(leg.Expiration.Sub(closeByDateTime).Hours() / (24 * 365)), 0.02, 0.2,
				strings.ToLower(leg.Spec.OptionType) == "call")
		}
		trade.Legs[i].ClosePremium = premium

		sign := 1.0
		if strings.ToLower(leg.Spec.Side) == "sell" {
			sign = -1.0
		}
		totalPremium += sign * premium * float64(leg.Spec.Qty)
	}
	trade.ClosePremium = totalPremium

	return totalPremium
}
