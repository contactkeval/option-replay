// Package legsum evaluates arithmetic expressions over per-leg values,
// e.g. "leg1+leg2", "leg1-leg2+leg3".
//
// The input slice is indexed 0-based (index 0 = leg1); expressions reference
// legs 1-based ("leg<N>").
package legsum

import (
	"fmt"
	"strconv"
	"strings"
)

// Eval parses expr and evaluates it against legValues (index 0 = leg1).
//
// Supported tokens are "leg<N>" (1-based index) and the "+" and "-" operators.
// The wrapper abs(...) returns the absolute value of the enclosed expression.
// An error is returned for empty expressions or out-of-range leg references.
func Eval(expr string, legValues []float64) (float64, error) {
	s := strings.ReplaceAll(expr, " ", "")
	if s == "" {
		return 0, fmt.Errorf("empty leg expression")
	}

	// Handle abs(...) wrapper
	if strings.HasPrefix(s, "abs(") && strings.HasSuffix(s, ")") && len(s) > 5 {
		v, err := Eval(s[4:len(s)-1], legValues)
		if err != nil {
			return 0, err
		}
		if v < 0 {
			return -v, nil
		}
		return v, nil
	}

	i := 0
	sign := 1.0
	var total float64
	first := true
	leadingSign := false
	lastWasLeg := false
	for i < len(s) {
		c := s[i]
		if c == '+' || c == '-' {
			if first {
				if leadingSign {
					return 0, fmt.Errorf("invalid leading operator at position %d in %q", i, expr)
				}
				leadingSign = true
				if c == '-' {
					sign = -1.0
				}
				i++
				continue
			}
			if !lastWasLeg {
				return 0, fmt.Errorf("invalid operator at position %d in %q", i, expr)
			}
			sign = 1.0
			if c == '-' {
				sign = -1.0
			}
			lastWasLeg = false
			i++
			continue
		}

		if !strings.HasPrefix(s[i:], "leg") {
			return 0, fmt.Errorf("invalid token at position %d in %q", i, expr)
		}
		i += 3
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			return 0, fmt.Errorf("missing leg number in %q", expr)
		}
		n, err := strconv.Atoi(s[i:j])
		if err != nil {
			return 0, fmt.Errorf("invalid leg number in %q: %w", expr, err)
		}
		i = j
		if n < 1 || n > len(legValues) {
			return 0, fmt.Errorf("leg%d out of range (have %d legs)", n, len(legValues))
		}
		total += sign * legValues[n-1]
		first = false
		lastWasLeg = true
	}
	if !lastWasLeg {
		return 0, fmt.Errorf("incomplete expression %q", expr)
	}
	return total, nil
}
