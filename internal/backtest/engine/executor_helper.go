package engine

import (
	"fmt"
	"math"
	"strings"
	"time"

	sch "github.com/contactkeval/option-replay/internal/backtest/sequence"
	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pricing"
)

// initConfiguration ensures default values and logging levels are set correctly.
func (e *Engine) initConfiguration() {
	logger.SetVerbosity(e.cfg.Verbosity)
	if e.cfg.ReportDir == "" {
		e.cfg.ReportDir = "./out"
	}
	if e.cfg.Seed == 0 {
		e.cfg.Seed = time.Now().UnixNano()
	}
	if e.cfg.Verbosity < VerbosityError || e.cfg.Verbosity > VerbosityTrace {
		e.cfg.Verbosity = VerbosityInfo
	}
	if e.cfg.Entry.StDt == "" {
		e.cfg.Entry.StDt = time.Now().AddDate(-1, 0, 0).Format("2006-01-02") // default to 1 month ago
	}
	if e.cfg.Entry.EnDt == "" {
		e.cfg.Entry.EnDt = time.Now().Format("2006-01-02") // default to today
	}
	if e.cfg.Entry.Mode == "" {
		e.cfg.Entry.Mode = "daily_time"
	}
	if e.cfg.Entry.TimeOfDay == "" {
		e.cfg.Entry.TimeOfDay = "9:45"
	}
	if e.cfg.ExpiryTime == "" {
		e.cfg.ExpiryTime = "16:00"
	}
	if e.cfg.Entry.Timezone == "" {
		e.cfg.Entry.Timezone = "America/New_York"
	}

	// Parse and combine entry date/time into time.Time objects
	day, err := time.Parse("2006-01-02", e.cfg.Entry.StDt)
	if err != nil {
		logger.Errorf("invalid start date format (YYYY-MM-DD): %v", err)
	}
	e.cfg.Entry.StartDate, _ = sch.CombineDateTime(day, e.cfg.Entry.TimeOfDay, e.cfg.Entry.Timezone)
	day, err = time.Parse("2006-01-02", e.cfg.Entry.EnDt)
	if err != nil {
		logger.Errorf("invalid end date format (YYYY-MM-DD): %v", err)
	}
	day = day.Add(time.Duration(24*time.Hour) - time.Minute)
	e.cfg.Entry.EndDate, _ = sch.CombineDateTime(day, day.Format("15:04"), e.cfg.Entry.Timezone)
}

// fetchDailyData retrieves daily underlying price bars for the duration of the backtest.
func (e *Engine) fetchDailyData() ([]data.Bar, error) {
	bars, err := e.dataProv.GetBars(e.cfg.Underlying, e.cfg.Entry.StartDate, e.cfg.Entry.EndDate, multiplierOne, timespanDay)
	if err != nil || len(bars) == 0 {
		return nil, fmt.Errorf("underlying data unavailable: %w", err)
	}
	return bars, nil
}

// getOpeningPrice attempts a market lookup, falling back to Black-Scholes if data is missing.
func (e *Engine) getOpeningPrice(
	leg st.TradeLeg,
	dt time.Time,
	underlyingPrice float64,
	histVol float64,
) float64 {
	p, err := e.dataProv.GetOptionPrice(e.cfg.Underlying, leg.Strike, leg.Expiration, leg.Spec.OptionType, dt)

	if err != nil {
		logger.Debugf("price fallback BS: %s %s K=%.2f", e.cfg.Underlying, leg.Spec.OptionType, leg.Strike)

		yearsToMaturity := leg.Expiration.Sub(dt).Hours() / (24 * 365)
		isCall := strings.ToLower(leg.Spec.OptionType) == "call"

		p = pricing.BlackScholesPrice(underlyingPrice, leg.Strike, yearsToMaturity, 0.02, histVol, isCall)
	}

	return p
}

