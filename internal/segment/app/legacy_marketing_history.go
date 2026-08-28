package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	legacyMarketingStateKind = "legacy_marketing_state"
	legacyMarketingValueKind = "legacy_marketing_value"
)

type LegacyMarketingHistoryWriter struct {
	store   segment.LegacyMarketingHistoryStore
	journal segment.LegacyMarketingHistoryJournal
}

func NewLegacyMarketingHistoryWriter(store segment.LegacyMarketingHistoryStore, journal segment.LegacyMarketingHistoryJournal) (*LegacyMarketingHistoryWriter, error) {
	if nilLegacyMarketing(store) || nilLegacyMarketing(journal) {
		return nil, segment.ErrLegacyMarketingHistoryUnavailable
	}
	return &LegacyMarketingHistoryWriter{store: store, journal: journal}, nil
}

func (w *LegacyMarketingHistoryWriter) ImportLegacyMarketingState(ctx context.Context, source string, value segment.HistoricalLegacyMarketingState) (segment.LegacyMarketingHistoryReceipt, error) {
	value = normalizeLegacyMarketingState(value)
	return importLegacyMarketing(w, ctx, legacyMarketingStateKind, source, value, HistoricalLegacyMarketingStateDigest, withLegacyMarketingStateID, func() (segment.HistoricalLegacyMarketingState, error) {
		return w.store.CreateHistoricalLegacyMarketingState(ctx, value)
	}, func(id int64) (segment.HistoricalLegacyMarketingState, error) {
		return w.store.GetHistoricalLegacyMarketingState(ctx, id)
	})
}

func (w *LegacyMarketingHistoryWriter) ImportLegacyMarketingValue(ctx context.Context, source string, value segment.HistoricalLegacyMarketingValue) (segment.LegacyMarketingHistoryReceipt, error) {
	value = normalizeLegacyMarketingValue(value)
	return importLegacyMarketing(w, ctx, legacyMarketingValueKind, source, value, HistoricalLegacyMarketingValueDigest, withLegacyMarketingValueID, func() (segment.HistoricalLegacyMarketingValue, error) {
		return w.store.CreateHistoricalLegacyMarketingValue(ctx, value)
	}, func(id int64) (segment.HistoricalLegacyMarketingValue, error) {
		return w.store.GetHistoricalLegacyMarketingValue(ctx, id)
	})
}

