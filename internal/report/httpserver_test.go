package report

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
)

func testReportCfg() engine.ReportSpec {
	return engine.ReportSpec{
		SummaryColumns: []engine.SummaryColumn{
			{Title: "day1", Columns: "leg1+leg2"},
			{Title: "day2", Columns: "leg3+leg4"},
			{Title: "calls", Columns: "leg1+leg3"},
			{Title: "puts", Columns: "leg2+leg4"},
			{Title: "total", Columns: "leg1+leg2+leg3+leg4"},
		},
	}
}

func TestReportHandler(t *testing.T) {
	outdir := "../../out"
	if _, err := os.Stat(filepath.Join(outdir, "data_2026090200006")); os.IsNotExist(err) {
		t.Skip("test data not available")
	}

	handler, err := ReportHandler(outdir, nil, testReportCfg())
	if err != nil {
		t.Fatalf("ReportHandler: %v", err)
	}

	// Page
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/api/data") {
		t.Error("page missing /api/data fetch")
	}
	if strings.Contains(rec.Body.String(), "initApp(%s)") {
		t.Error("page still has embedded data placeholder")
	}

	// Data endpoint
	req = httptest.NewRequest("GET", "/api/data", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET /api/data status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	var payload ReportData
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshaling payload: %v", err)
	}
	if len(payload.Trades) == 0 {
		t.Fatal("no trades parsed from CSVs")
	}
	if len(payload.SummaryTitles) == 0 {
		t.Fatal("no summary titles parsed")
	}
	if len(payload.Trades[0].Candles) == 0 {
		t.Fatal("trades missing candles")
	}
	if len(payload.Trades[0].Volume) == 0 {
		t.Fatal("trades missing volume")
	}

	// 404 for unknown paths
	req = httptest.NewRequest("GET", "/nope", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("GET /nope status = %d, want 404", rec.Code)
	}
}
