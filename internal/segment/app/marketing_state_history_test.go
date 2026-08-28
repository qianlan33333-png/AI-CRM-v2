package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestMarketingStateHistoryWriterPreservesSignedAndPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
	store, journal := &marketingStateTestStore{}, &marketingStateTestJournal{}
	w, err := NewMarketingStateHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := marketingSnapshot(at)
	snapshot.SourceID = -10
	person, batch := int64(-2), int64(-3)
	snapshot.PersonSourceID, snapshot.LastBatchSourceID = &person, &batch
	before, err := HistoricalMarketingStateSnapshotDigest(withMarketingStateSnapshotID(normalizeMarketingStateSnapshot(snapshot), 1))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.ExternalUserIDDigest = marketingDigest(99)
	after, err := HistoricalMarketingStateSnapshotDigest(withMarketingStateSnapshotID(normalizeMarketingStateSnapshot(snapshot), 1))
	if err != nil || before == after {
		t.Fatalf("private digest ignored: %v", err)
	}
	snapshot.ExternalUserIDDigest = marketingDigest(4)
	receipt, err := w.ImportMarketingStateSnapshot(context.Background(), hex.EncodeToString(snapshot.SourceKeyDigest[:]), snapshot)
	if err != nil || receipt.Replayed || store.snapshot.SourceID != -10 || *store.snapshot.PersonSourceID != -2 || store.snapshot.CreatedAt.Location() != time.UTC || store.snapshot.CreatedAt.Nanosecond() != 123456000 {
		t.Fatalf("receipt=%+v stored=%+v err=%v", receipt, store.snapshot, err)
	}
	if _, err := w.ImportMarketingStateSnapshot(context.Background(), hex.EncodeToString(snapshot.SourceKeyDigest[:]), snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.StatePayloadDigest = marketingDigest(100)
	if _, err := w.ImportMarketingStateSnapshot(context.Background(), hex.EncodeToString(snapshot.SourceKeyDigest[:]), snapshot); !errors.Is(err, segment.ErrMarketingStateHistoryConflict) {
		t.Fatalf("drift=%v", err)
	}

	journal.found = false
	change := marketingChange(at)
	change.SourceID = -11
	if _, err := w.ImportMarketingStateChange(context.Background(), hex.EncodeToString(change.SourceKeyDigest[:]), change); err != nil {
		t.Fatal(err)
	}
	journal.found = false
	valueSnapshot := valueSnapshot(at)
	valueSnapshot.SourceID = -12
	if _, err := w.ImportValueSegmentSnapshot(context.Background(), hex.EncodeToString(valueSnapshot.SourceKeyDigest[:]), valueSnapshot); err != nil {
		t.Fatal(err)
	}
	journal.found = false
	valueChange := valueChange(at)
	valueChange.SourceID = -13
	if _, err := w.ImportValueSegmentChange(context.Background(), hex.EncodeToString(valueChange.SourceKeyDigest[:]), valueChange); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ImportValueSegmentChange(context.Background(), "bad", valueChange); !errors.Is(err, segment.ErrMarketingStateHistoryInvalid) {
		t.Fatal(err)
	}
}

