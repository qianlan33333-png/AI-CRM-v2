package store

import (
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

func TestParseTagEffectIDIsCanonicalAndInt64Bounded(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  int64
	}{
		{value: "eer_1", want: 1},
		{value: "eer_9223372036854775807", want: 9223372036854775807},
	} {
		got, err := parseTagEffectID(test.value)
		if err != nil || got != test.want {
			t.Fatalf("parseTagEffectID(%q) = %d, %v; want %d", test.value, got, err, test.want)
		}
	}
	for _, value := range []string{"", "1", "eer_", "eer_0", "eer_01", "eer_+1", "eer_-1", "eer_ 1", "eer_1 ", "eer_9223372036854775808"} {
		if _, err := parseTagEffectID(value); !errors.Is(err, tag.ErrInvalidCommand) {
			t.Fatalf("parseTagEffectID(%q) error = %v, want ErrInvalidCommand", value, err)
		}
	}
}

func TestParseTagReceiptIDRejectsEffectIDs(t *testing.T) {
	t.Parallel()
	if got, err := parseTagReceiptID("eerop_9"); err != nil || got != 9 {
		t.Fatalf("parseTagReceiptID() = %d, %v", got, err)
	}
	if _, err := parseTagReceiptID("eer_9"); !errors.Is(err, tag.ErrInvalidCommand) {
		t.Fatalf("parseTagReceiptID(effect ID) error = %v", err)
	}
}
