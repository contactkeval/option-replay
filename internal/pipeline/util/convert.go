package util

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func NanosecondsToSeconds(v uint64) uint32 {
	return uint32(v / 1_000_000_000)
}

func PriceToUint32(v float64) uint32 {
	return uint32(math.Round(v * float64(config.PriceScale)))
}

func PriceFromUint32(v uint32) float64 {
	return float64(v) / float64(config.PriceScale)
}

// EncodeExpiryDate matches the uint32 encoding written into parquet
// (year*100000 + month*100 + day).
func EncodeExpiryDate(t time.Time) uint32 {
	t = t.UTC()
	return uint32(t.Year()*100000 + int(t.Month())*100 + t.Day())
}

func DecodeExpiryDate(v uint32) time.Time {
	year := int(v / 100000)
	rem := int(v % 100000)
	month := rem / 100
	day := rem % 100
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// StrikeToUint32 stores OCC strikes as strike*1000 (e.g. 390.00 -> 390000).
func StrikeToUint32(strike float64) uint32 {
	return uint32(math.Round(strike * 1000))
}

func StrikeFromUint32(v uint32) float64 {
	return float64(v) / 1000.0
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

	return uint32(f * float64(config.PriceScale))
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
