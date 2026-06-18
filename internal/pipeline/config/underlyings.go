package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
)

var AllowedUnderlyings = map[string]struct{}{}

func LoadAllowedUnderlyings(
	path string,
) error {

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf(
			"open allowed underlyings file %s: %w",
			path,
			err,
		)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		line := strings.TrimSpace(
			scanner.Text(),
		)

		// skip empty lines
		if line == "" {
			continue
		}

		// skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		ticker := strings.ToUpper(line)

		AllowedUnderlyings[ticker] = struct{}{}
	}

	logger.Infof(
		"loaded allowed underlyings=%d",
		len(AllowedUnderlyings),
	)

	if err := scanner.Err(); err != nil {
		return fmt.Errorf(
			"scan allowed underlyings file %s: %w",
			path,
			err,
		)
	}

	return nil
}

func IsAllowedUnderlying(
	ticker string,
) bool {
	_, ok := AllowedUnderlyings[strings.ToUpper(ticker)]
	return ok
}

func (f *DXFloat) UnmarshalJSON(data []byte) error {

	s := strings.TrimSpace(string(data))

	if s == `"NaN"` || s == "null" {
		*f = DXFloat(math.NaN())
		return nil
	}

	var v float64

	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	*f = DXFloat(v)

	return nil
}
