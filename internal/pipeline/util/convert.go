package util

import (
	"math"
	"strconv"
	"strings"

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

// Converts:
// "1715600700000000000"
// ->
// unix seconds uint32
func NanosecondsToSecondsMust(
	value string,
) uint32 {

	nanos, err := strconv.ParseUint(
		strings.TrimSpace(value),
		10,
		64,
	)

	if err != nil {
		panic(err)
	}

	return uint32(nanos / 1_000_000_000)
}

// Converts:
// "523.17"
// ->
// 52317
//
// Precision:
// 2 decimal fixed-point
func PriceStringToUint32(
	value string,
) uint32 {

	f, err := strconv.ParseFloat(
		strings.TrimSpace(value),
		64,
	)

	if err != nil {
		panic(err)
	}

	return uint32(f * float64(constants.PriceScale))
}

// Converts:
// "12345"
// ->
// 12345
func Uint32Must(
	value string,
) uint32 {

	v, err := strconv.ParseUint(
		strings.TrimSpace(value),
		10,
		32,
	)

	if err != nil {
		panic(err)
	}

	return uint32(v)
}
