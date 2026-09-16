package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
)

func main() {
	var (
		symbolArg = flag.String("contract", "", "OCC option contract, e.g. SPY250127C00607000")
		outArg    = flag.String("out", "", "output CSV path (default: <CONTRACT>.csv in cwd)")
		fromArg   = flag.String("from", "", "optional start datetime 2006-01-02 15:04 (America/New_York)")
		toArg     = flag.String("to", "", "optional end datetime 2006-01-02 15:04 (America/New_York)")
	)
	flag.Parse()

	if *symbolArg == "" {
		logger.Fatalf("-contract is required (e.g. SPY250127C00607000)")
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		logger.Fatalf("load timezone: %v", err)
	}
	from, to, err := parseWindow(*fromArg, *toArg, loc)
	if err != nil {
		logger.Fatalf("%v", err)
	}

	contract := normalizeContract(*symbolArg)
	outPath := *outArg
	if outPath == "" {
		outPath = strings.ToUpper(strings.TrimPrefix(*symbolArg, "O:")) + ".csv"
	}

	prov, err := data.NewParquetDataProviderFromConfig(nil)
	if err != nil {
		logger.Fatalf("open parquet provider: %v", err)
	}
	defer prov.Close()

	logger.Infof("reading %s from parquet", contract)
	bars, err := prov.GetBars(contract, from, to, data.MultiplierOne, data.TimespanMinute)
	if err != nil {
		logger.Fatalf("get bars: %v", err)
	}
	if len(bars) == 0 {
		logger.Fatalf("no minute bars found for %s", contract)
	}

	if err := writeCSV(outPath, contract, bars, loc); err != nil {
		logger.Fatalf("write csv: %v", err)
	}

	logger.Infof("wrote %d bars to %s", len(bars), outPath)
}

func parseWindow(fromStr, toStr string, loc *time.Location) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error
	if fromStr != "" {
		from, err = time.ParseInLocation("2006-01-02 15:04", fromStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid -from %q: %v", fromStr, err)
		}
	}
	if toStr != "" {
		to, err = time.ParseInLocation("2006-01-02 15:04", toStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid -to %q: %v", toStr, err)
		}
	}
	return from, to, nil
}

func normalizeContract(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.HasPrefix(s, "O:") {
		return s
	}
	return "O:" + s
}

func writeCSV(path, contract string, bars []data.Bar, loc *time.Location) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	w := csv.NewWriter(out)
	defer w.Flush()

	if err := w.Write([]string{
		"contract", "timestamp", "datetime_utc", "datetime_ny",
		"open", "high", "low", "close", "volume", "transactions",
	}); err != nil {
		return err
	}

	for _, b := range bars {
		row := []string{
			contract,
			fmt.Sprintf("%d", b.Date.Unix()),
			b.Date.UTC().Format("2006-01-02 15:04:05"),
			b.Date.In(loc).Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.4f", b.Open),
			fmt.Sprintf("%.4f", b.High),
			fmt.Sprintf("%.4f", b.Low),
			fmt.Sprintf("%.4f", b.Close),
			fmt.Sprintf("%d", int64(b.Volume)),
			fmt.Sprintf("%d", b.Count),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}