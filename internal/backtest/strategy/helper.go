package strategy

import (
	"fmt"
	"math"
	"time"

	"github.com/contactkeval/option-replay/internal/pricing"
)

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

func fetchATMOptionPrices(underlying string, expiry time.Time, asOfPrice float64) (call float64, put float64, err error) {
	// TODO: call your option chain API
	return 5.20, 4.85, nil
}

func estimateIVFromATM(call, put, spot float64) float64 {
	// TODO: real IV estimator
	return 0.20
}

func ImpliedVolatility(
	isCall bool, // Is the option a call?
	marketPrice float64, // Market price of the option
	S float64, // Spot price
	K float64, // Strike price
	T float64, // Time to expiration in years
	r float64, // Risk-free interest rate
) (float64, error) {

	sigma := 0.2 // initial guess (20%)
	tol := 1e-6

	for i := 0; i < 100; i++ {
		price := pricing.BlackScholesPrice(isCall, S, K, T, r, sigma)
		vega := pricing.BlackScholesVega(S, K, T, r, sigma)

		diff := price - marketPrice
		if math.Abs(diff) < tol {
			return sigma, nil
		}

		sigma -= diff / vega
		if sigma <= 0 {
			sigma = tol
		}
	}

	return 0, fmt.Errorf("IV did not converge")
}

func computeStrikeFromDelta(delta, spot, iv float64, expiry time.Time) float64 {
	// TODO: real delta → strike model
	return spot * (1 - (delta/100.0)*0.5)
}

func StrikeFromDelta(
	isCall bool,
	delta float64,
	S float64,
	T float64,
	r float64,
	sigma float64,
) float64 {

	var d1 float64
	if isCall {
		d1 = pricing.NormInv(delta)
	} else {
		d1 = pricing.NormInv(delta + 1)
	}

	exponent := d1*sigma*math.Sqrt(T) - (r+0.5*sigma*sigma)*T
	K := S * math.Exp(-exponent)

	return K
}
