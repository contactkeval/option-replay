package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
)

func TestParseMinDataCSV(t *testing.T) {
	td, titles, err := parseMinDataCSV("../../out/data_2026090200006/minData_00017.csv", 17, nil)
	if err != nil {
		t.Fatalf("parseMinDataCSV: %v", err)
	}
	if td == nil {
		t.Fatal("expected non-nil TradeData")
	}
	if td.ID != 17 {
		t.Errorf("ID = %d, want 17", td.ID)
	}
	if len(td.Timestamps) != 21 {
		t.Errorf("Timestamps = %d rows, want 21", len(td.Timestamps))
	}
	if len(td.Summary) != 5 {
		t.Errorf("Summary = %d columns, want 5", len(td.Summary))
	}
	expectedTitles := []string{"day1", "day2", "calls", "puts", "total"}
	if len(titles) != len(expectedTitles) {
		t.Fatalf("titles = %v, want %v", titles, expectedTitles)
	}
	for i, want := range expectedTitles {
		if titles[i] != want {
			t.Errorf("titles[%d] = %q, want %q", i, titles[i], want)
		}
	}

	// First row: total=2.60, base100 total=342.11
	if td.Timestamps[0] != "15:40" {
		t.Errorf("Timestamps[0] = %q, want '15:40'", td.Timestamps[0])
	}
	if td.Summary["total"][0] != 2.60 {
		t.Errorf("Summary[total][0] = %v, want 2.60", td.Summary["total"][0])
	}
	if td.Summary["day1"][0] != 0.76 {
		t.Errorf("Summary[day1][0] = %v, want 0.76", td.Summary["day1"][0])
	}

	// Third row: total=2.62
	if td.Summary["total"][2] != 2.62 {
		t.Errorf("Summary[total][2] = %v, want 2.62", td.Summary["total"][2])
	}

	// Underlying OHLC + volume parsed
	if len(td.Candles) != 21 {
		t.Fatalf("Candles = %d, want 21", len(td.Candles))
	}
	if len(td.Volume) != 21 {
		t.Fatalf("Volume = %d, want 21", len(td.Volume))
	}
	// First row underlying: Open 680.31, High 680.38, Low 680.27, Close 680.30, Volume 202216
	c0 := td.Candles[0]
	if c0.Open != 680.31 || c0.High != 680.38 || c0.Low != 680.27 || c0.Close != 680.30 {
		t.Errorf("Candles[0] = %+v, want Open 680.31 High 680.38 Low 680.27 Close 680.30", c0)
	}
	if td.Volume[0] != 202216 {
		t.Errorf("Volume[0] = %v, want 202216", td.Volume[0])
	}
}

func TestWriteHTMLReport(t *testing.T) {
	dataDir := "../../out/data_2026090200006"
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skip("test data not available")
	}

	tmpDir := t.TempDir()
	runID := "2026090200006"
	cfg := engine.ReportSpec{
		SummaryColumns: []engine.SummaryColumn{
			{Title: "day1", Columns: "leg1+leg2"},
			{Title: "day2", Columns: "leg3+leg4"},
			{Title: "calls", Columns: "leg1+leg3"},
			{Title: "puts", Columns: "leg2+leg4"},
			{Title: "total", Columns: "leg1+leg2+leg3+leg4"},
		},
	}

	err := WriteHTMLReport(nil, dataDir, tmpDir, runID, cfg)
	if err != nil {
		t.Fatalf("WriteHTMLReport: %v", err)
	}

	htmlPath := filepath.Join(tmpDir, "report_2026090200006.html")
	b, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	html := string(b)
	if !strings.Contains(html, "echarts") {
		t.Error("output missing echarts reference")
	}
	if !strings.Contains(html, "chart1") {
		t.Error("output missing chart1")
	}
	if !strings.Contains(html, "chart2") {
		t.Error("output missing chart2")
	}
	if !strings.Contains(html, `"runID":"2026090200006"`) {
		t.Error("output missing runID data")
	}
	if !strings.Contains(html, `"timestamps"`) {
		t.Error("output missing timestamps data")
	}

	t.Logf("HTML report size: %d bytes", len(b))
}
