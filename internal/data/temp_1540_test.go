package data

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

type DiffRow struct {
	Date time.Time
	Diff float64
}

type TempBar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// type TempProvider interface {
// 	GetOptionPrice(string, float64, time.Time, string, time.Time) (float64, error)
// }

var (
	Temp_underlying = "SPX"
	step            = 5.0
	localDataProv   = NewLocalFileDataProvider("..\\..\\input\\data", NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))) // Massive data provider as secondary
)

// ---------------------------------------------------
// Load OHLC
// ---------------------------------------------------
// func loadOHLC(path string) ([]Bar, error) {
// 	file, err := os.Open(path)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer file.Close()

// 	reader := csv.NewReader(file)

// 	// skip header
// 	reader.Read()

// 	var bars []Bar

// 	for {
// 		row, err := reader.Read()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			continue
// 		}

// 		t, _ := time.Parse(time.RFC3339, row[0])
// 		open, _ := strconv.ParseFloat(row[1], 64)
// 		high, _ := strconv.ParseFloat(row[2], 64)
// 		low, _ := strconv.ParseFloat(row[3], 64)
// 		closeVal, _ := strconv.ParseFloat(row[4], 64)
// 		vol, _ := strconv.ParseFloat(row[5], 64)

// 		bars = append(bars, Bar{
// 			Date:   t,
// 			Open:   open,
// 			High:   high,
// 			Low:    low,
// 			Close:  closeVal,
// 			Volume: vol,
// 		})
// 	}

// 	return bars, nil
// }

// ---------------------------------------------------
// Get 15:40 close
// ---------------------------------------------------
func getCloseAt1540(date time.Time, loc *time.Location) (float64, error) {
	target := time.Date(date.Year(), date.Month(), date.Day(), 15, 40, 0, 0, loc)

	bars, _ := localDataProv.GetBars("SPY", target, target.Add(5*time.Minute), multiplierOne, timespanMinute)

	if len(bars) > 0 {
		return bars[0].Close, nil
	}
	return 0, fmt.Errorf("15:40 bar not found for %v", date)
}

// ---------------------------------------------------
// Find Best Strike
// ---------------------------------------------------
func findBestStrike(
	underlying string,
	baseStrike float64,
	expiry time.Time,
	openTime time.Time,
	step float64,
	dataProv Provider,
) (bestStrike, bestCall, bestPut float64, err error) {

	getPrices := func(strike float64) (float64, float64, error) {
		call, err := dataProv.GetOptionPrice(underlying, strike, expiry, "call", openTime)
		if err != nil {
			return 0, 0, err
		}
		put, err := dataProv.GetOptionPrice(underlying, strike, expiry, "put", openTime)
		return call, put, err
	}

	call, put, err := getPrices(baseStrike)
	if err != nil {
		return
	}

	prevDiff := math.Abs(call - put)

	bestStrike = baseStrike
	bestCall = call
	bestPut = put

	var direction float64
	if call > put {
		direction = step
	} else {
		direction = -step
	}

	currentStrike := baseStrike + direction

	for {
		call, put, err := getPrices(currentStrike)
		if err != nil {
			break
		}

		currDiff := math.Abs(call - put)

		if currDiff > prevDiff {
			break
		}

		prevDiff = currDiff
		bestStrike = currentStrike
		bestCall = call
		bestPut = put

		currentStrike += direction
	}

	return
}

// ---------------------------------------------------
// MAIN EXECUTION
// ---------------------------------------------------
func TestMain(t *testing.T) {

	// Open Diff.csv
	diffFile, err := os.Open("..\\..\\input\\data\\massive\\spy_spx_diff_sample.csv")
	if err != nil {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		logger.Infof("Current directory: %s", dir)
		t.Fatal(err)
	}
	defer diffFile.Close()

	reader := csv.NewReader(diffFile)
	reader.Read() // skip header

	// Output file
	outFile, err := os.Create("Output.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	defer writer.Flush()

	// Header
	writer.Write([]string{"Date", "Diff", "Close", "Strike", "Call", "Put"})

	loc, _ := time.LoadLocation("America/New_York")

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		date, _ := time.ParseInLocation("02-01-2006", record[0], loc)
		diff, _ := strconv.ParseFloat(record[1], 64)

		// 1. Get close
		closePrice, err := getCloseAt1540(date, loc)
		if err != nil {
			continue
		}
		closePrice = closePrice*10 + diff // SPX is often quoted in 1/10th points

		// 2. ATM
		atmStrike := math.Round(closePrice/step) * step

		// 3. Time = 15:40
		openTime := time.Date(date.Year(), date.Month(), date.Day(), 15, 40, 0, 0, loc)

		// 4. Find strike
		bestStrike, call, put, err := findBestStrike(
			Temp_underlying,
			atmStrike,
			date,
			openTime,
			step,
			localDataProv,
		)
		if err != nil {
			continue
		}

		// 5. Write output
		writer.Write([]string{
			date.Format("02-01-2006"),
			fmt.Sprintf("%.2f", diff),
			fmt.Sprintf("%.2f", closePrice),
			fmt.Sprintf("%.0f", bestStrike),
			fmt.Sprintf("%.2f", call),
			fmt.Sprintf("%.2f", put),
		})
	}
}
