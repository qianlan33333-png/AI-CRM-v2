package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
)

const (
	staticCycleStrategyKind = "cycle_strategy"
	staticCycleVersionKind  = "cycle_version"
	staticCycleDocumentKind = "cycle_document"
)

// StaticCycleHistoryWriter writes immutable V1 static facts and their receipt
// in the caller's transaction.
type StaticCycleHistoryWriter struct {
	store   cycle.StaticCycleHistoryStore
	journal cycle.StaticCycleHistoryJournal
}

func NewStaticCycleHistoryWriter(store cycle.StaticCycleHistoryStore, journal cycle.StaticCycleHistoryJournal) (*StaticCycleHistoryWriter, error) {
	if nilStaticCycle(store) || nilStaticCycle(journal) {
		return nil, cycle.ErrStaticCycleHistoryUnavailable
	}
	return &StaticCycleHistoryWriter{store: store, journal: journal}, nil
}

func (w *StaticCycleHistoryWriter) ImportCycleStrategy(ctx context.Context, sourceHex string, value cycle.HistoricalCycleStrategy) (cycle.StaticCycleHistoryReceipt, error) {
	value = normalizeCycleStrategy(value)
	return importStaticCycle(w, ctx, staticCycleStrategyKind, sourceHex, value, HistoricalCycleStrategyDigest,
		func(v cycle.HistoricalCycleStrategy, id int64) cycle.HistoricalCycleStrategy { v.ID = id; return v },
		func() (cycle.HistoricalCycleStrategy, error) {
			return w.store.CreateHistoricalCycleStrategy(ctx, value)
		},
		func(id int64) (cycle.HistoricalCycleStrategy, error) {
			return w.store.GetHistoricalCycleStrategy(ctx, id)
		})
}

func (w *StaticCycleHistoryWriter) ImportCycleVersion(ctx context.Context, sourceHex string, value cycle.HistoricalCycleVersion) (cycle.StaticCycleHistoryReceipt, error) {
	value = normalizeCycleVersion(value)
	return importStaticCycle(w, ctx, staticCycleVersionKind, sourceHex, value, HistoricalCycleVersionDigest,
		func(v cycle.HistoricalCycleVersion, id int64) cycle.HistoricalCycleVersion { v.ID = id; return v },
		func() (cycle.HistoricalCycleVersion, error) { return w.store.CreateHistoricalCycleVersion(ctx, value) },
		func(id int64) (cycle.HistoricalCycleVersion, error) {
			return w.store.GetHistoricalCycleVersion(ctx, id)
		})
}

func (w *StaticCycleHistoryWriter) ImportCycleDocument(ctx context.Context, sourceHex string, value cycle.HistoricalCycleDocument) (cycle.StaticCycleHistoryReceipt, error) {
	value = normalizeCycleDocument(value)
	return importStaticCycle(w, ctx, staticCycleDocumentKind, sourceHex, value, HistoricalCycleDocumentDigest,
		func(v cycle.HistoricalCycleDocument, id int64) cycle.HistoricalCycleDocument { v.ID = id; return v },
		func() (cycle.HistoricalCycleDocument, error) {
			return w.store.CreateHistoricalCycleDocument(ctx, value)
		},
		func(id int64) (cycle.HistoricalCycleDocument, error) {
			return w.store.GetHistoricalCycleDocument(ctx, id)
		})
}

