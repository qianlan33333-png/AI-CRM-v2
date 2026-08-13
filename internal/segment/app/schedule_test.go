package app

import (
	"errors"
	"testing"
	"time"
)

func TestRefreshCronMatchesClosedGrammarAtUTCMinute(t *testing.T) {
	reference := time.Date(2026, time.August, 13, 9, 15, 47, 0, time.FixedZone("UTC+8", 8*60*60))
	for _, test := range []struct {
		cron string
		want bool
	}{
		{cron: "15 1 13 8 *", want: true},
		{cron: "*/15 1 * * 4", want: true},
		{cron: "14 1 * * *", want: false},
		{cron: "15 1 12 8 *", want: false},
	} {
		got, err := RefreshCronMatches(test.cron, reference)
		if err != nil || got != test.want {
			t.Fatalf("RefreshCronMatches(%q) = %v, %v; want %v, nil", test.cron, got, err, test.want)
		}
	}
	if _, err := RefreshCronMatches("@daily", reference); !errors.Is(err, ErrInvalidRefreshSchedule) {
		t.Fatalf("RefreshCronMatches(invalid) error = %v, want invalid schedule", err)
	}
}
