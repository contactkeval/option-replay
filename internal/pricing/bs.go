package pricing

import (
	"fmt"
	"math"
)

const sqrt2Pi = 2.5066282746310002

// BlackScholesPrice calculates the price of a European option using the Black-Scholes model.
//
// Parameters:
//   - isCall: true for call option, false for put option
//   - S: spot price of the underlying asset
//   - K: strike price of the option
//   - T: time to expiry in years
//   - r: risk-free interest rate (annual)
//   - sigma: volatility of the underlying asset (annual, as a decimal)
//
// Returns:
//
//	The theoretical price of the option. If time to expiry or volatility is zero or negative,
//	returns the intrinsic value of the option.
//
// Note: This implementation uses the standard Black-Scholes formula for European options
// and relies on normCDF for the cumulative standard normal distribution function.
func BlackScholesPrice(
	isCall bool,
	S float64, // spot
	K float64, // strike
	T float64, // time to expiry in years
	r float64, // risk-free rate
	sigma float64, // volatility
) float64 {

	if T <= 0 || sigma <= 0 {
		return math.Max(0, S-K) // intrinsic fallback
	}

	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*T) / (sigma * math.Sqrt(T))
	d2 := d1 - sigma*math.Sqrt(T)

	if isCall {
		return S*normCDF(d1) - K*math.Exp(-r*T)*normCDF(d2)
	}
	return K*math.Exp(-r*T)*normCDF(-d2) - S*normCDF(-d1)
}

// BlackScholesVega calculates the vega of a European option using the Black-Scholes model.
// Vega measures the sensitivity of the option price to changes in the underlying asset's volatility.
//
// Parameters:
//   - S: Current price of the underlying asset
//   - K: Strike price of the option
//   - T: Time to expiration in years
//   - r: Risk-free interest rate
//   - sigma: Volatility (standard deviation) of the underlying asset's returns
//
// Returns:
//   The vega value, representing the change in option price per 1% change in volatility.
//   Returns 0 if T or sigma is non-positive.
func BlackScholesVega(
	S float64,
	K float64,
	T float64,
	r float64,
	sigma float64,
) float64 {

	if T <= 0 || sigma <= 0 {
		return 0
	}

	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*T) / (sigma * math.Sqrt(T))
	return S * normPDF(d1) * math.Sqrt(T)
}

// ImpliedVolATM calculates the implied volatility at-the-money using Newton-Raphson method.
// It takes the underlying price S, strike price K, time to expiry T (in years),
// risk-free rate r, and both call and put prices at the strike.
// The function iteratively solves for the volatility that makes the Black-Scholes price
// match the market price (average of call and put prices).
// Returns the implied volatility or an error if convergence fails or inputs are invalid.
func ImpliedVolATM(
	S, K, T, r float64,
	callPrice, putPrice float64,
) (float64, error) {

	if T <= 0 {
		return 0, fmt.Errorf("invalid expiry")
	}

	marketPrice := (callPrice + putPrice) / 2

	// Initial guess: 20%
	sigma := 0.20

	const (
		maxIter = 100
		tol     = 1e-6
	)

	for i := 0; i < maxIter; i++ {
		price := BlackScholesPrice(true, S, K, T, r, sigma)
		diff := price - marketPrice

		if math.Abs(diff) < tol {
			return sigma, nil
		}

		vega := BlackScholesVega(S, K, T, r, sigma)
		if vega < 1e-8 {
			break
		}

		sigma -= diff / vega

		// Guardrails
		if sigma <= 0 {
			sigma = 1e-4
		}
		if sigma > 5 {
			sigma = 5
		}
	}

	return 0, fmt.Errorf("implied vol did not converge")
}

// normPDF calculates the probability density function (PDF) of the standard normal distribution.
// It takes a float64 value x and returns the probability density at that point.
// The formula used is: exp(-0.5 * x^2) / sqrt(2π)
func normPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / sqrt2Pi
}

// normCDF computes the cumulative distribution function of the standard normal distribution
// for a given value x using the error function approximation.
// It returns a value between 0 and 1 representing the probability that a standard normal
// random variable is less than or equal to x.
func normCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}

// NormInv computes the inverse of the standard normal cumulative distribution function (quantile function).
// It returns the value x such that the cumulative probability at x equals p.
//
// The function uses a rational approximation algorithm based on Wichura's method,
// which provides high accuracy across the entire range of valid probabilities.
//
// Parameters:
//   - p: A probability value in the range (0, 1) (exclusive). Values outside this range will cause a panic.
//
// Returns:
//   The quantile value corresponding to the input probability p.
//
// Panics:
//   If p is not strictly between 0 and 1.
//
// Example:
//   NormInv(0.975) // Returns approximately 1.96 (95% confidence level)
//   NormInv(0.025) // Returns approximately -1.96
func NormInv(p float64) float64 {
	if p <= 0 || p >= 1 {
		panic("NormInv: p must be in (0,1)")
	}

	// Coefficients
	a := []float64{
		-3.969683028665376e+01,
		2.209460984245205e+02,
		-2.759285104469687e+02,
		1.383577518672690e+02,
		-3.066479806614716e+01,
		2.506628277459239e+00,
	}

	b := []float64{
		-5.447609879822406e+01,
		1.615858368580409e+02,
		-1.556989798598866e+02,
		6.680131188771972e+01,
		-1.328068155288572e+01,
	}

	c := []float64{
		-7.784894002430293e-03,
		-3.223964580411365e-01,
		-2.400758277161838e+00,
		-2.549732539343734e+00,
		4.374664141464968e+00,
		2.938163982698783e+00,
	}

	d := []float64{
		7.784695709041462e-03,
		3.224671290700398e-01,
		2.445134137142996e+00,
		3.754408661907416e+00,
	}

	plow := 0.02425
	phigh := 1 - plow

	var q, r float64

	if p < plow {
		q = math.Sqrt(-2 * math.Log(p))
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}

	if p > phigh {
		q = math.Sqrt(-2 * math.Log(1-p))
		return -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}

	q = p - 0.5
	r = q * q
	return (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q /
		(((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
}


