package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/logger"
)

var (
	startDateStr = "2026-04-01"
	endDateStr   = "2026-04-02"
	symbolsInput = "SPY,QQQ"
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

func dateRange(start, end time.Time) []time.Time {
	var days []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
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
			start := time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, loc)
			end := time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, loc)
			bars, err := localDataProv.GetBars(symbol, start, end, data.MultiplierOne, data.TimespanMinute)
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
	header := []string{"DateTime"}
	for _, sym := range symbols {
		header = append(header,
			sym+"_Open",
			sym+"_High",
			sym+"_Low",
			sym+"_Close",
			sym+"_Volume",
		)
	}
	writer.Write(header)

	// Rows
	for _, ts := range timestamps {
		row := []string{
			time.Unix(ts, 0).In(loc).Format(time.RFC3339),
		}
		for _, sym := range symbols {
			bar, ok := dataBars[sym][ts]
			if !ok {
				// missing bar → empty columns
				row = append(row, "", "", "", "", "")
				continue
			}

			row = append(row,
				fmt.Sprintf("%.2f", bar.Open),
				fmt.Sprintf("%.2f", bar.High),
				fmt.Sprintf("%.2f", bar.Low),
				fmt.Sprintf("%.2f", bar.Close),
				fmt.Sprintf("%.0f", bar.Volume),
			)
		}
		writer.Write(row)
	}

	logger.Infof("Done: %s generated", outputFile)
}
