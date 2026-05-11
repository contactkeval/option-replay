package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
)

var (
	startDateStr = "2024-07-02"
	endDateStr   = "2024-07-02"
	symbolsInput = "SPY,O:SPY240702C00545000,O:SPY240702P00545000,O:SPY240703C00545000,O:SPY240703P00545000,O:SPY240705C00545000,O:SPY240705P00545000"
	outputFile   = "minute_bars_wide.csv"
)

var localDataProv = data.NewLocalFileDataProvider(
	"..\\..\\input\\data",
	data.NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")),
)

// ----------------------------------
// Helpers
// ----------------------------------

type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

func parseSymbols(input string) []string {
	parts := strings.Split(input, ",")
	var out []string
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func dateRange(startDate, endDate time.Time) []time.Time {
	var days []time.Time
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	return days
}

// ----------------------------------
// MAIN
// ----------------------------------

func main() {

	loc, _ := time.LoadLocation("America/New_York")
	startDate, _ := time.ParseInLocation("2006-01-02", startDateStr, loc)
	endDate, _ := time.ParseInLocation("2006-01-02", endDateStr, loc)
	symbols := parseSymbols(symbolsInput)

	// symbol → timestamp → bar
	dataBars := make(map[string]map[int64]Bar)
	// master timestamp set
	tsSet := make(map[int64]struct{})
	days := dateRange(startDate, endDate)

	for _, symbol := range symbols {
		logger.Infof("Fetching: %s", symbol)
		dataBars[symbol] = make(map[int64]Bar)

		for _, day := range days {
			fromDate := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, loc)
			toDate := time.Date(day.Year(), day.Month(), day.Day(), 16, 30, 0, 0, loc)
			bars, err := localDataProv.GetBars(symbol, fromDate, toDate, data.MultiplierOne, data.TimespanMinute)
			if err != nil {
				continue
			}

			for _, b := range bars {
				ts := b.Date.Unix()
				dataBars[symbol][ts] = Bar{
					Date:   b.Date,
					Open:   b.Open,
					High:   b.High,
					Low:    b.Low,
					Close:  b.Close,
					Volume: b.Volume,
				}
				tsSet[ts] = struct{}{}
			}
		}
	}

	// ----------------------------------
	// Build sorted timestamps
	// ----------------------------------
	var timestamps []int64
	for ts := range tsSet {
		timestamps = append(timestamps, ts)
	}

	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	// ----------------------------------
	// Write CSV
	// ----------------------------------

	out, _ := os.Create(outputFile)
	defer out.Close()

	writer := csv.NewWriter(out)
	defer writer.Flush()

	// Header
	header1 := []string{"", "", ""}
	header2 := []string{"Timestamp", "Date", "Time"}
	for _, symbol := range symbols {
		header2 = append(header2, "",
			"Open",
			"High",
			"Low",
			"Close",
			"Volume",
		)
		header1 = append(header1, "", symbol, "", "", "", "")
	}
	writer.Write(header1)
	writer.Write(header2)

	// Rows
	for _, ts := range timestamps {
		row := []string{
			fmt.Sprintf("%d", ts),
			time.Unix(ts, 0).In(loc).Format("2006-01-02"),
			time.Unix(ts, 0).In(loc).Format("15:04:05"),
		}
		for _, symbol := range symbols {
			bar, ok := dataBars[symbol][ts]
			if !ok {
				// missing bar → empty columns
				row = append(row, "", "", "", "", "", "")
				continue
			}

			row = append(row, "",
				fmt.Sprintf("%.2f", bar.Open),
				fmt.Sprintf("%.2f", bar.High),
				fmt.Sprintf("%.2f", bar.Low),
				fmt.Sprintf("%.2f", bar.Close),
				fmt.Sprintf("%.0f", bar.Volume),
			)
		}
		writer.Write(row)
	}

	absPath, err := filepath.Abs(outputFile)
	if err != nil {
		logger.Infof("Done: %s generated", outputFile)
	} else {
		logger.Infof("Done: %s generated", absPath)
	}
}
