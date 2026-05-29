package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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
