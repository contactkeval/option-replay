package data

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

// ---------------------------------------------------
// Helpers
// ---------------------------------------------------

func getStraddle(strike float64, expiry time.Time, t time.Time) (float64, float64, error) {
	call, err1 := localDataProv.GetOptionPrice(Underlying, strike, expiry, "call", t)
	put, err2 := localDataProv.GetOptionPrice(Underlying, strike, expiry, "put", t)

	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("option error")
	}
	return call, put, nil
}

// ---------------------------------------------------
// MAIN
// ---------------------------------------------------

func TestSPYIntradayLevels(t *testing.T) {

	loc, _ := time.LoadLocation("America/New_York")

	// -------------------------
	// Load CSV
	// -------------------------
	file, err := os.Open("..\\..\\input\\data\\massive\\spy_eod.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Read() // header

	type Day struct {
		Date time.Time
		High float64
		Low  float64
	}

	var days []Day

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		d, err := time.ParseInLocation("01/02/2006", rec[0], loc)
		if err != nil {
			continue
		}

		high := parseFloat(rec[3])
		low := parseFloat(rec[4])

		days = append(days, Day{d, high, low})
	}

	// sort ascending
	sort.Slice(days, func(i, j int) bool {
		return days[i].Date.Before(days[j].Date)
	})

	// -------------------------
	// Output
	// -------------------------
	out, _ := os.Create("spy_intraday_levels.csv")
	defer out.Close()

	writer := csv.NewWriter(out)
	defer writer.Flush()

	writer.Write([]string{
		"Date", "Strike", "InitCredit", "",

		"Open_Spot", "Open_Call1", "Open_Put1", "Open_Call2", "Open_Put2",

		"", "50_Time", "50_Spot", "50_Call1", "50_Put1", "50_Call2", "50_Put2",
		"", "100_Time", "100_Spot", "100_Call1", "100_Put1", "100_Call2", "100_Put2",
		"", "150_Time", "150_Spot", "150_Call1", "150_Put1", "150_Call2", "150_Put2",
		"", "200_Time", "200_Spot", "200_Call1", "200_Put1", "200_Call2", "200_Put2",

		"", "Close_Spot", "Close_Call1", "Close_Put1", "Close_Call2", "Close_Put2",
	})

	// -------------------------
	// LOOP
	// -------------------------

	for i := 0; i < len(days); i++ {

		if i+2 >= len(days) {
			continue
		}

		tradeDate := days[i].Date
		exp0 := days[i].Date
		exp1 := days[i+1].Date
		exp2 := days[i+2].Date

		dayHigh := days[i].High
		dayLow := days[i].Low

		tOpen := time.Date(tradeDate.Year(), tradeDate.Month(), tradeDate.Day(), 9, 45, 0, 0, loc)
		tClose := time.Date(tradeDate.Year(), tradeDate.Month(), tradeDate.Day(), 15, 45, 0, 0, loc)

		// --- Spot & strike ---
		spot, err := getSpot(tOpen)
		if err != nil {
			continue
		}

		strike := math.Round(spot/step) * step
		maxMove := math.Max(dayHigh-strike, strike-dayLow)

		// --- Initial credit ---
		c0, p0, err := getStraddle(strike, exp0, tOpen)
		if err != nil {
			continue
		}
		initCredit := c0 + p0

		levels := []float64{
			0.5 * initCredit,
			1.0 * initCredit,
			1.5 * initCredit,
			2.0 * initCredit,
		}

		// --- Open snapshot ---
		spotOpen := spot
		c1o, p1o, _ := getStraddle(strike, exp1, tOpen)
		c2o, p2o, _ := getStraddle(strike, exp2, tOpen)

		// tracking
		hit := make([]bool, 4)

		times := make([]string, 4)
		spotLevels := make([]float64, 4)

		call1 := make([]float64, 4)
		put1 := make([]float64, 4)
		call2 := make([]float64, 4)
		put2 := make([]float64, 4)

		// --- Intraday bars ---
		bars, err := localDataProv.GetBars(Underlying, tOpen, tClose, MultiplierOne, TimespanMinute)
		if err != nil {
			continue
		}

		hitLevel := 0
		for _, bar := range bars {

			upMove := bar.High - strike
			downMove := strike - bar.Low
			move := math.Max(upMove, downMove)

			for j := hitLevel; j < 4; j++ {

				if move >= levels[j] {

					hit[j] = true
					hitLevel++

					times[j] = bar.Date.In(loc).Format("15:04")
					spotLevels[j] = bar.Close

					c1, p1, err1 := getStraddle(strike, exp1, bar.Date)
					c2, p2, err2 := getStraddle(strike, exp2, bar.Date)

					if err1 == nil && err2 == nil {
						call1[j], put1[j] = c1, p1
						call2[j], put2[j] = c2, p2
					}
				}
			}
			if hitLevel >= 4 {
				// logger.Infof("all levels hit for %s", tradeDate.Format("2006-01-02"))
				break
			}
			if move >= maxMove {
				// logger.Infof("max move hit for %s", tradeDate.Format("2006-01-02"))
				break
			}
		}

		// --- Close ---
		spotClose, _ := getSpot(tClose)
		c1c, p1c, _ := getStraddle(strike, exp1, tClose)
		c2c, p2c, _ := getStraddle(strike, exp2, tClose)

		// --- Write ---
		row := []string{
			tradeDate.Format("2006-01-02"),
			fmt.Sprintf("%.0f", strike),
			fmt.Sprintf("%.2f", initCredit), "",

			fmt.Sprintf("%.2f", spotOpen),
			fmt.Sprintf("%.2f", c1o),
			fmt.Sprintf("%.2f", p1o),
			fmt.Sprintf("%.2f", c2o),
			fmt.Sprintf("%.2f", p2o),
		}

		for j := 0; j < 4; j++ {
			row = append(row, "",
				times[j],
				fmt.Sprintf("%.2f", spotLevels[j]),
				fmt.Sprintf("%.2f", call1[j]),
				fmt.Sprintf("%.2f", put1[j]),
				fmt.Sprintf("%.2f", call2[j]),
				fmt.Sprintf("%.2f", put2[j]),
			)
		}

		row = append(row, "",
			fmt.Sprintf("%.2f", spotClose),
			fmt.Sprintf("%.2f", c1c),
			fmt.Sprintf("%.2f", p1c),
			fmt.Sprintf("%.2f", c2c),
			fmt.Sprintf("%.2f", p2c),
		)

		writer.Write(row)
	}

	logger.Infof("Done: spy_intraday_levels.csv generated")
}
