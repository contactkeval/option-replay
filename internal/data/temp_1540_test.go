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
	// step            = 5.0
	// localDataProv   = NewLocalFileDataProvider("..\\..\\input\\data", NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))) // Massive data provider as secondary
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
func getClosePrice(underlying string, date time.Time, loc *time.Location) (float64, error) {

	bars, _ := localDataProv.GetBars(underlying, date, date.Add(5*time.Minute), MultiplierOne, TimespanMinute)

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
	localDataProv Provider,
) (bestStrike, bestCall, bestPut float64, err error) {

	getPrices := func(strike float64) (float64, float64, error) {
		call, err := localDataProv.GetOptionPrice(underlying, strike, expiry, "call", openTime)
		if err != nil {
			return 0, 0, err
		}
		put, err := localDataProv.GetOptionPrice(underlying, strike, expiry, "put", openTime)
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
	diffFile, err := os.Open("..\\..\\input\\data\\massive\\spy_spx_diff.csv")
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
	writer.Write([]string{"Date", "Strike", "Diff", "Close", "", "OpenSPY", "Call0o", "Put0o", "Call1o", "Put1o", "", "CloseSPY", "Call0c", "Put0c", "Call1c", "Put1c"})

	loc, _ := time.LoadLocation("America/New_York")

	currRecord, _ := reader.Read()
	closeAfter := 18 * time.Minute
	for {
		nextRecord, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		expDate, _ := time.ParseInLocation("02-01-2006", currRecord[0], loc)
		diff, _ := strconv.ParseFloat(currRecord[1], 64)
		nextDate, _ := time.ParseInLocation("02-01-2006", nextRecord[0], loc)
		currRecord = nextRecord

		// 1. Get close
		tradeTime := time.Date(expDate.Year(), expDate.Month(), expDate.Day(), 15, 40, 0, 0, loc)
		closePrice, err := getClosePrice("SPY", tradeTime, loc)
		if err != nil {
			continue
		}
		closePrice = closePrice*10 + diff // SPX is often quoted in 1/10th points

		// 2. ATM
		atmStrike := math.Round(closePrice/step) * step

		// 3. Time = 15:40
		openTime := time.Date(expDate.Year(), expDate.Month(), expDate.Day(), 15, 40, 0, 0, loc)

		// 4. Find strike
		bestStrike, call, put, err := findBestStrike(
			Temp_underlying,
			atmStrike,
			expDate,
			openTime,
			step,
			localDataProv,
		)
		if err != nil {
			continue
		}

		getPrices := func(strike float64) (float64, float64, error) {
			call, err := localDataProv.GetOptionPrice(Temp_underlying, strike, expDate, "call", openTime)
			if err != nil {
				return 0, 0, err
			}
			put, err := localDataProv.GetOptionPrice(Temp_underlying, strike, expDate, "put", openTime)
			return call, put, err
		}

		currDate := expDate
		expDate = nextDate
		call1, put1, err := getPrices(bestStrike)
		if err != nil {
			return
		}

		openTime = openTime.Add(closeAfter)
		expDate = currDate
		call2, put2, err := getPrices(bestStrike)
		if err != nil {
			return
		}

		expDate = nextDate
		call3, put3, err := getPrices(bestStrike)
		if err != nil {
			return
		}

		tradeTime = tradeTime.Add(closeAfter)
		closePrice2, err := getClosePrice("SPY", tradeTime, loc)
		if err != nil {
			continue
		}

		expDate = currDate
		// 5. Write output
		writer.Write([]string{
			expDate.Format("02-01-2006"),
			fmt.Sprintf("%.0f", bestStrike),
			fmt.Sprintf("%.2f", diff),
			fmt.Sprintf("%.2f", closePrice),
			"",
			fmt.Sprintf("%.2f", (closePrice-diff)/10),
			fmt.Sprintf("%.2f", call),
			fmt.Sprintf("%.2f", put),
			fmt.Sprintf("%.2f", call1),
			fmt.Sprintf("%.2f", put1),
			"",
			fmt.Sprintf("%.2f", closePrice2),
			fmt.Sprintf("%.2f", call2),
			fmt.Sprintf("%.2f", put2),
			fmt.Sprintf("%.2f", call3),
			fmt.Sprintf("%.2f", put3),
		})
	}
}

func TestSPXPipeline(t *testing.T) {
	// This test can be used to run the entire SPX pipeline for a single date, using the logic from TestMain.
	// It can be useful for debugging and validating the individual components of the pipeline.
}
