package staging

import (
	"fmt"
)

type ParsedTicker struct {
	Underlying string
	Expiry     string
	OptionType string
	Strike     string
}

func ParseTicker(ticker string) (*ParsedTicker, error) {

	if len(ticker) < 17 {
		return nil, fmt.Errorf("invalid ticker: %s", ticker)
	}

	if ticker[:2] != "O:" {
		return nil, fmt.Errorf("not option ticker: %s", ticker)
	}

	raw := ticker[2:]

	strike := raw[len(raw)-8:]
	optionType := raw[len(raw)-9 : len(raw)-8]
	expiry := raw[len(raw)-15 : len(raw)-9]
	underlying := raw[:len(raw)-15]

	return &ParsedTicker{
		Underlying: underlying,
		Expiry:     expiry,
		OptionType: optionType,
		Strike:     strike,
	}, nil
}
