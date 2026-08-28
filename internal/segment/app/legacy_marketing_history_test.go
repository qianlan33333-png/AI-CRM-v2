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

func TestLegacyMarketingHistoryWriterReplayAndPrivateDigestBinding(t *testing.T) {
	store := &legacyMarketingHistoryStoreFake{}
	journal := &legacyMarketingHistoryJournalFake{}
	writer, err := NewLegacyMarketingHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}

	state := legacyMarketingStateFixture()
	source := hex.EncodeToString(state.SourceKeyDigest[:])
	first, err := writer.ImportLegacyMarketingState(context.Background(), source, state)
	if err != nil || first.Replayed || first.Kind != legacyMarketingStateKind || store.stateCreates != 1 {
		t.Fatalf("first state receipt = %#v, %v", first, err)
	}
	replayed, err := writer.ImportLegacyMarketingState(context.Background(), source, state)
	if err != nil || !replayed.Replayed || replayed.TargetID != first.TargetID || store.stateCreates != 1 {
		t.Fatalf("replayed state receipt = %#v, %v", replayed, err)
	}
	storedState := store.states[first.TargetID]
	storedState.ExternalUserIDDigest[0]++
	store.states[first.TargetID] = storedState
	if _, err := writer.ImportLegacyMarketingState(context.Background(), source, state); !errors.Is(err, segment.ErrLegacyMarketingHistoryConflict) {
		t.Fatalf("private identity drift error = %v", err)
	}

	value := legacyMarketingValueFixture()
	source = hex.EncodeToString(value.SourceKeyDigest[:])
	first, err = writer.ImportLegacyMarketingValue(context.Background(), source, value)
	if err != nil || first.Replayed || first.Kind != legacyMarketingValueKind || store.valueCreates != 1 {
		t.Fatalf("first value receipt = %#v, %v", first, err)
	}
	storedValue := store.values[first.TargetID]
	storedValue.ScoreBreakdownDigest[0]++
	store.values[first.TargetID] = storedValue
	if _, err := writer.ImportLegacyMarketingValue(context.Background(), source, value); !errors.Is(err, segment.ErrLegacyMarketingHistoryConflict) {
		t.Fatalf("private value drift error = %v", err)
	}
}

func TestLegacyMarketingHistoryDigestPreservesPrivateFieldsAndUTC(t *testing.T) {
	state := legacyMarketingStateFixture()
	state.CreatedAt = state.CreatedAt.In(time.FixedZone("legacy", 8*60*60)).Add(789 * time.Nanosecond)
	normalized := normalizeLegacyMarketingState(state)
	if normalized.CreatedAt.Location() != time.UTC || normalized.CreatedAt.Nanosecond()%int(time.Microsecond) != 0 {
		t.Fatal("state time was not normalized to UTC microseconds")
	}
	digest, err := HistoricalLegacyMarketingStateDigest(withLegacyMarketingStateID(normalized, 1))
	if err != nil {
		t.Fatal(err)
	}
	mutated := withLegacyMarketingStateID(normalized, 1)
	mutated.SourceFieldDigest[0]++
	if changed, err := HistoricalLegacyMarketingStateDigest(mutated); err != nil || changed == digest {
		t.Fatal("source field digest was not bound")
	}
	mutated = withLegacyMarketingStateID(normalized, 1)
	mutated.StatePayloadDigest[0]++
	if changed, err := HistoricalLegacyMarketingStateDigest(mutated); err != nil || changed == digest {
		t.Fatal("state payload digest was not bound")
	}

	value := legacyMarketingValueFixture()
	digest, err = HistoricalLegacyMarketingValueDigest(withLegacyMarketingValueID(value, 1))
	if err != nil {
		t.Fatal(err)
	}
	changedValue := withLegacyMarketingValueID(value, 1)
	changedValue.ScoreBreakdownDigest[0]++
	if changed, err := HistoricalLegacyMarketingValueDigest(changedValue); err != nil || changed == digest {
		t.Fatal("score breakdown digest was not bound")
	}
}

func TestLegacyMarketingHistoryWriterRejectsInvalidAndTypedNil(t *testing.T) {
	var nilStore *legacyMarketingHistoryStoreFake
	if _, err := NewLegacyMarketingHistoryWriter(nilStore, &legacyMarketingHistoryJournalFake{}); !errors.Is(err, segment.ErrLegacyMarketingHistoryUnavailable) {
		t.Fatalf("typed nil store error = %v", err)
	}
	writer, err := NewLegacyMarketingHistoryWriter(&legacyMarketingHistoryStoreFake{}, &legacyMarketingHistoryJournalFake{})
	if err != nil {
		t.Fatal(err)
	}
	state := legacyMarketingStateFixture()
	if _, err := writer.ImportLegacyMarketingState(context.Background(), "wrong", state); !errors.Is(err, segment.ErrLegacyMarketingHistoryInvalid) {
		t.Fatalf("source binding error = %v", err)
	}
	state.SourcePayloadDigest = [sha256.Size]byte{}
	if _, err := writer.ImportLegacyMarketingState(context.Background(), hex.EncodeToString(state.SourceKeyDigest[:]), state); !errors.Is(err, segment.ErrLegacyMarketingHistoryInvalid) {
		t.Fatalf("empty payload error = %v", err)
	}
}