func importStaticCycle[T any](w *StaticCycleHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (cycle.StaticCycleHistoryReceipt, error) {
	var empty cycle.StaticCycleHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilStaticCycle(w.store) || nilStaticCycle(w.journal) {
		return empty, cycle.ErrStaticCycleHistoryUnavailable
	}
	key, payload, id, ok := staticCycleIdentity(value)
	if !ok || kind == "" || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, cycle.ErrStaticCycleHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, cycle.ErrStaticCycleHistoryInvalid
	}
	receipt, found, err := w.journal.LoadStaticCycleHistory(ctx, kind, source)
	if err != nil {
		return empty, staticCycleError(err)
	}
	if found {
		if !validStaticCycleReceipt(receipt, kind, source, payload) {
			return empty, cycle.ErrStaticCycleHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, staticCycleError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, cycle.ErrStaticCycleHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, staticCycleError(err)
	}
	_, _, targetID, ok := staticCycleIdentity(actual)
	if !ok || targetID < 1 {
		return empty, cycle.ErrStaticCycleHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, cycle.ErrStaticCycleHistoryConflict
	}
	receipt = cycle.StaticCycleHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordStaticCycleHistory(ctx, receipt); err != nil {
		return empty, staticCycleError(err)
	}
	return receipt, nil
}

func HistoricalCycleStrategyDigest(value cycle.HistoricalCycleStrategy) ([32]byte, error) {
	if !validCycleStrategy(value, true) {
		return [32]byte{}, cycle.ErrStaticCycleHistoryInvalid
	}
	return staticCycleDigest(staticCycleStrategyKind, value)
}
func HistoricalCycleVersionDigest(value cycle.HistoricalCycleVersion) ([32]byte, error) {
	if !validCycleVersion(value, true) {
		return [32]byte{}, cycle.ErrStaticCycleHistoryInvalid
	}
	return staticCycleDigest(staticCycleVersionKind, value)
}
func HistoricalCycleDocumentDigest(value cycle.HistoricalCycleDocument) ([32]byte, error) {
	if !validCycleDocument(value, true) {
		return [32]byte{}, cycle.ErrStaticCycleHistoryInvalid
	}
	return staticCycleDigest(staticCycleDocumentKind, value)
}
func staticCycleDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: kind, Value: value})
	if err != nil {
		return [32]byte{}, cycle.ErrStaticCycleHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validCycleIdentity(id int64, key, payload [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && key != ([32]byte{}) && payload != ([32]byte{})
}
func validCycleTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}
func validCycleOptionalTime(value *time.Time, stored bool) bool {
	return value == nil || validCycleTime(*value, stored)
}
func validCycleStrategy(value cycle.HistoricalCycleStrategy, stored bool) bool {
	return validCycleIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, stored) && validCycleTime(value.CreatedAt, stored) && validCycleTime(value.UpdatedAt, stored)
}
func validCycleVersion(value cycle.HistoricalCycleVersion, stored bool) bool {
	return validCycleIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, stored) && value.StrategyHistoryID > 0 && validCycleOptionalTime(value.EffectiveFrom, stored) && validCycleOptionalTime(value.ConfirmedAt, stored) && validCycleTime(value.CreatedAt, stored)
}
func validCycleDocument(value cycle.HistoricalCycleDocument, stored bool) bool {
	return validCycleIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, stored) && value.VersionHistoryID > 0 && validCycleOptionalTime(value.ExecutionGuideGeneratedAt, stored) && validCycleOptionalTime(value.CopyGuideGeneratedAt, stored) && validCycleOptionalTime(value.MeasurementGuideGeneratedAt, stored) && validCycleTime(value.CreatedAt, stored)
}

func normalizeCycleTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
func normalizeCycleOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := normalizeCycleTime(*value)
	return &copy
}
func normalizeCycleStrategy(value cycle.HistoricalCycleStrategy) cycle.HistoricalCycleStrategy {
	value.CreatedAt, value.UpdatedAt = normalizeCycleTime(value.CreatedAt), normalizeCycleTime(value.UpdatedAt)
	return value
}
func normalizeCycleVersion(value cycle.HistoricalCycleVersion) cycle.HistoricalCycleVersion {
	value.EffectiveFrom, value.ConfirmedAt, value.CreatedAt = normalizeCycleOptionalTime(value.EffectiveFrom), normalizeCycleOptionalTime(value.ConfirmedAt), normalizeCycleTime(value.CreatedAt)
	return value
}
func normalizeCycleDocument(value cycle.HistoricalCycleDocument) cycle.HistoricalCycleDocument {
	value.ExecutionGuideGeneratedAt, value.CopyGuideGeneratedAt, value.MeasurementGuideGeneratedAt, value.CreatedAt = normalizeCycleOptionalTime(value.ExecutionGuideGeneratedAt), normalizeCycleOptionalTime(value.CopyGuideGeneratedAt), normalizeCycleOptionalTime(value.MeasurementGuideGeneratedAt), normalizeCycleTime(value.CreatedAt)
	return value
}

func staticCycleIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch value := value.(type) {
	case cycle.HistoricalCycleStrategy:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	case cycle.HistoricalCycleVersion:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	case cycle.HistoricalCycleDocument:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	default:
		return [32]byte{}, [32]byte{}, 0, false
	}
}
func validStaticCycleReceipt(value cycle.StaticCycleHistoryReceipt, kind, source string, payload [32]byte) bool {
	return value.Kind == kind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}
func staticCycleError(err error) error {
	if errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
		return cycle.ErrStaticCycleHistoryInvalid
	}
	if errors.Is(err, cycle.ErrStaticCycleHistoryConflict) {
		return cycle.ErrStaticCycleHistoryConflict
	}
	return cycle.ErrStaticCycleHistoryUnavailable
}
func nilStaticCycle(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
