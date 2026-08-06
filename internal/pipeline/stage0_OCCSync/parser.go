package stage0_occ

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/contactkeval/option-replay/internal/db"
)

// OCC Series Download fixed-width layout (0-based, verified against live files):
//
//	[0:4]   Exchange (space-padded), e.g. "CBOE", "C2  "
//	[4:5]   Action: A / D / M
//	[5:13]  Report / file date MM/DD/YY
//	[13:43] Security description (30)
//	[43:49] Underlying symbol (6, space-padded; may have a leading digit on FLEX/EURO rows)
//	[49:58] Expiration MmmDDYYYY, e.g. "Aug132026"
//	[58:66] Strike: 5-digit integer + 3-digit decimal, space-padded on deletes
//	[66:67] Call indicator ("C") or blank
//	[68:69] Put indicator ("P") or blank (also used as an unrelated flag on calls)
//	[70:78] Activity date MM/DD/YY (adds only; deletes are often 70 chars)
const (
	minRecordLen = 67
	addRecordLen = 78
)

func ParseRecord(line string) (db.OCCRecord, error) {
	line = strings.TrimRight(line, "\r\n")

	if len(line) < minRecordLen {
		return db.OCCRecord{}, fmt.Errorf(
			"record too short: got %d chars",
			len(line),
		)
	}

	var rec db.OCCRecord

	rec.Exchange = strings.TrimSpace(line[0:4])
	rec.Action = strings.TrimSpace(line[4:5])

	switch rec.Action {
	case ActionAdd, ActionDelete, ActionModify:
	default:
		return db.OCCRecord{}, fmt.Errorf("unknown action %q", rec.Action)
	}

	reportDate, err := time.Parse("01/02/06", strings.TrimSpace(line[5:13]))
	if err != nil {
		return db.OCCRecord{}, fmt.Errorf("parse report date: %w", err)
	}

	rawSymbol := strings.TrimSpace(line[43:49])
	rec.Underlying = normalizeUnderlying(rawSymbol)
	if rec.Underlying == "" {
		return db.OCCRecord{}, fmt.Errorf("missing underlying symbol")
	}

	month := strings.TrimSpace(line[49:52])
	day := strings.TrimSpace(line[52:54])
	year := strings.TrimSpace(line[54:58])

	expiry, err := time.Parse(
		"02-Jan-2006",
		fmt.Sprintf("%s-%s-%s", day, month, year),
	)
	if err != nil {
		return db.OCCRecord{}, fmt.Errorf("parse expiry %q: %w", day+month+year, err)
	}
	rec.ExpiryDate = expiry

	strike, err := parseStrike(line[58:66])
	if err != nil {
		return db.OCCRecord{}, err
	}
	rec.Strike = strike

	optType, err := parseOptionType(line)
	if err != nil {
		return db.OCCRecord{}, err
	}
	rec.Type = optType

	rec.ActivityDate = reportDate
	if len(line) >= addRecordLen {
		activity := strings.TrimSpace(line[70:78])
		if activity != "" {
			if t, err := time.Parse("01/02/06", activity); err == nil {
				rec.ActivityDate = t
			}
		}
	}

	rec.OCCSymbol = fmt.Sprintf(
		"%s%s%s%08d",
		rec.Underlying,
		rec.ExpiryDate.Format("060102"),
		strings.ToUpper(rec.Type[:1]),
		int(rec.Strike*1000+0.5),
	)

	return rec, nil
}

func parseStrike(field string) (float64, error) {
	if len(field) < 8 {
		field = field + strings.Repeat(" ", 8-len(field))
	}

	intPart := strings.TrimSpace(field[0:5])
	decPart := strings.TrimSpace(field[5:8])

	if intPart == "" {
		intPart = "0"
	}
	if decPart == "" {
		decPart = "000"
	}

	strike, err := strconv.ParseFloat(intPart+"."+decPart, 64)
	if err != nil {
		return 0, fmt.Errorf("parse strike %q: %w", field, err)
	}

	return strike, nil
}

func parseOptionType(line string) (string, error) {
	callPut := line[66:67]

	switch callPut {
	case "C", "c":
		return "call", nil
	case "P", "p":
		return "put", nil
	case " ":
		// Puts are often encoded as blank at 66 with "P" at 68.
		if len(line) > 68 && (line[68] == 'P' || line[68] == 'p') {
			return "put", nil
		}
	}

	return "", fmt.Errorf("unknown option type at positions 66/68")
}

// normalizeUnderlying trims padding and strips a leading digit that appears on
// some FLEX/EURO rows where the description crowds the symbol field (e.g. "2AMP" → "AMP").
func normalizeUnderlying(sym string) string {
	sym = strings.TrimSpace(sym)
	sym = strings.TrimLeftFunc(sym, unicode.IsDigit)
	return strings.ToUpper(strings.TrimSpace(sym))
}

// GroupNoForExpiry spreads contracts across weekend download groups 0–3.
func GroupNoForExpiry(expiry time.Time) int {
	week := (expiry.Day() - 1) / 7
	if week > 3 {
		week = 3
	}
	return week
}