func importLegacyMarketing[T any](w *LegacyMarketingHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([sha256.Size]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (segment.LegacyMarketingHistoryReceipt, error) {
	var empty segment.LegacyMarketingHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilLegacyMarketing(w.store) || nilLegacyMarketing(w.journal) {
		return empty, segment.ErrLegacyMarketingHistoryUnavailable
	}
	key, payload, id, ok := legacyMarketingIdentity(value)
	if !ok || id != 0 || key == ([sha256.Size]byte{}) || payload == ([sha256.Size]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, segment.ErrLegacyMarketingHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, segment.ErrLegacyMarketingHistoryInvalid
	}
	receipt, found, err := w.journal.LoadLegacyMarketingHistory(ctx, kind, source)
	if err != nil {
		return empty, legacyMarketingError(err)
	}
	if found {
		if !validLegacyMarketingReceipt(receipt, kind, source, payload) {
			return empty, segment.ErrLegacyMarketingHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, legacyMarketingError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, segment.ErrLegacyMarketingHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, legacyMarketingError(err)
	}
	_, _, targetID, ok := legacyMarketingIdentity(actual)
	if !ok || targetID < 1 {
		return empty, segment.ErrLegacyMarketingHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, segment.ErrLegacyMarketingHistoryConflict
	}
	receipt = segment.LegacyMarketingHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordLegacyMarketingHistory(ctx, receipt); err != nil {
		return empty, legacyMarketingError(err)
	}
	return receipt, nil
}

// Digest every private provenance and identity field explicitly; Port JSON omits them.
func HistoricalLegacyMarketingStateDigest(v segment.HistoricalLegacyMarketingState) ([sha256.Size]byte, error) {
	if !validLegacyMarketingState(v, true) {
		return [sha256.Size]byte{}, segment.ErrLegacyMarketingHistoryInvalid
	}
	return legacyMarketingDigest(legacyMarketingStateKind, struct {
		ID                                           int64
		Key, Payload, Field, External, State         [sha256.Size]byte
		SourceID                                     int64
		Scenario, Phase, Label, Reason, Lifecycle    string
		Batch                                        *int64
		BatchStatus, WindowStart, WindowEnd, Trigger string
		Entered, Exited                              *time.Time
		Exit                                         string
		Created, Updated                             time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest, v.SourceID, v.ScenarioKey, v.MarketingPhase, v.PhaseLabel, v.PhaseReason, v.LifecycleStatus, v.LastBatchSourceID, v.LastBatchStatus, v.LastBatchWindowStart, v.LastBatchWindowEnd, v.LastTriggerMessageAt, v.EnteredAt, v.ExitedAt, v.ExitReason, v.CreatedAt, v.UpdatedAt})
}

func HistoricalLegacyMarketingValueDigest(v segment.HistoricalLegacyMarketingValue) ([sha256.Size]byte, error) {
	if !validLegacyMarketingValue(v, true) {
		return [sha256.Size]byte{}, segment.ErrLegacyMarketingHistoryInvalid
	}
	return legacyMarketingDigest(legacyMarketingValueKind, struct {
		ID                                              int64
		Key, Payload, Field, External, Breakdown, State [sha256.Size]byte
		SourceID                                        int64
		Scenario, Segment, Label                        string
		Score                                           int64
		Created, Updated                                time.Time
	}{v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.ScoreBreakdownDigest, v.StatePayloadDigest, v.SourceID, v.ScenarioKey, v.ValueSegment, v.SegmentLabel, v.Score, v.CreatedAt, v.UpdatedAt})
}

func legacyMarketingDigest(kind string, value any) ([sha256.Size]byte, error) {
	b, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [sha256.Size]byte{}, segment.ErrLegacyMarketingHistoryInvalid
	}
	return sha256.Sum256(b), nil
}

func validLegacyMarketingState(v segment.HistoricalLegacyMarketingState, stored bool) bool {
	return validLegacyMarketingIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && nonzero(v.ExternalUserIDDigest) && nonzero(v.StatePayloadDigest) && validLegacyMarketingTime(v.CreatedAt, stored) && validLegacyMarketingTime(v.UpdatedAt, stored) && validLegacyMarketingOptionalTime(v.EnteredAt, stored) && validLegacyMarketingOptionalTime(v.ExitedAt, stored)
}
func validLegacyMarketingValue(v segment.HistoricalLegacyMarketingValue, stored bool) bool {
	return validLegacyMarketingIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && nonzero(v.ExternalUserIDDigest) && nonzero(v.ScoreBreakdownDigest) && nonzero(v.StatePayloadDigest) && validLegacyMarketingTime(v.CreatedAt, stored) && validLegacyMarketingTime(v.UpdatedAt, stored)
}
func validLegacyMarketingIdentity(id int64, key, payload, field [sha256.Size]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && nonzero(key) && nonzero(payload) && nonzero(field)
}
func validLegacyMarketingTime(v time.Time, stored bool) bool {
	return !v.IsZero() && (!stored || v.Location() == time.UTC && v.Equal(v.UTC().Truncate(time.Microsecond)))
}
func validLegacyMarketingOptionalTime(v *time.Time, stored bool) bool {
	return v == nil || validLegacyMarketingTime(*v, stored)
}
func nonzero(v [sha256.Size]byte) bool                   { return v != ([sha256.Size]byte{}) }
func normalizeLegacyMarketingTime(v time.Time) time.Time { return v.UTC().Truncate(time.Microsecond) }
func normalizeLegacyMarketingOptionalTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	x := normalizeLegacyMarketingTime(*v)
	return &x
}
func normalizeLegacyMarketingState(v segment.HistoricalLegacyMarketingState) segment.HistoricalLegacyMarketingState {
	v.EnteredAt, v.ExitedAt = normalizeLegacyMarketingOptionalTime(v.EnteredAt), normalizeLegacyMarketingOptionalTime(v.ExitedAt)
	v.CreatedAt, v.UpdatedAt = normalizeLegacyMarketingTime(v.CreatedAt), normalizeLegacyMarketingTime(v.UpdatedAt)
	return v
}
func normalizeLegacyMarketingValue(v segment.HistoricalLegacyMarketingValue) segment.HistoricalLegacyMarketingValue {
	v.CreatedAt, v.UpdatedAt = normalizeLegacyMarketingTime(v.CreatedAt), normalizeLegacyMarketingTime(v.UpdatedAt)
	return v
}
func withLegacyMarketingStateID(v segment.HistoricalLegacyMarketingState, id int64) segment.HistoricalLegacyMarketingState {
	v.ID = id
	return v
}
func withLegacyMarketingValueID(v segment.HistoricalLegacyMarketingValue, id int64) segment.HistoricalLegacyMarketingValue {
	v.ID = id
	return v
}
func legacyMarketingIdentity(value any) ([sha256.Size]byte, [sha256.Size]byte, int64, bool) {
	switch v := value.(type) {
	case segment.HistoricalLegacyMarketingState:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case segment.HistoricalLegacyMarketingValue:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [sha256.Size]byte{}, [sha256.Size]byte{}, 0, false
}
func validLegacyMarketingReceipt(v segment.LegacyMarketingHistoryReceipt, kind, source string, payload [sha256.Size]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && nonzero(v.TargetDigest)
}
func legacyMarketingError(err error) error {
	if errors.Is(err, segment.ErrLegacyMarketingHistoryInvalid) {
		return segment.ErrLegacyMarketingHistoryInvalid
	}
	if errors.Is(err, segment.ErrLegacyMarketingHistoryConflict) {
		return segment.ErrLegacyMarketingHistoryConflict
	}
	return segment.ErrLegacyMarketingHistoryUnavailable
}
func nilLegacyMarketing(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	switch x.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return x.IsNil()
	}
	return false
}
