package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
	"github.com/contactkeval/option-replay/internal/logger"
)

// TradeData holds the parsed data for one trade's minData CSV.
type TradeData struct {
	ID         int                  `json:"id"`
	PnL        float64              `json:"pnl"`
	Timestamps []string             `json:"timestamps"`
	Summary    map[string][]float64 `json:"summary"`
	Candles    []Candle             `json:"candles"`
	Volume     []float64            `json:"volume"`
}

// Candle is a single OHLC bar for the underlying.
type Candle struct {
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

// ReportData is the top-level JSON payload embedded in the HTML.
type ReportData struct {
	RunID          string            `json:"runID"`
	SummaryTitles  []string          `json:"summaryTitles"`
	Trades         []TradeData       `json:"trades"`
	Timestamps     []string          `json:"timestamps"`
}

// WriteHTMLReport reads all minData CSVs from dataDir and generates a
// self-contained ECharts HTML report in outdir.
func WriteHTMLReport(trades []engine.Trade, dataDir, outDir, runID string, reportCfg engine.ReportSpec) error {
	if !hasSummaryColumns(reportCfg) {
		return nil
	}

	reportData, err := BuildReportData(trades, dataDir, runID)
	if err != nil {
		return fmt.Errorf("building report data: %w", err)
	}

	jsonBytes, err := json.Marshal(reportData)
	if err != nil {
		return fmt.Errorf("marshaling report data: %w", err)
	}

	htmlPath := filepath.Join(outDir, fmt.Sprintf("report_%s.html", runID))
	f, err := os.Create(htmlPath)
	if err != nil {
		return fmt.Errorf("creating report file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(strings.Replace(htmlTemplate, "%s", string(jsonBytes), 1))
	if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	logger.Infof("html report written to %s", htmlPath)
	return nil
}

func hasSummaryColumns(cfg engine.ReportSpec) bool {
	return len(cfg.SummaryColumns) > 0
}

// BuildReportData scans all minData CSVs in dataDir and returns the parsed
// report payload (trades, timestamps, summary column titles).
func BuildReportData(trades []engine.Trade, dataDir, runID string) (*ReportData, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("reading data dir: %w", err)
	}

	var csvFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "minData_") && strings.HasSuffix(e.Name(), ".csv") {
			csvFiles = append(csvFiles, e.Name())
		}
	}
	sort.Strings(csvFiles)

	reportData := &ReportData{
		RunID:  runID,
		Trades: make([]TradeData, 0, len(csvFiles)),
	}

	for i, name := range csvFiles {
		td, titles, err := parseMinDataCSV(filepath.Join(dataDir, name), i+1, trades)
		if err != nil {
			logger.Warnf("skipping %s: %v", name, err)
			continue
		}
		if td != nil {
			reportData.Trades = append(reportData.Trades, *td)
			if len(reportData.SummaryTitles) == 0 && len(titles) > 0 {
				reportData.SummaryTitles = titles
			}
		}
	}

	if len(reportData.Trades) > 0 {
		reportData.Timestamps = reportData.Trades[0].Timestamps
	}

	return reportData, nil
}

func parseMinDataCSV(path string, tradeIndex int, trades []engine.Trade) (*TradeData, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("reading csv: %w", err)
	}
	if len(records) < 3 {
		return nil, nil, fmt.Errorf("not enough rows (%d)", len(records))
	}

	header1 := records[0]
	header2 := records[1]

	summaryStart := findSectionStart(header1, "Summary")
	if summaryStart < 0 {
		return nil, nil, fmt.Errorf("summary section not found")
	}

	summaryCols, titles := findSummaryColumns(header2, summaryStart)
	if len(summaryCols) == 0 {
		return nil, nil, fmt.Errorf("no summary columns found")
	}

	undStart := findSectionStart(header1, "Underlying")
	var undCols []summaryCol
	if undStart < 0 {
		return nil, nil, fmt.Errorf("underlying section not found")
	}
	undCols, _ = findSummaryColumns(header2, undStart)

	var pnl float64
	if tradeIndex <= len(trades) {
		t := trades[tradeIndex-1]
		pnl = t.ClosePremium - t.OpenPremium
	}

	td := &TradeData{
		ID:         tradeIndex,
		PnL:        pnl,
		Timestamps: make([]string, 0, len(records)-2),
		Summary:    make(map[string][]float64, len(titles)),
		Candles:    make([]Candle, 0, len(records)-2),
		Volume:     make([]float64, 0, len(records)-2),
	}
	for _, title := range titles {
		td.Summary[title] = make([]float64, 0, len(records)-2)
	}
	undIdx := map[string]int{}
	for _, uc := range undCols {
		undIdx[uc.title] = uc.idx
	}

	for _, row := range records[2:] {
		if len(row) == 0 {
			continue
		}
		ts := extractTimestamp(row[0])
		td.Timestamps = append(td.Timestamps, ts)
		for _, sc := range summaryCols {
			td.Summary[sc.title] = append(td.Summary[sc.title], parseFloat(row, sc.idx))
		}
		td.Candles = append(td.Candles, Candle{
			Open:  parseFloat(row, undIdx["Open"]),
			High:  parseFloat(row, undIdx["High"]),
			Low:   parseFloat(row, undIdx["Low"]),
			Close: parseFloat(row, undIdx["Close"]),
		})
		td.Volume = append(td.Volume, parseFloat(row, undIdx["Volume"]))
	}

	return td, titles, nil
}

// findSectionStart locates the column index where a named section begins in header row 1.
func findSectionStart(header []string, name string) int {
	for i, h := range header {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	return -1
}

type summaryCol struct {
	title string
	idx   int
}

// findSummaryColumns finds all non-empty sub-columns in a section starting at sectionStart.
func findSummaryColumns(header2 []string, sectionStart int) ([]summaryCol, []string) {
	var cols []summaryCol
	var titles []string
	for i := sectionStart; i < len(header2); i++ {
		t := strings.TrimSpace(header2[i])
		if t == "" {
			break
		}
		cols = append(cols, summaryCol{title: t, idx: i})
		titles = append(titles, t)
	}
	return cols, titles
}

func extractTimestamp(raw string) string {
	// Raw format: "2025-12-19 15:40" — extract time part "15:40"
	parts := strings.SplitN(strings.TrimSpace(raw), " ", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return strings.TrimSpace(raw)
}

func parseFloat(row []string, idx int) float64 {
	if idx >= len(row) {
		return 0
	}
	s := strings.TrimSpace(row[idx])
	s = strings.Trim(s, "\"")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
