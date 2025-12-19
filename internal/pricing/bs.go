package pricing

import (
	"math"
	"time"
)

func BlackScholesPrice(S, K, r, sigma float64, t time.Duration, optType string) float64 {
	years := t.Hours() / 24.0 / 365.0
	if years <= 0 {
		if optType == "call" { return math.Max(0.0, S-K) }
		return math.Max(0.0, K-S)
	}
	if sigma <= 0 {
		if optType == "call" { return math.Max(0.0, S-K) }
		return math.Max(0.0, K-S)
	}

	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*years) / (sigma*math.Sqrt(years))
	d2 := d1 - sigma*math.Sqrt(years)
	if optType == "call" {
		return S*normCdf(d1) - K*math.Exp(-r*years)*normCdf(d2)
	}
	return K*math.Exp(-r*years)*normCdf(-d2) - S*normCdf(-d1)
}

func normCdf(x float64) float64 { return 0.5 * (1.0 + math.Erf(x/math.Sqrt2)) }
