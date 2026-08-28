package app

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestHXCRuntimeHistoryDigestCoversPrivateFacts(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	sender := runtimeSenderFixture(1, at)
	sender.ID = 1
	first, err := HistoricalHXCSenderConfigDigest(sender)
	if err != nil {
		t.Fatal(err)
	}
	sender.PrivateDigest[0]++
	second, err := HistoricalHXCSenderConfigDigest(sender)
	if err != nil || first == second {
		t.Fatalf("sender private digest drift: %v", err)
	}
	record := runtimeSendFixture(2, at)
	record.ID = 1
	first, err = HistoricalHXCSendRecordDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	record.SourceFieldDigest[0]++
	second, err = HistoricalHXCSendRecordDigest(record)
	if err != nil || first == second {
		t.Fatalf("send field digest drift: %v", err)
	}
}

func TestHXCRuntimeHistoryWriterReplayAndPrivateDrift(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*60*60))
	store := &runtimeHistoryStoreFake{}
	journal := &runtimeHistoryJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}}
	writer, err := NewHXCRuntimeHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	sender := runtimeSenderFixture(1, at)
	record := runtimeSendFixture(2, at)
	senderReceipt, err := writer.ImportSenderConfig(context.Background(), hex.EncodeToString(sender.SourceKeyDigest[:]), sender)
	if err != nil || senderReceipt.Kind != hxc.HXCHistorySenderConfig || senderReceipt.Replayed {
		t.Fatalf("sender import: %#v %v", senderReceipt, err)
	}
	if stored := store.senders[senderReceipt.TargetID]; stored.CreatedAt.Location() != time.UTC || stored.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("sender time not normalized: %#v", stored.CreatedAt)
	}
	recordReceipt, err := writer.ImportSendRecord(context.Background(), hex.EncodeToString(record.SourceKeyDigest[:]), record)
	if err != nil || recordReceipt.Kind != hxc.HXCHistorySendRecord || recordReceipt.Replayed {
		t.Fatalf("record import: %#v %v", recordReceipt, err)
	}
	replay, err := writer.ImportSendRecord(context.Background(), hex.EncodeToString(record.SourceKeyDigest[:]), record)
	if err != nil || !replay.Replayed || replay.TargetDigest != recordReceipt.TargetDigest {
		t.Fatalf("record replay: %#v %v", replay, err)
	}
	record.PrivateDigest[0]++
	if _, err := writer.ImportSendRecord(context.Background(), hex.EncodeToString(record.SourceKeyDigest[:]), record); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
		t.Fatalf("private drift = %v", err)
	}
}

func TestHXCRuntimeHistoryWriterRejectsInvalidInput(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	writer, err := NewHXCRuntimeHistoryWriter(&runtimeHistoryStoreFake{}, &runtimeHistoryJournalFake{entries: map[string]hxc.HXCHistoryReceipt{}})
	if err != nil {
		t.Fatal(err)
	}
	value := runtimeSenderFixture(3, at)
	value.SourceFieldDigest = [32]byte{}
	if _, err := writer.ImportSenderConfig(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatalf("zero source field digest = %v", err)
	}
	if _, err := writer.ImportSenderConfig(context.Background(), "not-source-key", runtimeSenderFixture(4, at)); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatalf("wrong source identifier = %v", err)
	}
}

type runtimeHistoryStoreFake struct {
	next    int64
	senders map[int64]hxc.HistoricalHXCSenderConfig
	records map[int64]hxc.HistoricalHXCSendRecord
}

func (s *runtimeHistoryStoreFake) CreateHistoricalHXCSenderConfig(_ context.Context, value hxc.HistoricalHXCSenderConfig) (hxc.HistoricalHXCSenderConfig, error) {
	if s.senders == nil {
		s.senders = map[int64]hxc.HistoricalHXCSenderConfig{}
	}
	s.next++
	value.ID = s.next
	s.senders[value.ID] = value
	return value, nil
}
func (s *runtimeHistoryStoreFake) GetHistoricalHXCSenderConfig(_ context.Context, id int64) (hxc.HistoricalHXCSenderConfig, error) {
	value, ok := s.senders[id]
	if !ok {
		return hxc.HistoricalHXCSenderConfig{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}
func (s *runtimeHistoryStoreFake) CreateHistoricalHXCSendRecord(_ context.Context, value hxc.HistoricalHXCSendRecord) (hxc.HistoricalHXCSendRecord, error) {
	if s.records == nil {
		s.records = map[int64]hxc.HistoricalHXCSendRecord{}
	}
	s.next++
	value.ID = s.next
	s.records[value.ID] = value
	return value, nil
}
func (s *runtimeHistoryStoreFake) GetHistoricalHXCSendRecord(_ context.Context, id int64) (hxc.HistoricalHXCSendRecord, error) {
	value, ok := s.records[id]
	if !ok {
		return hxc.HistoricalHXCSendRecord{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

type runtimeHistoryJournalFake struct {
	entries map[string]hxc.HXCHistoryReceipt
}

func (j *runtimeHistoryJournalFake) LoadHXCHistory(_ context.Context, kind, source string) (hxc.HXCHistoryReceipt, bool, error) {
	value, ok := j.entries[kind+":"+source]
	return value, ok, nil
}
func (j *runtimeHistoryJournalFake) RecordHXCHistory(_ context.Context, value hxc.HXCHistoryReceipt) error {
	j.entries[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

func runtimeIdentityFixture(first byte) hxc.HistoricalHXCRuntimeIdentity {
	var key, payload, field, private [32]byte
	key[0], payload[0], field[0], private[0] = first, first+20, first+40, first+60
	return hxc.HistoricalHXCRuntimeIdentity{SourceID: int64(first) - 5, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private}
}
func runtimeSenderFixture(first byte, at time.Time) hxc.HistoricalHXCSenderConfig {
	return hxc.HistoricalHXCSenderConfig{HistoricalHXCRuntimeIdentity: runtimeIdentityFixture(first), Priority: -4, OriginalIsActive: false, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func runtimeSendFixture(first byte, at time.Time) hxc.HistoricalHXCSendRecord {
	target := int64(-9)
	last := at.Add(time.Second)
	return hxc.HistoricalHXCSendRecord{HistoricalHXCRuntimeIdentity: runtimeIdentityFixture(first), TaskType: "", OriginalStatus: "", SelectedCount: -1, EligibleCount: 0, SentCount: 1, SkippedCount: -2, PlannedCount: 3, QueuedCount: -4, DispatchingCount: 5, SucceededCount: -6, FailedCount: 7, BlockedCount: -8, CancelledCount: 9, ImageCount: -10, IncludeDoNotDisturb: false, TargetSource: "", TargetSourceID: &target, CreatedAt: at, LastStatusSyncAt: &last, LastRefreshedAt: nil}
}
