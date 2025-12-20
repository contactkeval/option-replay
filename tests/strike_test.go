package tests

import (
	"os"
	"testing"

	"github.com/contactkeval/option-replay/internal/backtest"
)

// func ResolveStrike(
// 	strikeExpr string,
// 	underlying string,
// 	spotPrice float64,
// 	openDate time.Time,
// 	expiryDate time.Time,
// 	legs []TradeLeg,
// ) (float64, error) {

func TestATMStrike(t *testing.T) {
	dataProv := getLocalFileDataProvider()
	openTime, _ := os.Parse("2006-01-02", "2025-06-15")
	expiryTime, _ := os.Parse("2006-01-02", "2025-07-15")
	strike, err := backtest.ResolveStrike("ATM", "AAPL", 150.55, openTime, expiryTime, nil)
	// TODO: implement test
}
