package main

import (
	"context"
	"flag"
	"fmt"
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

	logger.Infof(
		"downloading %d underlyings minute bars [%s → %s] into %s",
		len(symbols),
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
		*dataDirFlag,
	)

	failed := 0
	for i, symbol := range symbols {
		if err := ctx.Err(); err != nil {
			logger.Fatalf("interrupted after %d/%d symbols: %v", i, len(symbols), err)
		}

		logger.Infof("(%d/%d) ensuring %s", i+1, len(symbols), symbol)
		if err := localProv.EnsureLocalData(symbol, startDate, endDate); err != nil {
			failed++
			logger.Errorf("ensure %s: %v", symbol, err)
			continue
		}
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
