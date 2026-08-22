package app

import (
	"errors"
	"testing"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func TestValidateDestinationURL(t *testing.T) {
	valid := []string{
		"https://example.com",
		"https://Example.COM/path?q=1#section",
		"https://docs.example.com:8443/path",
		"https://xn--fsq.example/path",
	}
	for _, candidate := range valid {
		if err := ValidateDestinationURL(candidate); err != nil {
			t.Errorf("valid %q: %v", candidate, err)
		}
	}
	invalid := []string{
		"",
		"http://example.com",
		"//example.com/path",
		"https://user:pass@example.com",
		"https://localhost/path",
		"https://app.local/path",
		"https://internal/path",
		"https://127.0.0.1/path",
		"https://10.0.0.1/path",
		"https://169.254.1.1/path",
		"https://[::1]/path",
		"https://2130706433/path",
		"https://0177.0.0.1/path",
		"https://0x7f000001/path",
		"https://example.com\\evil",
		"https://example.com/%5Cevil",
		"https://example.com:",
		"https://example.com:0/path",
		"https://example.com:99999/path",
	}
	for _, candidate := range invalid {
		if err := ValidateDestinationURL(candidate); !errors.Is(err, radarport.ErrInvalidArgument) {
			t.Errorf("invalid %q: %v", candidate, err)
		}
	}
}

func TestNormalizeReferencesAndLengths(t *testing.T) {
	zero := int64(0)
	_, err := normalizeCreate(radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            "Name",
		Title:           "Title",
		DestinationURL:  "https://example.com",
		CoverImageID:    &zero,
		ActorID:         1,
		IdempotencyKey:  "radar-validation-001",
	})
	if !errors.Is(err, radarport.ErrInvalidArgument) {
		t.Fatalf("zero reference err=%v", err)
	}
	tooLong := make([]rune, radarport.MaximumNameRunes+1)
	for index := range tooLong {
		tooLong[index] = '名'
	}
	_, err = normalizeCreate(radarport.CreateCommand{
		ExpectedVersion: 0,
		Name:            string(tooLong),
		Title:           "Title",
		DestinationURL:  "https://example.com",
		ActorID:         1,
		IdempotencyKey:  "radar-validation-002",
	})
	if !errors.Is(err, radarport.ErrInvalidArgument) {
		t.Fatalf("long name err=%v", err)
	}
}
