package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
)

func WriteJSON(res *engine.Result, outdir string) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outdir, "trades.json"), b, 0644)
}

func WriteCSV(trades []engine.Trade, outdir string) error {
	f, err := os.Create(filepath.Join(outdir, "trades.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	headers := []string{"id", "open_time", "open_underlying", "open_premium", "close_time", "close_underlying", "close_premium", "pnl", "strategy_high", "strategy_low", "closed_by", "legs_json"}
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, t := range trades {
		closeTime := ""
		if t.CloseDateTime != nil {
			closeTime = t.CloseDateTime.Format("2006-01-02")
		}
		pnl := t.ClosePremium - t.OpenPremium
		legsJson, _ := json.Marshal(t.Legs)
		row := []string{fmt.Sprintf("%d", t.ID), t.OpenDateTime.Format("2006-01-02"), fmt.Sprintf("%.2f", t.UnderlyingAtOpen), fmt.Sprintf("%.2f", t.OpenPremium), closeTime, fmt.Sprintf("%.2f", t.UnderlyingAtClose), fmt.Sprintf("%.2f", t.ClosePremium), fmt.Sprintf("%.2f", pnl), fmt.Sprintf("%.2f", t.HighPremium), fmt.Sprintf("%.2f", t.LowPremium), t.ClosedBy, string(legsJson)}
		_ = w.Write(row)
	}
	return nil
}
