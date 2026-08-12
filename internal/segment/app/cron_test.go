package app

import (
	"errors"
	"testing"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestCanonicalRefreshCron(t *testing.T) {
	value := func(raw string) *string { return &raw }
	tests := []struct {
		name      string
		mode      segmentport.RefreshMode
		input     *string
		want      *string
		wantError bool
	}{
		{name: "manual nil", mode: segmentport.RefreshModeManual},
		{name: "manual cron rejected", mode: segmentport.RefreshModeManual, input: value("0 0 * * *"), wantError: true},
		{name: "scheduled canonical wildcard", mode: segmentport.RefreshModeScheduled, input: value("0 0 * * *"), want: value("0 0 * * *")},
		{name: "scheduled whitespace and leading zeros", mode: segmentport.RefreshModeScheduled, input: value("  */15\t09  01,05 * * "), want: value("*/15 9 1,5 * *")},
		{name: "scheduled full range normalizes", mode: segmentport.RefreshModeScheduled, input: value("0-59/1 0-23 1-31 1-12 0-6"), want: value("* * * * *")},
		{name: "field range does not widen", mode: segmentport.RefreshModeScheduled, input: value("0-6 0 * * *"), want: value("0-6 0 * * *")},
		{name: "scheduled range step", mode: segmentport.RefreshModeScheduled, input: value("5-55/10 8-18/2 * 1,06 *"), want: value("5-55/10 8-18/2 * 1,6 *")},
		{name: "missing scheduled cron", mode: segmentport.RefreshModeScheduled, wantError: true},
		{name: "empty scheduled cron", mode: segmentport.RefreshModeScheduled, input: value(""), wantError: true},
		{name: "unknown mode", mode: segmentport.RefreshMode("hourly"), input: value("0 0 * * *"), wantError: true},
		{name: "macro rejected", mode: segmentport.RefreshModeScheduled, input: value("@daily"), wantError: true},
		{name: "six fields rejected", mode: segmentport.RefreshModeScheduled, input: value("0 0 0 * * *"), wantError: true},
		{name: "named month rejected", mode: segmentport.RefreshModeScheduled, input: value("0 0 * jan *"), wantError: true},
		{name: "question mark rejected", mode: segmentport.RefreshModeScheduled, input: value("0 0 ? * *"), wantError: true},
		{name: "negative rejected", mode: segmentport.RefreshModeScheduled, input: value("-1 0 * * *"), wantError: true},
		{name: "out of range rejected", mode: segmentport.RefreshModeScheduled, input: value("60 0 * * *"), wantError: true},
		{name: "descending range rejected", mode: segmentport.RefreshModeScheduled, input: value("5-1 0 * * *"), wantError: true},
		{name: "single value step rejected", mode: segmentport.RefreshModeScheduled, input: value("1/2 0 * * *"), wantError: true},
		{name: "zero step rejected", mode: segmentport.RefreshModeScheduled, input: value("*/0 0 * * *"), wantError: true},
		{name: "oversized step rejected", mode: segmentport.RefreshModeScheduled, input: value("*/61 0 * * *"), wantError: true},
		{name: "duplicate list value rejected", mode: segmentport.RefreshModeScheduled, input: value("1,1 0 * * *"), wantError: true},
		{name: "overlapping list range rejected", mode: segmentport.RefreshModeScheduled, input: value("1-3,3-5 0 * * *"), wantError: true},
		{name: "ambiguous day fields rejected", mode: segmentport.RefreshModeScheduled, input: value("0 0 1 * 1"), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CanonicalRefreshCron(test.mode, test.input)
			if test.wantError {
				if !errors.Is(err, ErrInvalidRefreshSchedule) {
					t.Fatalf("CanonicalRefreshCron() error = %v, want invalid schedule", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.want == nil {
				if got != nil {
					t.Fatalf("CanonicalRefreshCron() = %q, want nil", *got)
				}
				return
			}
			if got == nil || *got != *test.want {
				t.Fatalf("CanonicalRefreshCron() = %v, want %q", got, *test.want)
			}
		})
	}
}

func FuzzCanonicalRefreshCron(f *testing.F) {
	for _, seed := range []string{"0 0 * * *", "*/15 9 * * 1-5", "@daily", "1,1 0 * * *"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		canonical, err := CanonicalRefreshCron(segmentport.RefreshModeScheduled, &raw)
		if err != nil {
			return
		}
		again, err := CanonicalRefreshCron(segmentport.RefreshModeScheduled, canonical)
		if err != nil || again == nil || *again != *canonical {
			t.Fatalf("canonical cron did not round-trip: %q -> %v, %v", raw, again, err)
		}
	})
}
