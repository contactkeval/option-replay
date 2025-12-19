package tests

import (
	"math"
	"testing"
	"time"
	"github.com/contactkeval/option-replay/internal/pricing"
)

// Simple sanity check: ATM call should have non-zero value
func TestBlackScholesCallBasic(t *testing.T) {
	price := 100.0
	strike := 100.0
	days := 30.0/365.0
	rate := 0.05
	iv := 0.20

	call := pricing.BlackScholesPrice(price, strike, rate, iv, time.Duration(days*24*365)*time.Hour, "call")
	if call <= 0 {
		t.Fatalf("expected call price > 0, got %f", call)
	}
}

// Put-call parity check
func TestBlackScholesPutCallParity(t *testing.T) {
	price := 100.0
	strike := 100.0
	days := 45.0/365.0
	rate := 0.03
	iv := 0.25

	call := pricing.BlackScholesPrice(price, strike, rate, iv, time.Duration(days*24*365)*time.Hour, "call")
	put := pricing.BlackScholesPrice(price, strike, rate, iv, time.Duration(days*24*365)*time.Hour, "put")

	lhs := call - put
	rhs := price - strike*math.Exp(-rate*days)

	if math.Abs(lhs-rhs) > 1e-6 {
		t.Fatalf("put-call parity violated: LHS=%f RHS=%f", lhs, rhs)
	}
}
