package util

import (
	"math"
	"strconv"

	"github.com/contactkeval/option-replay/internal/pipeline/constants"
)

func NanosecondsToSeconds(v uint64) uint32 {
	return uint32(v / 1_000_000_000)
}

func PriceToUint32(v float64) uint32 {
	return uint32(math.Round(v * float64(constants.PriceScale)))
}

func FormatOptionType(v bool) string {
	if v {
		return "C"
	}
	return "P"
}

func ParseOptionType(v string) bool {
	return v == "C"
}

func ParseStrike(v string) uint32 {
	n, _ := strconv.ParseUint(v, 10, 32)
	return uint32(n)
}
