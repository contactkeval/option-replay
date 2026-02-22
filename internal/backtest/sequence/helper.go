package sequence

import (
	"fmt"
	"time"
)

// --------------------------------------------------------------------------------------------
// Helper functions
// --------------------------------------------------------------------------------------------

// CombineDateTime combines a date, time-of-day (HH:MM),
// and timezone into a time.Time
func CombineDateTime(
	day time.Time,
	timeOfDay string,
	timeZone string,
) (time.Time, error) {

	// Load timezone
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone: %w", err)
	}

	// Parse HH:MM
	parsedTime, err := time.Parse("15:04", timeOfDay)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timeOfDay format (HH:MM): %w", err)
	}

	// Construct final time
	result := time.Date(
		day.Year(),
		day.Month(),
		day.Day(),
		parsedTime.Hour(),
		parsedTime.Minute(),
		0,
		0,
		loc,
	)

	return result, nil
}

func deduplicateDates(dates []time.Time) []time.Time {
	seen := map[string]bool{}
	var final []time.Time
	for _, d := range dates {
		k := d.Format("2006-01-02")
		if !seen[k] {
			final = append(final, d)
			seen[k] = true
		}
	}
	return final
}

func intSliceContains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
