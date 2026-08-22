package domain

import (
	"strings"
	"testing"
	"time"
)

func TestMemberRejectsInvalidStateTimesWithoutPretendingToSanitizeFreeText(t *testing.T) {
	now := time.Date(2026, 8, 23, 2, 0, 0, 0, time.UTC)
	freeText := "mobile course follow-up"
	member := Member{MemberRef: "spm_AAAAAAAAAAAAAAAAAAAAAA", ServiceProductID: 1, CustomerID: 2, State: StateActive, Source: SourceManual, StartsAt: now, Remark: &freeText, Version: 1, CreatedAt: now, UpdatedAt: now}
	if !member.Valid() {
		t.Fatal("bounded free text must not be heuristically classified")
	}
	member.State = StateExpired
	if member.Valid() {
		t.Fatal("expired member without expired_at must be rejected")
	}
	member.ExpiredAt = &now
	if !member.Valid() {
		t.Fatal("valid expired member rejected")
	}
}

func TestOptionalTextIsBoundedAndCanonical(t *testing.T) {
	tooLong := strings.Repeat("好", MaximumAllianceRunes+1)
	for _, value := range []*string{ptr(" padded"), &tooLong, ptr("")} {
		if ValidOptionalText(value, MaximumAllianceRunes) {
			t.Fatalf("unsafe value accepted: %q", *value)
		}
	}
}

func ptr(value string) *string { return &value }
