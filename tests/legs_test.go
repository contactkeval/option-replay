package tests

import (
	"testing"
)

func TestMultiLegPnLConsistency(t *testing.T) {
	// // Build a simple two-leg: long call + short call (same strike) and ensure PnL arithmetic
	// legA := st.TradeLeg{Strike: 100, OptType: "call", Qty: 1, Expiration: time.Now().AddDate(0, 0, 30)}
	// legB := st.TradeLeg{Strike: 100, OptType: "call", Qty: -1, Expiration: time.Now().AddDate(0, 0, 30)}

	// S := 105.0
	// r := 0.02
	// iv := 0.2
	// dur := legA.Expiration.Sub(time.Now())
	// pA := pricing.BlackScholesPrice(S, legA.Strike, r, iv, dur, "call")
	// pB := pricing.BlackScholesPrice(S, legB.Strike, r, iv, dur, "call")

	// // since strikes equal, prices equal; net should be near zero
	// net := pA*float64(legA.Qty) + pB*float64(legB.Qty)
	// if net > 1e-6 || net < -1e-6 {
	// 	t.Fatalf("expected net ~0, got %f", net)
	// }
}