func TestMarketingStateHistoryRejectsMissingEnvelopeAndNormalizesNullable(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	v := marketingSnapshot(at)
	v.SourceFieldDigest = [32]byte{}
	if _, err := HistoricalMarketingStateSnapshotDigest(withMarketingStateSnapshotID(v, 1)); !errors.Is(err, segment.ErrMarketingStateHistoryInvalid) {
		t.Fatal(err)
	}
	v = marketingSnapshot(at)
	v.EnteredAt, v.ExitedAt = nil, nil
	if _, err := HistoricalMarketingStateSnapshotDigest(withMarketingStateSnapshotID(v, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMarketingStateHistoryWriter(nil, &marketingStateTestJournal{}); !errors.Is(err, segment.ErrMarketingStateHistoryUnavailable) {
		t.Fatal(err)
	}
}

type marketingStateTestStore struct {
	snapshot      segment.HistoricalMarketingStateSnapshot
	change        segment.HistoricalMarketingStateChange
	valueSnapshot segment.HistoricalValueSegmentSnapshot
	valueChange   segment.HistoricalValueSegmentChange
	next          int64
}

func (s *marketingStateTestStore) id() int64 { s.next++; return s.next }
func (s *marketingStateTestStore) CreateHistoricalMarketingStateSnapshot(_ context.Context, v segment.HistoricalMarketingStateSnapshot) (segment.HistoricalMarketingStateSnapshot, error) {
	v.ID = s.id()
	s.snapshot = v
	return v, nil
}
func (s *marketingStateTestStore) GetHistoricalMarketingStateSnapshot(_ context.Context, id int64) (segment.HistoricalMarketingStateSnapshot, error) {
	if s.snapshot.ID != id {
		return segment.HistoricalMarketingStateSnapshot{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return s.snapshot, nil
}
func (s *marketingStateTestStore) CreateHistoricalMarketingStateChange(_ context.Context, v segment.HistoricalMarketingStateChange) (segment.HistoricalMarketingStateChange, error) {
	v.ID = s.id()
	s.change = v
	return v, nil
}
func (s *marketingStateTestStore) GetHistoricalMarketingStateChange(_ context.Context, id int64) (segment.HistoricalMarketingStateChange, error) {
	if s.change.ID != id {
		return segment.HistoricalMarketingStateChange{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return s.change, nil
}
func (s *marketingStateTestStore) CreateHistoricalValueSegmentSnapshot(_ context.Context, v segment.HistoricalValueSegmentSnapshot) (segment.HistoricalValueSegmentSnapshot, error) {
	v.ID = s.id()
	s.valueSnapshot = v
	return v, nil
}
func (s *marketingStateTestStore) GetHistoricalValueSegmentSnapshot(_ context.Context, id int64) (segment.HistoricalValueSegmentSnapshot, error) {
	if s.valueSnapshot.ID != id {
		return segment.HistoricalValueSegmentSnapshot{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return s.valueSnapshot, nil
}
func (s *marketingStateTestStore) CreateHistoricalValueSegmentChange(_ context.Context, v segment.HistoricalValueSegmentChange) (segment.HistoricalValueSegmentChange, error) {
	v.ID = s.id()
	s.valueChange = v
	return v, nil
}
func (s *marketingStateTestStore) GetHistoricalValueSegmentChange(_ context.Context, id int64) (segment.HistoricalValueSegmentChange, error) {
	if s.valueChange.ID != id {
		return segment.HistoricalValueSegmentChange{}, segment.ErrMarketingStateHistoryUnavailable
	}
	return s.valueChange, nil
}

type marketingStateTestJournal struct {
	receipt segment.MarketingStateHistoryReceipt
	found   bool
}

func (j *marketingStateTestJournal) LoadMarketingStateHistory(context.Context, string, string) (segment.MarketingStateHistoryReceipt, bool, error) {
	return j.receipt, j.found, nil
}
func (j *marketingStateTestJournal) RecordMarketingStateHistory(_ context.Context, r segment.MarketingStateHistoryReceipt) error {
	j.receipt = r
	j.found = true
	return nil
}
func marketingDigest(v byte) [32]byte { return sha256.Sum256([]byte{v}) }
func marketingSnapshot(at time.Time) segment.HistoricalMarketingStateSnapshot {
	return segment.HistoricalMarketingStateSnapshot{SourceKeyDigest: marketingDigest(1), SourcePayloadDigest: marketingDigest(2), SourceFieldDigest: marketingDigest(3), ExternalUserIDDigest: marketingDigest(4), StatePayloadDigest: marketingDigest(5), EnteredAt: &at, ExitedAt: &at, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func marketingChange(at time.Time) segment.HistoricalMarketingStateChange {
	return segment.HistoricalMarketingStateChange{SourceKeyDigest: marketingDigest(6), SourcePayloadDigest: marketingDigest(7), SourceFieldDigest: marketingDigest(8), ExternalUserIDDigest: marketingDigest(9), StatePayloadDigest: marketingDigest(10), RecordedAt: at, CreatedAt: at.Add(-time.Second)}
}
func valueSnapshot(at time.Time) segment.HistoricalValueSegmentSnapshot {
	return segment.HistoricalValueSegmentSnapshot{SourceKeyDigest: marketingDigest(11), SourcePayloadDigest: marketingDigest(12), SourceFieldDigest: marketingDigest(13), ExternalUserIDDigest: marketingDigest(14), MatchedQuestionIDsDigest: marketingDigest(15), StatePayloadDigest: marketingDigest(16), EvaluatedAt: at, ComputedAt: at.Add(-time.Second), CreatedAt: at.Add(-2 * time.Second), UpdatedAt: at.Add(-3 * time.Second)}
}
func valueChange(at time.Time) segment.HistoricalValueSegmentChange {
	return segment.HistoricalValueSegmentChange{SourceKeyDigest: marketingDigest(17), SourcePayloadDigest: marketingDigest(18), SourceFieldDigest: marketingDigest(19), ExternalUserIDDigest: marketingDigest(20), MatchedQuestionIDsDigest: marketingDigest(21), StatePayloadDigest: marketingDigest(22), EvaluatedAt: at, RecordedAt: at.Add(-time.Second), CreatedAt: at.Add(-2 * time.Second)}
}
