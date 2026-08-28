package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestWeComContactHistoryDigestsBindPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	event := wecomEvent(at)
	event.ID = 1
	before, err := HistoricalWeComExternalContactEventLogDigest(event)
	if err != nil {
		t.Fatal(err)
	}
	event.EventKeyDigest = wecomDigest(99)
	after, _ := HistoricalWeComExternalContactEventLogDigest(event)
	if before == after {
		t.Fatal("private event key omitted from digest")
	}
	follow := wecomFollow(at)
	follow.ID = 1
	before, err = HistoricalWeComExternalContactFollowUserDigest(follow)
	if err != nil {
		t.Fatal(err)
	}
	follow.State = "private-state-drift"
	after, _ = HistoricalWeComExternalContactFollowUserDigest(follow)
	if before == after {
		t.Fatal("private follow state omitted from digest")
	}
	encoded, err := json.Marshal(follow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-state-drift") {
		t.Fatal("private follow state leaked")
	}
}

func TestWeComContactHistoryWriterReplayConflictAndTimeNormalization(t *testing.T) {
	at := time.Date(2026, 8, 28, 9, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store, journal := &wecomHistoryFakeStore{}, &wecomHistoryFakeJournal{}
	writer, err := NewWeComContactHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	event := wecomEvent(at)
	source := hex.EncodeToString(event.SourceKeyDigest[:])
	receipt, err := writer.ImportHistoricalWeComExternalContactEventLog(context.Background(), source, event)
	if err != nil || receipt.Kind != wecomContactEventLogKind || receipt.TargetID != 1 || receipt.Replayed {
		t.Fatalf("first event receipt=%+v err=%v", receipt, err)
	}
	if store.event.CreatedAt.Location() != time.UTC || store.event.CreatedAt.Nanosecond() != 123456000 || !store.event.UpdatedAt.Before(store.event.CreatedAt) {
		t.Fatal("event source time fidelity changed")
	}
	if replay, err := writer.ImportHistoricalWeComExternalContactEventLog(context.Background(), source, event); err != nil || !replay.Replayed {
		t.Fatalf("event replay=%+v err=%v", replay, err)
	}
	event.ErrorMessageDigest = wecomDigest(91)
	if _, err := writer.ImportHistoricalWeComExternalContactEventLog(context.Background(), source, event); !errors.Is(err, contact.ErrWeComContactHistoryConflict) {
		t.Fatalf("event private drift err=%v", err)
	}
	journal.found = false
	follow := wecomFollow(at)
	followSource := hex.EncodeToString(follow.SourceKeyDigest[:])
	if receipt, err := writer.ImportHistoricalWeComExternalContactFollowUser(context.Background(), followSource, follow); err != nil || receipt.Kind != wecomContactFollowUserKind {
		t.Fatalf("follow receipt=%+v err=%v", receipt, err)
	}
	if store.follow.FirstSeenAt.Location() != time.UTC || store.follow.FirstSeenAt.Nanosecond() != 123456000 || store.follow.AddWay == nil || *store.follow.AddWay != -4 || store.follow.CreateTime != nil {
		t.Fatal("follow nullable or time fidelity changed")
	}
	if _, err := writer.ImportHistoricalWeComExternalContactFollowUser(context.Background(), "bad", follow); !errors.Is(err, contact.ErrWeComContactHistoryInvalid) {
		t.Fatalf("bad source err=%v", err)
	}
}

type wecomHistoryFakeStore struct {
	event  contact.HistoricalWeComExternalContactEventLog
	follow contact.HistoricalWeComExternalContactFollowUser
}

func (s *wecomHistoryFakeStore) CreateHistoricalWeComExternalContactEventLog(_ context.Context, v contact.HistoricalWeComExternalContactEventLog) (contact.HistoricalWeComExternalContactEventLog, error) {
	v.ID = 1
	s.event = v
	return v, nil
}
func (s *wecomHistoryFakeStore) GetHistoricalWeComExternalContactEventLog(_ context.Context, id int64) (contact.HistoricalWeComExternalContactEventLog, error) {
	if s.event.ID != id {
		return contact.HistoricalWeComExternalContactEventLog{}, contact.ErrWeComContactHistoryUnavailable
	}
	return s.event, nil
}
func (s *wecomHistoryFakeStore) CreateHistoricalWeComExternalContactFollowUser(_ context.Context, v contact.HistoricalWeComExternalContactFollowUser) (contact.HistoricalWeComExternalContactFollowUser, error) {
	v.ID = 2
	s.follow = v
	return v, nil
}
func (s *wecomHistoryFakeStore) GetHistoricalWeComExternalContactFollowUser(_ context.Context, id int64) (contact.HistoricalWeComExternalContactFollowUser, error) {
	if s.follow.ID != id {
		return contact.HistoricalWeComExternalContactFollowUser{}, contact.ErrWeComContactHistoryUnavailable
	}
	return s.follow, nil
}

type wecomHistoryFakeJournal struct {
	receipt contact.WeComContactHistoryReceipt
	found   bool
}

func (j *wecomHistoryFakeJournal) LoadWeComContactHistory(context.Context, string, string) (contact.WeComContactHistoryReceipt, bool, error) {
	return j.receipt, j.found, nil
}
func (j *wecomHistoryFakeJournal) RecordWeComContactHistory(_ context.Context, receipt contact.WeComContactHistoryReceipt) error {
	j.receipt, j.found = receipt, true
	return nil
}
func wecomDigest(value byte) [32]byte { return sha256.Sum256([]byte{value}) }
func wecomEvent(at time.Time) contact.HistoricalWeComExternalContactEventLog {
	return contact.HistoricalWeComExternalContactEventLog{SourceKeyDigest: wecomDigest(1), SourcePayloadDigest: wecomDigest(2), SourceFieldDigest: wecomDigest(3), SourceID: -7, CorpIDDigest: wecomDigest(4), EventType: "", ChangeType: "", ExternalUserIDDigest: wecomDigest(5), UserIDDigest: wecomDigest(6), EventTime: nil, EventKeyDigest: wecomDigest(7), PayloadXMLDigest: wecomDigest(8), PayloadJSONDigest: wecomDigest(9), ProcessStatus: "", RetryCount: -2, ErrorMessageDigest: wecomDigest(10), CreatedAt: at, UpdatedAt: at.Add(-time.Second), IdentitySyncStatus: "", IdentitySyncErrorCodeDigest: wecomDigest(11), IdentitySyncErrorMessageDigest: wecomDigest(12), IdentitySyncResponseDigest: wecomDigest(13)}
}
func wecomFollow(at time.Time) contact.HistoricalWeComExternalContactFollowUser {
	addWay := int32(-4)
	return contact.HistoricalWeComExternalContactFollowUser{SourceKeyDigest: wecomDigest(14), SourcePayloadDigest: wecomDigest(15), SourceFieldDigest: wecomDigest(16), SourceID: 0, CorpIDDigest: wecomDigest(17), ExternalUserIDDigest: wecomDigest(18), UserIDDigest: wecomDigest(19), RelationStatus: "", IsPrimary: false, RemarkDigest: wecomDigest(20), DescriptionDigest: wecomDigest(21), AddWay: &addWay, State: "private-state", OperUserIDDigest: wecomDigest(22), CreateTime: nil, RawFollowUserDigest: wecomDigest(23), FirstSeenAt: at, LastSeenAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}
