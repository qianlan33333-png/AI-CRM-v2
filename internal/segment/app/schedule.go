package app

import (
	"strings"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// RefreshCronMatches reports whether the closed S04B cron grammar selects the
// UTC minute containing reference. Invalid stored configuration never runs a
// refresh: callers receive ErrInvalidRefreshSchedule and must fail closed.
func RefreshCronMatches(refreshCron string, reference time.Time) (bool, error) {
	canonical, err := CanonicalRefreshCron(segmentport.RefreshModeScheduled, &refreshCron)
	if err != nil || canonical == nil || reference.IsZero() {
		return false, ErrInvalidRefreshSchedule
	}
	fields := strings.Fields(*canonical)
	if len(fields) != 5 {
		return false, ErrInvalidRefreshSchedule
	}
	instant := reference.UTC()
	values := [5]int{instant.Minute(), instant.Hour(), instant.Day(), int(instant.Month()), int(instant.Weekday())}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for index, field := range fields {
		matches, err := cronFieldMatches(field, values[index], ranges[index][0], ranges[index][1])
		if err != nil || !matches {
			return matches, err
		}
	}
	return true, nil
}

func cronFieldMatches(field string, value, minimum, maximum int) (bool, error) {
	for _, rawTerm := range strings.Split(field, ",") {
		term, err := parseCronTerm(rawTerm, minimum, maximum)
		if err != nil {
			return false, err
		}
		if value >= term.start && value <= term.end && (value-term.start)%term.step == 0 {
			return true, nil
		}
	}
	return false, nil
}
