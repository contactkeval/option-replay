package stage1_raw_to_expiry

import (
	"errors"
	"strconv"
	"strings"

	"github.com/contactkeval/option-replay/internal/pipeline/model"
)

func ParseTicker(raw string) (model.ParsedTicker, error) {

	// Example:
	// O:SPY230327P00390000
	if !strings.HasPrefix(raw, "O:") {
		return model.ParsedTicker{}, errors.New("invalid option ticker")
	}

	s := raw[2:]
	if len(s) < 15 {
		return model.ParsedTicker{}, errors.New("invalid ticker length")
	}

	expiryStart := len(s) - 15
	underlying := s[:expiryStart]
	expiry := s[expiryStart : expiryStart+6]
	optionTypeChar := s[expiryStart+6]
	strikeStr := s[expiryStart+7:]
	strike, err := strconv.ParseUint(strikeStr, 10, 32)
	if err != nil {
		return model.ParsedTicker{}, err
	}

	return model.ParsedTicker{
		Underlying: strings.ToUpper(underlying),
		Expiry:     expiry,
		OptionType: optionTypeChar == 'C',
		Strike:     uint32(strike),
	}, nil
}
