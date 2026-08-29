package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHXCMemberUsageHistoryWriterReplaysAndRejectsTargetDrift(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*60*60))
	store := &memberUsageStoreFake{}
	journal := &memberUsageJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}}
	writer, err := NewHXCMemberUsageHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := memberUsageHistoryFixture(1, at)
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	receipt, err := writer.ImportMemberUsage(context.Background(), source, value)
	if err != nil || receipt.Kind != hxc.HXCHistoryMemberUsage || receipt.Replayed || receipt.TargetID < 1 {
		t.Fatalf("first import = %#v, %v", receipt, err)
	}
	stored := store.values[receipt.TargetID]
	if stored.ProjectedAt.Location() != time.UTC || stored.ProjectedAt.Nanosecond()%1000 != 0 || string(stored.PayloadJSON) != "null" {
		t.Fatalf("stored normalization/raw JSON = %#v", stored)
	}
	replay, err := writer.ImportMemberUsage(context.Background(), source, value)
	if err != nil || !replay.Replayed || replay.TargetDigest != receipt.TargetDigest || store.creates != 1 {
		t.Fatalf("replay = %#v, creates=%d, err=%v", replay, store.creates, err)
	}
	stored.MobileHash += "-drift"
	store.values[receipt.TargetID] = stored
	if _, err := writer.ImportMemberUsage(context.Background(), source, value); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
		t.Fatalf("target private drift = %v", err)
	}
}

func TestHistoricalHXCMemberUsageDigestCoversAllFields(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	value := normalizeHXCMemberUsage(memberUsageHistoryFixture(3, at))
	value.ID = 1
	base, err := HistoricalHXCMemberUsageDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	changedTime := at.Add(time.Second)
	for name, mutate := range map[string]func(*hxc.HistoricalHXCMemberUsage){
		"id":                  func(v *hxc.HistoricalHXCMemberUsage) { v.ID++ },
		"source key":          func(v *hxc.HistoricalHXCMemberUsage) { v.SourceKeyDigest[0]++ },
		"source payload":      func(v *hxc.HistoricalHXCMemberUsage) { v.SourcePayloadDigest[0]++ },
		"source field":        func(v *hxc.HistoricalHXCMemberUsage) { v.SourceFieldDigest[0]++ },
		"generation":          func(v *hxc.HistoricalHXCMemberUsage) { v.Generation-- },
		"union":               func(v *hxc.HistoricalHXCMemberUsage) { v.UnionID += "x" },
		"owner":               func(v *hxc.HistoricalHXCMemberUsage) { v.OwnerUserID += "x" },
		"mobile":              func(v *hxc.HistoricalHXCMemberUsage) { v.MobileHash += "x" },
		"member":              func(v *hxc.HistoricalHXCMemberUsage) { v.IsMember = !v.IsMember },
		"registered":          func(v *hxc.HistoricalHXCMemberUsage) { v.IsRegistered = !v.IsRegistered },
		"registered at":       func(v *hxc.HistoricalHXCMemberUsage) { v.RegisteredAt = &changedTime },
		"usage":               func(v *hxc.HistoricalHXCMemberUsage) { v.HasRealUsage = !v.HasRealUsage },
		"first used":          func(v *hxc.HistoricalHXCMemberUsage) { v.FirstUsedAt = &changedTime },
		"last used":           func(v *hxc.HistoricalHXCMemberUsage) { v.LastUsedAt = &changedTime },
		"member since":        func(v *hxc.HistoricalHXCMemberUsage) { v.MemberSince = &changedTime },
		"expires":             func(v *hxc.HistoricalHXCMemberUsage) { v.MembershipExpiresAt = &changedTime },
		"tier":                func(v *hxc.HistoricalHXCMemberUsage) { v.MembershipTier += "x" },
		"status":              func(v *hxc.HistoricalHXCMemberUsage) { v.MembershipStatus += "x" },
		"membership source":   func(v *hxc.HistoricalHXCMemberUsage) { v.MembershipSource += "x" },
		"registration source": func(v *hxc.HistoricalHXCMemberUsage) { v.RegistrationSource += "x" },
		"usage source":        func(v *hxc.HistoricalHXCMemberUsage) { v.UsageSource += "x" },
		"updated":             func(v *hxc.HistoricalHXCMemberUsage) { v.UpdatedAt = &changedTime },
		"payload JSON":        func(v *hxc.HistoricalHXCMemberUsage) { v.PayloadJSON = json.RawMessage(`{"changed":true}`) },
		"projected":           func(v *hxc.HistoricalHXCMemberUsage) { v.ProjectedAt = changedTime },
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneMemberUsage(value)
			mutate(&changed)
			digest, err := HistoricalHXCMemberUsageDigest(changed)
			if err != nil || digest == base {
				t.Fatalf("digest did not cover %s: %v", name, err)
			}
		})
	}
}