type legacyMarketingHistoryStoreFake struct {
	states       map[int64]segment.HistoricalLegacyMarketingState
	values       map[int64]segment.HistoricalLegacyMarketingValue
	stateCreates int
	valueCreates int
}

func (f *legacyMarketingHistoryStoreFake) CreateHistoricalLegacyMarketingState(_ context.Context, value segment.HistoricalLegacyMarketingState) (segment.HistoricalLegacyMarketingState, error) {
	if f.states == nil {
		f.states = map[int64]segment.HistoricalLegacyMarketingState{}
	}
	f.stateCreates++
	value.ID = int64(f.stateCreates)
	f.states[value.ID] = value
	return value, nil
}
func (f *legacyMarketingHistoryStoreFake) GetHistoricalLegacyMarketingState(_ context.Context, id int64) (segment.HistoricalLegacyMarketingState, error) {
	value, ok := f.states[id]
	if !ok {
		return segment.HistoricalLegacyMarketingState{}, segment.ErrLegacyMarketingHistoryUnavailable
	}
	return value, nil
}
func (f *legacyMarketingHistoryStoreFake) CreateHistoricalLegacyMarketingValue(_ context.Context, value segment.HistoricalLegacyMarketingValue) (segment.HistoricalLegacyMarketingValue, error) {
	if f.values == nil {
		f.values = map[int64]segment.HistoricalLegacyMarketingValue{}
	}
	f.valueCreates++
	value.ID = int64(f.valueCreates)
	f.values[value.ID] = value
	return value, nil
}
func (f *legacyMarketingHistoryStoreFake) GetHistoricalLegacyMarketingValue(_ context.Context, id int64) (segment.HistoricalLegacyMarketingValue, error) {
	value, ok := f.values[id]
	if !ok {
		return segment.HistoricalLegacyMarketingValue{}, segment.ErrLegacyMarketingHistoryUnavailable
	}
	return value, nil
}

type legacyMarketingHistoryJournalFake struct {
	receipts map[string]segment.LegacyMarketingHistoryReceipt
}

func (f *legacyMarketingHistoryJournalFake) LoadLegacyMarketingHistory(_ context.Context, kind, source string) (segment.LegacyMarketingHistoryReceipt, bool, error) {
	value, ok := f.receipts[kind+"/"+source]
	return value, ok, nil
}
func (f *legacyMarketingHistoryJournalFake) RecordLegacyMarketingHistory(_ context.Context, value segment.LegacyMarketingHistoryReceipt) error {
	if f.receipts == nil {
		f.receipts = map[string]segment.LegacyMarketingHistoryReceipt{}
	}
	f.receipts[value.Kind+"/"+value.SourceIdentifier] = value
	return nil
}

func legacyMarketingStateFixture() segment.HistoricalLegacyMarketingState {
	at := time.Date(2026, 8, 28, 12, 13, 14, 123456000, time.UTC)
	batch := int64(-7)
	return segment.HistoricalLegacyMarketingState{
		SourceKeyDigest: legacyMarketingTestDigest(1), SourcePayloadDigest: legacyMarketingTestDigest(2), SourceFieldDigest: legacyMarketingTestDigest(3),
		SourceID: -1, ExternalUserIDDigest: legacyMarketingTestDigest(4), ScenarioKey: "", MarketingPhase: "", PhaseLabel: "", PhaseReason: "", LifecycleStatus: "",
		LastBatchSourceID: &batch, LastBatchStatus: "", LastBatchWindowStart: "", LastBatchWindowEnd: "", LastTriggerMessageAt: "", EnteredAt: &at, ExitReason: "", StatePayloadDigest: legacyMarketingTestDigest(5), CreatedAt: at, UpdatedAt: at,
	}
}
func legacyMarketingValueFixture() segment.HistoricalLegacyMarketingValue {
	at := time.Date(2026, 8, 28, 12, 13, 14, 123456000, time.UTC)
	return segment.HistoricalLegacyMarketingValue{SourceKeyDigest: legacyMarketingTestDigest(11), SourcePayloadDigest: legacyMarketingTestDigest(12), SourceFieldDigest: legacyMarketingTestDigest(13), SourceID: 0, ExternalUserIDDigest: legacyMarketingTestDigest(14), ScenarioKey: "", ValueSegment: "", SegmentLabel: "", Score: -9, ScoreBreakdownDigest: legacyMarketingTestDigest(15), StatePayloadDigest: legacyMarketingTestDigest(16), CreatedAt: at, UpdatedAt: at}
}
func legacyMarketingTestDigest(first byte) [sha256.Size]byte {
	var value [sha256.Size]byte
	value[0] = first
	return value
}

var _ segment.LegacyMarketingHistoryStore = (*legacyMarketingHistoryStoreFake)(nil)
var _ segment.LegacyMarketingHistoryJournal = (*legacyMarketingHistoryJournalFake)(nil)
