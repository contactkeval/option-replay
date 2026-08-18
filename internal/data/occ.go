package data

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func isOptionSymbol(symbol string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "O:")
}

func normalizeUnderlying(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if isOptionSymbol(s) {
		parsed, err := parseOptionSymbol(s)
		if err == nil {
			return parsed.Underlying
		}
	}
	parts := strings.SplitN(s, ":", 2)
	return parts[len(parts)-1]
}

func parseOptionSymbol(raw string) (config.ParsedTicker, error) {
	if !strings.HasPrefix(raw, "O:") && !strings.HasPrefix(raw, "o:") {
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

	strikeVal, err := strconv.ParseUint(strikeStr, 10, 32)
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
		OptionType: optionTypeChar == 'C' || optionTypeChar == 'c',
		Strike:     uint32(strikeVal),
	}, nil
}

func formatOptionSymbol(underlying string, expiryDate time.Time, optionType string, strike float64) string {
	expDt := expiryDate.UTC().Format("060102")
	optType := "C"
	if opt := strings.ToLower(optionType); opt == "put" || opt == "p" {
		optType = "P"
	}
	strikeStr := fmt.Sprintf("%08d", int(math.Round(strike*1000)))

	underlyingParts := strings.SplitN(underlying, ":", 2)
	underlying = underlyingParts[len(underlyingParts)-1]
	switch {
	case underlying == "SPX":
		underlying = "SPXW"
	case underlying == "NDX":
		underlying = "NDXP"
	case underlying == "RUT":
		underlying = "RUTW"
	case underlying == "VIX":
		underlying = "VIXW"
	}
	return fmt.Sprintf("O:%s%s%s%s", strings.ToUpper(underlying), expDt, optType, strikeStr)
}

func optionTypeFromString(optionType string) bool {
	opt := strings.ToLower(strings.TrimSpace(optionType))
	return opt == "call" || opt == "c"
}

func optionTypeToString(isCall bool) string {
	if isCall {
		return "call"
	}
	return "put"
}