func TestHXCMemberUsageHistoryWriterRejectsInvalidInput(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	store := &memberUsageStoreFake{}
	journal := &memberUsageJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}}
	writer, err := NewHXCMemberUsageHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := memberUsageHistoryFixture(4, at)
	for name, mutate := range map[string]func(*hxc.HistoricalHXCMemberUsage){
		"zero field":  func(v *hxc.HistoricalHXCMemberUsage) { v.SourceFieldDigest = [32]byte{} },
		"nil JSON":    func(v *hxc.HistoricalHXCMemberUsage) { v.PayloadJSON = nil },
		"bad JSON":    func(v *hxc.HistoricalHXCMemberUsage) { v.PayloadJSON = json.RawMessage("{") },
		"NUL private": func(v *hxc.HistoricalHXCMemberUsage) { v.UnionID = "bad\x00" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := cloneMemberUsage(value)
			mutate(&invalid)
			if _, err := writer.ImportMemberUsage(context.Background(), hex.EncodeToString(invalid.SourceKeyDigest[:]), invalid); !errors.Is(err, hxc.ErrHXCHistoryInvalid) || store.creates != 0 {
				t.Fatalf("invalid %s = %v, creates=%d", name, err, store.creates)
			}
		})
	}
	if _, err := writer.ImportMemberUsage(context.Background(), "wrong", value); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatalf("wrong source = %v", err)
	}
}

type memberUsageStoreFake struct {
	next, creates int64
	values        map[int64]hxc.HistoricalHXCMemberUsage
}

func (s *memberUsageStoreFake) CreateHistoricalHXCMemberUsage(_ context.Context, value hxc.HistoricalHXCMemberUsage) (hxc.HistoricalHXCMemberUsage, error) {
	if s.values == nil {
		s.values = map[int64]hxc.HistoricalHXCMemberUsage{}
	}
	s.next++
	s.creates++
	value.ID = s.next
	s.values[value.ID] = cloneMemberUsage(value)
	return value, nil
}

func (s *memberUsageStoreFake) GetHistoricalHXCMemberUsage(_ context.Context, id int64) (hxc.HistoricalHXCMemberUsage, error) {
	value, ok := s.values[id]
	if !ok {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryUnavailable
	}
	return cloneMemberUsage(value), nil
}

type memberUsageJournalFake struct {
	entries map[string]hxc.HXCHistoryReceipt
}

func (j *memberUsageJournalFake) LoadHXCHistory(_ context.Context, kind, source string) (hxc.HXCHistoryReceipt, bool, error) {
	value, ok := j.entries[kind+":"+source]
	return value, ok, nil
}

func (j *memberUsageJournalFake) RecordHXCHistory(_ context.Context, value hxc.HXCHistoryReceipt) error {
	j.entries[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func memberUsageHistoryFixture(first byte, at time.Time) hxc.HistoricalHXCMemberUsage {
	digest := func(offset byte) [32]byte { return sha256.Sum256([]byte{first, offset}) }
	registered := at.Add(-time.Hour)
	firstUsed := at.Add(-2 * time.Hour)
	lastUsed := at.Add(-3 * time.Hour)
	memberSince := at.Add(-4 * time.Hour)
	expires := at.Add(24 * time.Hour)
	updated := at.Add(-5 * time.Hour)
	return hxc.HistoricalHXCMemberUsage{
		SourceKeyDigest: digest(1), SourcePayloadDigest: digest(2), SourceFieldDigest: digest(3), Generation: -7,
		UnionID: "union-private", OwnerUserID: "owner-private", MobileHash: "mobile-private", IsMember: false, IsRegistered: true, RegisteredAt: &registered,
		HasRealUsage: true, FirstUsedAt: &firstUsed, LastUsedAt: &lastUsed, MemberSince: &memberSince, MembershipExpiresAt: &expires,
		MembershipTier: "tier", MembershipStatus: "status", MembershipSource: "membership", RegistrationSource: "registration", UsageSource: "usage", UpdatedAt: &updated,
		PayloadJSON: json.RawMessage("null"), ProjectedAt: at,
	}
}

func cloneMemberUsage(value hxc.HistoricalHXCMemberUsage) hxc.HistoricalHXCMemberUsage {
	value.PayloadJSON = append(json.RawMessage(nil), value.PayloadJSON...)
	for original, destination := range map[*time.Time]**time.Time{
		value.RegisteredAt: &value.RegisteredAt, value.FirstUsedAt: &value.FirstUsedAt, value.LastUsedAt: &value.LastUsedAt,
		value.MemberSince: &value.MemberSince, value.MembershipExpiresAt: &value.MembershipExpiresAt, value.UpdatedAt: &value.UpdatedAt,
	} {
		if original != nil {
			copy := *original
			*destination = &copy
		}
	}
	return value
}
