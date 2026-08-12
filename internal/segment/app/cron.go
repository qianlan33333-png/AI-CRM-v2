package app

import (
	"errors"
	"strconv"
	"strings"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var ErrInvalidRefreshSchedule = errors.New("invalid segment refresh schedule")

// CanonicalRefreshCron validates the closed S04B five-field numeric cron
// grammar and returns its stable spelling. It only validates stored Segment
// configuration; registering a River periodic job remains S04E work.
func CanonicalRefreshCron(mode segmentport.RefreshMode, refreshCron *string) (*string, error) {
	switch mode {
	case segmentport.RefreshModeManual:
		if refreshCron != nil {
			return nil, ErrInvalidRefreshSchedule
		}
		return nil, nil
	case segmentport.RefreshModeScheduled:
		if refreshCron == nil {
			return nil, ErrInvalidRefreshSchedule
		}
		canonical, err := canonicalCron(*refreshCron)
		if err != nil {
			return nil, ErrInvalidRefreshSchedule
		}
		return &canonical, nil
	default:
		return nil, ErrInvalidRefreshSchedule
	}
}

func canonicalCron(raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) != 5 {
		return "", ErrInvalidRefreshSchedule
	}

	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	canonical := make([]string, len(fields))
	for index, field := range fields {
		value, err := canonicalCronField(field, ranges[index][0], ranges[index][1])
		if err != nil {
			return "", err
		}
		canonical[index] = value
	}
	// Different cron engines disagree about the meaning of a restricted
	// day-of-month plus day-of-week. Keep the stored grammar unambiguous.
	if canonical[2] != "*" && canonical[4] != "*" {
		return "", ErrInvalidRefreshSchedule
	}
	return strings.Join(canonical, " "), nil
}

func canonicalCronField(raw string, minimum, maximum int) (string, error) {
	if raw == "" {
		return "", ErrInvalidRefreshSchedule
	}
	seen := make([]bool, maximum-minimum+1)
	terms := strings.Split(raw, ",")
	canonical := make([]string, 0, len(terms))
	for _, rawTerm := range terms {
		term, err := parseCronTerm(rawTerm, minimum, maximum)
		if err != nil {
			return "", err
		}
		for value := term.start; value <= term.end; value += term.step {
			position := value - minimum
			if seen[position] {
				return "", ErrInvalidRefreshSchedule
			}
			seen[position] = true
		}
		canonical = append(canonical, term.String(minimum, maximum))
	}
	return strings.Join(canonical, ","), nil
}

type cronTerm struct {
	start   int
	end     int
	step    int
	hasStep bool
}

func (term cronTerm) String(minimum, maximum int) string {
	base := strconv.Itoa(term.start)
	if term.start == minimum && term.end == maximum {
		base = "*"
	} else if term.start != term.end {
		base += "-" + strconv.Itoa(term.end)
	}
	if term.hasStep && term.step != 1 {
		return base + "/" + strconv.Itoa(term.step)
	}
	return base
}

func parseCronTerm(raw string, minimum, maximum int) (cronTerm, error) {
	parts := strings.Split(raw, "/")
	if len(parts) > 2 || raw == "" || parts[0] == "" {
		return cronTerm{}, ErrInvalidRefreshSchedule
	}
	term, rangeOrStar, err := parseCronBase(parts[0], minimum, maximum)
	if err != nil {
		return cronTerm{}, err
	}
	if len(parts) == 1 {
		return term, nil
	}
	if !rangeOrStar || parts[1] == "" {
		return cronTerm{}, ErrInvalidRefreshSchedule
	}
	step, err := parseCronNumber(parts[1], 1, term.end-term.start+1)
	if err != nil {
		return cronTerm{}, err
	}
	term.step = step
	term.hasStep = true
	return term, nil
}

func parseCronBase(raw string, minimum, maximum int) (cronTerm, bool, error) {
	if raw == "*" {
		return cronTerm{start: minimum, end: maximum, step: 1}, true, nil
	}
	parts := strings.Split(raw, "-")
	if len(parts) == 1 {
		value, err := parseCronNumber(parts[0], minimum, maximum)
		return cronTerm{start: value, end: value, step: 1}, false, err
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return cronTerm{}, false, ErrInvalidRefreshSchedule
	}
	start, err := parseCronNumber(parts[0], minimum, maximum)
	if err != nil {
		return cronTerm{}, false, err
	}
	end, err := parseCronNumber(parts[1], minimum, maximum)
	if err != nil || start > end {
		return cronTerm{}, false, ErrInvalidRefreshSchedule
	}
	return cronTerm{start: start, end: end, step: 1}, true, nil
}

func parseCronNumber(raw string, minimum, maximum int) (int, error) {
	if raw == "" {
		return 0, ErrInvalidRefreshSchedule
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, ErrInvalidRefreshSchedule
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, ErrInvalidRefreshSchedule
	}
	return value, nil
}
