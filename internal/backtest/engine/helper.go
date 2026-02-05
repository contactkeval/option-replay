package engine

import (
	"fmt"
	"math"
	"strings"
	"time"

	st "github.com/contactkeval/option-replay/internal/backtest/strategy"
	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pricing"
)

// initConfiguration ensures default values and logging levels are set correctly.
func (e *Engine) initConfiguration() {
	if e.cfg.ReportDir == "" {
		e.cfg.ReportDir = "./out"
	}
	if e.cfg.Seed == 0 {
		e.cfg.Seed = time.Now().UnixNano()
	}
	if e.cfg.Verbosity < VerbosityError || e.cfg.Verbosity > VerbosityTrace {
		e.cfg.Verbosity = VerbosityInfo
	}
	logger.SetVerbosity(e.cfg.Verbosity)
}

// fetchDailyData retrieves daily underlying price bars for the duration of the backtest.
func (e *Engine) fetchDailyData() ([]data.Bar, error) {
	bars, err := e.prov.GetBars(e.cfg.Underlying, e.cfg.Entry.StartDate, e.cfg.Entry.EndDate, 1, "day")
	if err != nil || len(bars) == 0 {
		return nil, fmt.Errorf("underlying data unavailable: %w", err)
	}
	return bars, nil
}

// getOpeningPrice attempts a market lookup, falling back to Black-Scholes if data is missing.
func (e *Engine) getOpeningPrice(leg st.TradeLeg, dt time.Time, underlyingPrice float64, hv float64) float64 {
	p, err := e.prov.GetOptionPrice(e.cfg.Underlying, leg.Strike, leg.Expiration, leg.Spec.OptionType, dt)

	if err != nil {
		logger.Debugf("price fallback BS: %s %s K=%.2f", e.cfg.Underlying, leg.Spec.OptionType, leg.Strike)

		yearsToMaturity := leg.Expiration.Sub(dt).Hours() / (24 * 365)
		isCall := strings.ToLower(leg.Spec.OptionType) == "call"

		p = pricing.BlackScholesPrice(underlyingPrice, leg.Strike, yearsToMaturity, 0.02, hv, isCall)
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
func (e *Engine) logTradeSummary(t Trade) {
	pnl := t.ClosePremium - t.OpenPremium
	pnlPct := 0.0
	if t.OpenPremium != 0 {
		pnlPct = (pnl / math.Abs(t.OpenPremium)) * 100
	}

	status := "🔴"
	if pnl > 0 {
		status = "🟢"
	}

	logger.Infof("%s Trade #%d Closed | Exit: %-20s | PnL: %8.2f (%6.2f%%)",
		status, t.ID, t.ClosedBy, pnl, pnlPct)
}

// checkUnderlyingMove scans minute bars to find the exact timestamp of a price breach.
func checkUnderlyingMove(bars []data.Bar, trade *Trade, limit float64) (time.Time, float64, bool) {
	for _, minuteBar := range bars {
		move := max(minuteBar.High-trade.UnderlyingAtOpen, trade.UnderlyingAtOpen-minuteBar.Low)
		if move >= limit {
			return minuteBar.Date, minuteBar.Close, true
		}
	}
	return time.Time{}, 0, false
}

// fetchAndAlignLegData synchronizes time series for the underlying and all option legs.
func fetchAndAlignLegData(trade *Trade, underlyingBars []data.Bar, end time.Time, cfg Config, prov data.Provider) map[time.Time][]data.Bar {
	legsCount := len(trade.Legs)
	minuteData := make(map[time.Time][]data.Bar)

	for _, bar := range underlyingBars {
		minuteData[bar.Date] = make([]data.Bar, legsCount+1)
		minuteData[bar.Date][0] = bar
	}

	for i, leg := range trade.Legs {
		symbol := data.OptionSymbolFromParts(cfg.Underlying, leg.Expiration, leg.Spec.OptionType, leg.Strike)
		bars, _ := prov.GetBars(symbol, trade.OpenDateTime, end, 1, "minute")

		for _, b := range bars {
			if _, exists := minuteData[b.Date]; exists {
				minuteData[b.Date][i+1] = b
			}
		}
	}
	return minuteData
}

// getValidators returns a slice of boolean functions used to evaluate exit criteria.
func getValidators(cfg Config, openPremium float64) []func(float64) bool {
	var v []func(float64) bool
	if cfg.Exit.StopLossPct != nil {
		stopAmt := openPremium * (*cfg.Exit.StopLossPct / 100.0)
		v = append(v, func(curr float64) bool { return curr <= openPremium-stopAmt })
	}
	if cfg.Exit.ProfitTargetPct != nil {
		profAmt := openPremium * (*cfg.Exit.ProfitTargetPct / 100.0)
		v = append(v, func(curr float64) bool { return curr >= openPremium+profAmt })
	}
	return v
}

// extractCloses transforms a slice of data bars into a simple slice of close prices.
func extractCloses(bars []data.Bar) []float64 {
	var closes []float64
	for _, b := range bars {
		closes = append(closes, b.Close)
	}
	return closes
}

// TODO: move to a data.BlackScholes package
// AnnualizedVolatility calculates the standard deviation of logarithmic returns normalized for a 252-day trading year.
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
