package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestCustomerStateHistoryDigestsBindPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	s := customerStateSnapshot(at)
	s.ID = 1
	before, err := HistoricalCustomerStatusSnapshotDigest(s)
	if err != nil {
		t.Fatal(err)
	}
	s.CustomerNameSnapshot = "changed"
	after, _ := HistoricalCustomerStatusSnapshotDigest(s)
	if before == after {
		t.Fatal("private snapshot fact omitted")
	}
	c := customerStateChange(at)
	c.ID = 1
	before, err = HistoricalCustomerStatusChangeDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	c.UnionID = "changed"
	after, _ = HistoricalCustomerStatusChangeDigest(c)
	if before == after {
		t.Fatal("private change fact omitted")
	}
	m := customerStateTerm(at)
	m.ID = 1
	before, err = HistoricalClassTermTagMappingDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	m.StrategySourceID = "changed"
	after, _ = HistoricalClassTermTagMappingDigest(m)
	if before == after {
		t.Fatal("private term fact omitted")
	}
}
func TestCustomerStateWriterReplayAndSource(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store := &customerStateFakeStore{}
	journal := &customerStateFakeJournal{}
	writer, err := NewCustomerStateHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	v := customerStateSnapshot(at)
	source := hex.EncodeToString(v.SourceKeyDigest[:])
	receipt, err := writer.ImportCustomerStatusSnapshot(context.Background(), source, v)
	if err != nil || receipt.TargetID != 1 || receipt.Replayed {
		t.Fatalf("first=%+v %v", receipt, err)
	}
	if store.snapshot.SetAt.Location() != time.UTC || store.snapshot.SetAt.Nanosecond() != 123456000 || !store.snapshot.UpdatedAt.Before(store.snapshot.CreatedAt) {
		t.Fatal("time fidelity")
	}
	if replay, err := writer.ImportCustomerStatusSnapshot(context.Background(), source, v); err != nil || !replay.Replayed {
		t.Fatalf("replay=%+v %v", replay, err)
	}
	v.CustomerNameSnapshot = "drift"
	if _, err := writer.ImportCustomerStatusSnapshot(context.Background(), source, v); !errors.Is(err, contact.ErrCustomerStateHistoryConflict) {
		t.Fatalf("drift=%v", err)
	}
	if _, err := writer.ImportCustomerStatusSnapshot(context.Background(), "bad", customerStateSnapshot(at)); !errors.Is(err, contact.ErrCustomerStateHistoryInvalid) {
		t.Fatalf("source=%v", err)
	}
	journal.found = false
	c := customerStateChange(at)
	if _, err := writer.ImportCustomerStatusChange(context.Background(), hex.EncodeToString(c.SourceKeyDigest[:]), c); err != nil {
		t.Fatal(err)
	}
	journal.found = false
	m := customerStateTerm(at)
	if _, err := writer.ImportClassTermTagMapping(context.Background(), hex.EncodeToString(m.SourceKeyDigest[:]), m); err != nil {
		t.Fatal(err)
	}
}

type customerStateFakeStore struct {
	snapshot contact.HistoricalCustomerStatusSnapshot
	change   contact.HistoricalCustomerStatusChange
	term     contact.HistoricalClassTermTagMapping
	n        int
}

