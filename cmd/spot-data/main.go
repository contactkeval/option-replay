package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func main() {
	underlyingsFlag := flag.String(
		"underlyings",
		"",
		"path to allowed underlyings file (default: internal/pipeline/config/allowed_underlyings.txt)",
	)
	dataDirFlag := flag.String(
		"data-dir",
		"input/data",
		"local CSV cache directory",
	)
	yearsFlag := flag.Int(
		"years",
		2,
		"how many years of minute bars to ensure",
	)
	onlyFlag := flag.String(
		"only",
		"",
		"comma-separated subset of symbols (default: all allowed underlyings)",
	)
	daysFlag := flag.Int(
		"days",
		7,
		"print minute bar counts for this many calendar days (America/New_York)",
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	apiKey := os.Getenv("MASSIVE_API_KEY")
	if apiKey == "" {
		logger.Fatalf("MASSIVE_API_KEY is not set")
	}

	underlyingsPath := *underlyingsFlag
	if underlyingsPath == "" {
		underlyingsPath = defaultUnderlyingsPath()
	}
	if err := config.LoadAllowedUnderlyings(underlyingsPath); err != nil {
		logger.Fatalf("load allowed underlyings: %v", err)
	}

	symbols, err := symbolsToDownload(*onlyFlag)
	if err != nil {
		logger.Fatalf("%v", err)
	}

	endDate := time.Now().UTC()
	startDate := endDate.AddDate(-*yearsFlag, 0, 0)
	localProv := data.NewLocalFileDataProvider(
		*dataDirFlag,
		data.NewMassiveDataProvider(apiKey),
	)

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		logger.Fatalf("load timezone: %v", err)
	}
	days := calendarDays(endDate, *daysFlag, loc)
	providerName := localProv.GetSecondary().GetName()

	logger.Infof(
		"downloading %d underlyings minute bars [%s → %s] into %s",
		len(symbols),
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
		*dataDirFlag,
	)

	failed := 0
	counts := make(map[string][]int, len(symbols))
	countErrs := make(map[string]error, len(symbols))
	for i, symbol := range symbols {
		if err := ctx.Err(); err != nil {
			logger.Fatalf("interrupted after %d/%d symbols: %v", i, len(symbols), err)
		}

		logger.Infof("(%d/%d) ensuring %s", i+1, len(symbols), symbol)
		if err := localProv.EnsureLocalData(symbol, startDate, endDate); err != nil {
			failed++
			logger.Errorf("ensure %s: %v", symbol, err)
			countErrs[symbol] = err
			continue
		}

		path := symbolCSVPath(*dataDirFlag, providerName, symbol)
		dayCounts, err := countBarsByDay(path, days, loc)
		if err != nil {
			failed++
			countErrs[symbol] = err
			continue
		}
		counts[symbol] = dayCounts
	}

	fmt.Fprintf(
		os.Stderr,
		"minute bar counts last %d days (%s)\n",
		*daysFlag,
		loc.String(),
	)
	if err := writeBarCountTable(os.Stdout, symbols, days, counts, countErrs); err != nil {
		logger.Fatalf("write bar counts: %v", err)
	}

	if failed > 0 {
		logger.Fatalf("spot download finished with %d/%d failures", failed, len(symbols))
	}
	logger.Infof("spot download finished: %d underlyings", len(symbols))
}

func symbolsToDownload(only string) ([]string, error) {
	wanted := make(map[string]struct{})
	if strings.TrimSpace(only) != "" {
		for _, part := range strings.Split(only, ",") {
			symbol := strings.ToUpper(strings.TrimSpace(part))
			if symbol == "" {
				continue
			}
			if !config.IsAllowedUnderlying(symbol) {
				return nil, fmt.Errorf("symbol %s is not in allowed underlyings", symbol)
			}
			wanted[symbol] = struct{}{}
		}
	}

	symbols := make([]string, 0, len(config.AllowedUnderlyings))
	for symbol := range config.AllowedUnderlyings {
		if len(wanted) > 0 {
			if _, ok := wanted[symbol]; !ok {
				continue
			}
		}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	if len(symbols) == 0 {
		return nil, fmt.Errorf("no underlyings to download")
	}
	return symbols, nil
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/spot-data [flags]\n")
		fmt.Fprintf(os.Stderr, "       go run .\\cmd\\spot-data\\main.go [flags]\n\n")
		fmt.Fprintf(os.Stderr, "No flags required. Defaults: 2 years of minute bars for all allowed\n")
		fmt.Fprintf(os.Stderr, "underlyings, then a 7-day CSV count on stdout. Requires MASSIVE_API_KEY.\n\n")
		flag.PrintDefaults()
	}
}

func calendarDays(now time.Time, n int, loc *time.Location) []time.Time {
	if n < 1 {
		n = 1
	}
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	days := make([]time.Time, n)
	for i := 0; i < n; i++ {
		days[i] = today.AddDate(0, 0, i-n+1)
	}
	return days
}

func symbolCSVPath(dataDir, providerName, symbol string) string {
	safe := strings.ReplaceAll(strings.ToUpper(symbol), ":", "-")
	return filepath.Join(dataDir, providerName, safe+".csv")
}

func countBarsByDay(path string, days []time.Time, loc *time.Location) ([]int, error) {
	counts := make([]int, len(days))
	index := make(map[string]int, len(days))
	for i, day := range days {
		index[day.Format("2006-01-02")] = i
	}

	file, err := os.Open(path)
	if err != nil {
		return counts, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	if _, err := reader.Read(); err != nil {
		return counts, err
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) == 0 {
			continue
		}

		ts, err := time.Parse(time.RFC3339, row[0])
		if err != nil {
			continue
		}
		key := ts.In(loc).Format("2006-01-02")
		if i, ok := index[key]; ok {
			counts[i]++
		}
	}

	return counts, nil
}

func writeBarCountTable(
	out io.Writer,
	symbols []string,
	days []time.Time,
	counts map[string][]int,
	errs map[string]error,
) error {
	w := csv.NewWriter(out)
	header := []string{"symbol"}
	for _, day := range days {
		header = append(header, day.Format("2006-01-02"))
	}
	header = append(header, "total", "error")
	if err := w.Write(header); err != nil {
		return err
	}

	for _, symbol := range symbols {
		row := make([]string, 0, len(days)+3)
		row = append(row, symbol)
		if err, ok := errs[symbol]; ok && err != nil {
			for range days {
				row = append(row, "")
			}
			row = append(row, "", err.Error())
			if err := w.Write(row); err != nil {
				return err
			}
			continue
		}
		total := 0
		for _, n := range counts[symbol] {
			total += n
			row = append(row, fmt.Sprintf("%d", n))
		}
		row = append(row, fmt.Sprintf("%d", total), "")
		if err := w.Write(row); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func defaultUnderlyingsPath() string {
	candidates := []string{
		filepath.Join("internal", "pipeline", "config", "allowed_underlyings.txt"),
		filepath.Join("..", "..", "internal", "pipeline", "config", "allowed_underlyings.txt"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}
