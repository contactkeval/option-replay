package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
)

func WriteJSON(res *engine.Result, outdir, runID string) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outdir, tradesFilename("json", runID)), b, 0644)
}

func WriteCSV(trades []engine.Trade, outdir, runID, timezone string) error {
	f, err := os.Create(filepath.Join(outdir, tradesFilename("csv", runID)))
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
	loc := reportLocation(timezone)
	for _, t := range trades {
		closeTime := ""
		if t.CloseDateTime != nil {
			closeTime = formatReportTime(*t.CloseDateTime, loc)
		}
		pnl := t.ClosePremium - t.OpenPremium
		legsJson, _ := json.Marshal(t.Legs)
		row := []string{fmt.Sprintf("%d", t.ID), formatReportTime(t.OpenDateTime, loc), fmt.Sprintf("%.2f", t.UnderlyingAtOpen), fmt.Sprintf("%.2f", t.OpenPremium), closeTime, fmt.Sprintf("%.2f", t.UnderlyingAtClose), fmt.Sprintf("%.2f", t.ClosePremium), fmt.Sprintf("%.2f", pnl), fmt.Sprintf("%.2f", t.HighPremium), fmt.Sprintf("%.2f", t.LowPremium), t.ClosedBy, string(legsJson)}
		_ = w.Write(row)
	}
	return nil
}

const reportTimeLayout = "2006-01-02 15:04"

func reportLocation(timezone string) *time.Location {
	if timezone == "" {
		timezone = "America/New_York"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func formatReportTime(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	return t.In(loc).Format(reportTimeLayout)
}

func tradesFilename(ext, runID string) string {
	if runID == "" {
		return "trades." + ext
	}
	return fmt.Sprintf("trades_%s.%s", runID, ext)
}

// NextRunID returns YYYYMMDD plus a 5-digit sequence unique in outdir for today.
// It inspects existing trades_*.csv, trades_*.json, and data_* folders so all three share the same suffix.
func NextRunID(outdir string) (string, error) {
	return nextRunID(outdir, time.Now())
}

func nextRunID(outdir string, now time.Time) (string, error) {
	day := now.Format("20060102")
	entries, err := os.ReadDir(outdir)
	if err != nil {
		if os.IsNotExist(err) {
			return day + "00001", nil
		}
		return "", err
	}

	maxSeq := 0
	tradesPrefix := "trades_" + day
	dataPrefix := "data_" + day
	for _, entry := range entries {
		name := entry.Name()
		var seqStr string
		switch {
		case strings.HasPrefix(name, tradesPrefix):
			seqStr = strings.TrimPrefix(name, tradesPrefix)
			seqStr = strings.TrimSuffix(seqStr, filepath.Ext(seqStr))
		case strings.HasPrefix(name, dataPrefix):
			seqStr = strings.TrimPrefix(name, dataPrefix)
		default:
			continue
		}
		if len(seqStr) != 5 {
			continue
		}
		n, convErr := strconv.Atoi(seqStr)
		if convErr != nil {
			continue
		}
		if n > maxSeq {
			maxSeq = n
		}
	}

	return fmt.Sprintf("%s%05d", day, maxSeq+1), nil
}
