package config

var AllowedUnderlyings = map[string]struct{}{
	"BABA": {},
	"COIN": {},
	"DIS":  {},
	"HOOD": {},
	"MU":   {},
	"NVDA": {},
	"SPY":  {},
}

func IsAllowedUnderlying(ticker string) bool {

	_, ok := AllowedUnderlyings[ticker]

	return ok
}