func (s *customerStateFakeStore) CreateHistoricalCustomerStatusSnapshot(_ context.Context, v contact.HistoricalCustomerStatusSnapshot) (contact.HistoricalCustomerStatusSnapshot, error) {
	s.n++
	v.ID = int64(s.n)
	s.snapshot = v
	return v, nil
}
func (s *customerStateFakeStore) GetHistoricalCustomerStatusSnapshot(_ context.Context, id int64) (contact.HistoricalCustomerStatusSnapshot, error) {
	if s.snapshot.ID != id {
		return contact.HistoricalCustomerStatusSnapshot{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return s.snapshot, nil
}
func (s *customerStateFakeStore) CreateHistoricalCustomerStatusChange(_ context.Context, v contact.HistoricalCustomerStatusChange) (contact.HistoricalCustomerStatusChange, error) {
	v.ID = 1
	s.change = v
	return v, nil
}
func (s *customerStateFakeStore) GetHistoricalCustomerStatusChange(_ context.Context, id int64) (contact.HistoricalCustomerStatusChange, error) {
	if s.change.ID != id {
		return contact.HistoricalCustomerStatusChange{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return s.change, nil
}
func (s *customerStateFakeStore) CreateHistoricalClassTermTagMapping(_ context.Context, v contact.HistoricalClassTermTagMapping) (contact.HistoricalClassTermTagMapping, error) {
	v.ID = 1
	s.term = v
	return v, nil
}
func (s *customerStateFakeStore) GetHistoricalClassTermTagMapping(_ context.Context, id int64) (contact.HistoricalClassTermTagMapping, error) {
	if s.term.ID != id {
		return contact.HistoricalClassTermTagMapping{}, contact.ErrCustomerStateHistoryUnavailable
	}
	return s.term, nil
}

type customerStateFakeJournal struct {
	r     contact.CustomerStateHistoryReceipt
	found bool
}

func (j *customerStateFakeJournal) LoadCustomerStateHistory(context.Context, string, string) (contact.CustomerStateHistoryReceipt, bool, error) {
	return j.r, j.found, nil
}
func (j *customerStateFakeJournal) RecordCustomerStateHistory(_ context.Context, r contact.CustomerStateHistoryReceipt) error {
	j.r = r
	j.found = true
	return nil
}
func customerStateDigest(b byte) [32]byte { return sha256.Sum256([]byte{b}) }
func customerStateSnapshot(at time.Time) contact.HistoricalCustomerStatusSnapshot {
	return contact.HistoricalCustomerStatusSnapshot{SourceKeyDigest: customerStateDigest(1), SourcePayloadDigest: customerStateDigest(2), SourceFieldDigest: customerStateDigest(3), SignupStatus: "", SignupLabelName: "", CustomerNameSnapshot: "customer", OwnerUserIDSnapshot: "owner", SetByUserIDDigest: customerStateDigest(4), SetAt: at, WeComTagSyncStatus: "", WeComTagSyncErrorHash: customerStateDigest(5), StatusFlagsDigest: customerStateDigest(6), CreatedAt: at, UpdatedAt: at.Add(-time.Second), UnionID: "union"}
}
func customerStateChange(at time.Time) contact.HistoricalCustomerStatusChange {
	return contact.HistoricalCustomerStatusChange{SourceKeyDigest: customerStateDigest(7), SourcePayloadDigest: customerStateDigest(8), SourceFieldDigest: customerStateDigest(9), SourceID: -1, OldSignupStatus: "", NewSignupStatus: "", OldLabelName: "", NewLabelName: "", CustomerNameSnapshot: "customer", OwnerUserIDSnapshot: "owner", SetByUserIDDigest: customerStateDigest(10), SetAt: at, WeComTagSyncStatus: "", WeComTagSyncErrorHash: customerStateDigest(11), StatusFlagsDigest: customerStateDigest(12), CreatedAt: at, UnionID: "union"}
}
func customerStateTerm(at time.Time) contact.HistoricalClassTermTagMapping {
	return contact.HistoricalClassTermTagMapping{SourceKeyDigest: customerStateDigest(13), SourcePayloadDigest: customerStateDigest(14), SourceFieldDigest: customerStateDigest(15), SourceID: -2, TagGroupName: "", TagName: "", ClassTermNo: -3, ClassTermLabel: "", OriginalActive: false, CreatedAt: at, UpdatedAt: at.Add(-time.Second), StrategySourceID: "strategy", GroupSourceID: "group", TagSourceID: "tag"}
}
