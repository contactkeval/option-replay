package stage2_finalize

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func ParseTicker(raw string) (config.ParsedTicker, error) {

	// Example:
	// O:SPY230327P00390000
	if !strings.HasPrefix(raw, "O:") {
		return config.ParsedTicker{}, errors.New("invalid option ticker")
	}

	s := raw[2:]
	if len(s) < 15 {
		return config.ParsedTicker{}, errors.New("invalid ticker length")
	}

	expiryStart := len(s) - 15
	underlying := s[:expiryStart]
	expiryStr := s[expiryStart : expiryStart+6]
	optionTypeChar := s[expiryStart+6]
	strikeStr := s[expiryStart+7:]
	strike, err := strconv.ParseUint(strikeStr, 10, 32)
	if err != nil {
		return config.ParsedTicker{}, fmt.Errorf("parse strike price: %w", err)
	}
	expiry, err := time.Parse("060102", expiryStr)
	if err != nil {
		return config.ParsedTicker{}, fmt.Errorf("parse expiry date: %w", err)
	}

	return config.ParsedTicker{
		Underlying: strings.ToUpper(underlying),
		ExpiryDate: expiry,
		OptionType: optionTypeChar == 'C',
		Strike:     uint32(strike),
	}, nil
}
