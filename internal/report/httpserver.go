package report

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/contactkeval/option-replay/internal/backtest/engine"
)

// WriteServerHTML writes the reusable report page (no embedded data) to the
// given path. The page fetches /api/data at load time.
func WriteServerHTML(dst string) error {
	if err := os.WriteFile(dst, []byte(serverHTML()), 0644); err != nil {
		return fmt.Errorf("writing server report: %w", err)
	}
	return nil
}

// serverInitSnippet replaces initApp(%s) in the embedded template, fetching the
// report data from the local /api/data endpoint instead of embedding it.
const serverInitSnippet = `fetch('/api/data')
    .then(function(r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
    })
    .then(initApp)
    .catch(function(err) {
        document.body.innerHTML =
            '<div style="font-family:sans-serif;color:#f66;padding:40px;text-align:center">' +
            'Failed to load report data: ' + err.message +
            '</div>';
    });`

func serverHTML() string {
	return strings.Replace(htmlTemplate, "initApp(%s);", serverInitSnippet, 1)
}

// latestDataDir returns the most recently named data_* directory under outdir,
// choosing the lexicographically greatest runID (YYYYMMDDNNNNN ordering).
func latestDataDir(outdir string) (string, string, error) {
	entries, err := os.ReadDir(outdir)
	if err != nil {
		return "", "", err
	}
	var runs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "data_") {
			runs = append(runs, strings.TrimPrefix(e.Name(), "data_"))
		}
	}
	if len(runs) == 0 {
		return "", "", fmt.Errorf("no data_* directories found in %s", outdir)
	}
	sort.Strings(runs)
	runID := runs[len(runs)-1]
	return filepath.Join(outdir, "data_"+runID), runID, nil
}

// ReportHandler builds an http.Handler serving the reusable report page and its
// data endpoint. It reads the most recent data_* directory under outdir.
func ReportHandler(outdir string, trades []engine.Trade, reportCfg engine.ReportSpec) (http.Handler, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/report.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(serverHTML()))
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		dataDir, runID, err := latestDataDir(outdir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		data, err := BuildReportData(trades, dataDir, runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	})

	return mux, nil
}

// StartReportServer is provided for callers that prefer to serve directly
// without building their own handler; it blocks until the server exits.
func StartReportServer(addr, outdir string, trades []engine.Trade, reportCfg engine.ReportSpec) error {
	handler, err := ReportHandler(outdir, trades, reportCfg)
	if err != nil {
		return err
	}
	return http.ListenAndServe(addr, handler)
}