// createBarMap optimizes lookup by converting a slice of bars to a date-keyed map.
func createBarMap(bars []data.Bar) map[string]data.Bar {
	m := make(map[string]data.Bar, len(bars))
	for _, b := range bars {
		m[b.Date.Format("2006-01-02")] = b
	}
	return m
}

// logTradeSummary provides formatted console output of trade results.
func logTradeSummary(trade Trade) {
	pnl := trade.ClosePremium - trade.OpenPremium
	pnlPct := 0.0
	if trade.OpenPremium != 0 {
		pnlPct = (pnl / math.Abs(trade.OpenPremium)) * 100
	}

	status := "🔴"
	if pnl > 0 {
		status = "🟢"
	}

	logger.Infof("%s Trade #%d Closed | Exit: %-20s | PnL: %8.2f (%6.2f%%)", status, trade.ID, trade.ClosedBy, pnl, pnlPct)
}

// checkUnderlyingMove scans minute bars to find the exact timestamp of a price breach.
func checkUnderlyingMove(
	bars []data.Bar,
	trade *Trade,
	limit float64,
) (time.Time, float64, bool) {
	for _, minuteBar := range bars {
		move := max(minuteBar.High-trade.UnderlyingAtOpen, trade.UnderlyingAtOpen-minuteBar.Low)
		if move >= limit {
			return minuteBar.Date, minuteBar.Close, true
		}
	}
	return time.Time{}, 0, false
}

// fetchAndAlignLegData synchronizes time series for the underlying and all option legs.
func fetchAndAlignLegData(
	trade *Trade,
	underlyingBars []data.Bar,
	closeByDateTime time.Time,
	dataProv data.Provider,
	cfg Config,
) []MinuteRow {
	legsCount := len(trade.Legs)

	// We use a temporary map for the 'Alignment' phase.
	// This handles cases where leg data might have gaps or different start times.
	alignmentMap := make(map[time.Time][]data.Bar)

	// Track timestamps in a slice to preserve the order of underlyingBars
	timestamps := make([]time.Time, 0, len(underlyingBars))

	// 1. Initialize alignment map with Underlying data
	for _, bar := range underlyingBars {
		row := make([]data.Bar, legsCount+1)
		row[legsCount] = bar
		alignmentMap[bar.Date] = row
		timestamps = append(timestamps, bar.Date)
	}

	// 2. Overlay Leg data into the alignment map
	for i, leg := range trade.Legs {
		symbol := dataProv.OptionSymbolFromParts(cfg.Underlying, leg.Expiration, leg.Spec.OptionType, leg.Strike)

		// Note: We ignore errors here assuming gaps result in zero-value bars
		bars, _ := dataProv.GetBars(symbol, trade.OpenDateTime, closeByDateTime, multiplierOne, timespanMinute)

		for _, bar := range bars {
			if row, exists := alignmentMap[bar.Date]; exists {
				row[i] = bar
			}
		}
	}

	// 3. Convert map to a sorted slice (The 'Flattening' phase)
	// Since 'timestamps' was built from 'underlyingBars', it is already sorted.
	minuteData := make([]MinuteRow, 0, len(timestamps))
	for _, ts := range timestamps {
		minuteData = append(minuteData, MinuteRow{
			Timestamp: ts,
			LegBars:   alignmentMap[ts],
		})
	}

	return minuteData
}

// getValidators returns a slice of boolean functions used to evaluate exit criteria.
func getValidators(cfg Config, currentPremium float64) []func(float64) bool {
	var v []func(float64) bool
	if cfg.Exit.StopLossPct != nil {
		stopAmt := currentPremium * (*cfg.Exit.StopLossPct / 100.0)
		v = append(v, func(curr float64) bool { return curr <= currentPremium-stopAmt })
	}
	if cfg.Exit.ProfitTargetPct != nil {
		profAmt := currentPremium * (*cfg.Exit.ProfitTargetPct / 100.0)
		v = append(v, func(curr float64) bool { return curr >= currentPremium+profAmt })
	}
	return v
}
