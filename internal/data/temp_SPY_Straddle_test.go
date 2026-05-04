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

var (
	Underlying = "SPY"
	step       = 1.0

	localDataProv = NewLocalFileDataProvider(
		"..\\..\\input\\data",
		NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY")),
	)
)

// ---------------------------------------------------
// Get Spot Price
// ---------------------------------------------------

func getSpot(t time.Time) (float64, error) {
	bars, err := localDataProv.GetBars(Underlying, t, t.Add(time.Minute), MultiplierOne, TimespanMinute)
	if err != nil || len(bars) == 0 {
		return 0, fmt.Errorf("no bars")
	}
	return bars[0].Close, nil
}

// ---------------------------------------------------
// MAIN
// ---------------------------------------------------

func TestSPYStraddleReport(t *testing.T) {

	loc, _ := time.LoadLocation("America/New_York")

	// --- Read CSV (trading days only) ---
	file, err := os.Open("..\\..\\input\\data\\massive\\spy_spx_diff_sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Read() // skip header

	var tradingDays []time.Time

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		d, _ := time.ParseInLocation("02-01-2006", rec[0], loc)
		tradingDays = append(tradingDays, d)
	}

	// --- Output ---
	out, _ := os.Create("spy_straddle_report.csv")
	defer out.Close()

	writer := csv.NewWriter(out)
	defer writer.Flush()

	writer.Write([]string{
		"Date", "Expiry", "DTE", "Strike",
		"Call_0945", "Put_0945",
		"Call_1545", "Put_1545",
	})

	// ---------------------------------------------------
	// LOOP
	// ---------------------------------------------------

	for i, tradeDate := range tradingDays {

		tOpen := time.Date(tradeDate.Year(), tradeDate.Month(), tradeDate.Day(), 9, 45, 0, 0, loc)
		tClose := time.Date(tradeDate.Year(), tradeDate.Month(), tradeDate.Day(), 15, 45, 0, 0, loc)

		spot, err := getSpot(tOpen)
		if err != nil {
			continue
		}

		atm := math.Round(spot/step) * step

		for dte := 0; dte <= 7; dte++ {

			expIdx := i + dte
			if expIdx >= len(tradingDays) {
				continue
			}

			expiry := tradingDays[expIdx]

			// --- Find best strike at open ---
			strike, callOpen, putOpen, err := findBestStrike(
				Underlying,
				atm,
				expiry,
				tOpen,
				step,
				localDataProv,
			)
			if err != nil {
				continue
			}

			// --- Close prices (same strike) ---
			callClose, err1 := localDataProv.GetOptionPrice(Underlying, strike, expiry, "call", tClose)
			putClose, err2 := localDataProv.GetOptionPrice(Underlying, strike, expiry, "put", tClose)

			if err1 != nil || err2 != nil {
				continue
			}

			writer.Write([]string{
				tradeDate.Format("02-01-2006"),
				expiry.Format("02-01-2006"),
				strconv.Itoa(dte),
				fmt.Sprintf("%.0f", strike),

				fmt.Sprintf("%.2f", callOpen),
				fmt.Sprintf("%.2f", putOpen),

				fmt.Sprintf("%.2f", callClose),
				fmt.Sprintf("%.2f", putClose),
			})
		}
	}

	logger.Infof("Done: spy_straddle_report.csv generated")
}
