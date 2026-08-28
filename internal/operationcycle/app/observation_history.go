package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"

	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
)

const (
	cycleMetricKind    = "cycle_metric"
	cycleReferenceKind = "cycle_reference"
)

// CycleObservationWriter persists sealed V1 observations with their receipt in
// the caller's transaction. It neither restores nor identifies a current run.
type CycleObservationWriter struct {
	store   cycle.CycleObservationStore
	journal cycle.CycleObservationJournal
}

func NewCycleObservationWriter(store cycle.CycleObservationStore, journal cycle.CycleObservationJournal) (*CycleObservationWriter, error) {
	if nilCycleObservation(store) || nilCycleObservation(journal) {
		return nil, cycle.ErrCycleObservationUnavailable
	}
	return &CycleObservationWriter{store: store, journal: journal}, nil
}

func (w *CycleObservationWriter) ImportHistoricalCycleMetric(ctx context.Context, source string, value cycle.HistoricalCycleMetric) (cycle.CycleObservationReceipt, error) {
	value = normalizeCycleMetric(value)
	return importCycleObservation(w, ctx, cycleMetricKind, source, value, HistoricalCycleMetricDigest,
		func(v cycle.HistoricalCycleMetric, id int64) cycle.HistoricalCycleMetric { v.ID = id; return v },
		func(v cycle.HistoricalCycleMetric) bool { return validCycleMetric(v, false) },
		func() (cycle.HistoricalCycleMetric, error) { return w.store.CreateHistoricalCycleMetric(ctx, value) },
		func(id int64) (cycle.HistoricalCycleMetric, error) { return w.store.GetHistoricalCycleMetric(ctx, id) })
}

func (w *CycleObservationWriter) ImportHistoricalCycleReference(ctx context.Context, source string, value cycle.HistoricalCycleReference) (cycle.CycleObservationReceipt, error) {
	value = normalizeCycleReference(value)
	return importCycleObservation(w, ctx, cycleReferenceKind, source, value, HistoricalCycleReferenceDigest,
		func(v cycle.HistoricalCycleReference, id int64) cycle.HistoricalCycleReference { v.ID = id; return v },
		func(v cycle.HistoricalCycleReference) bool { return validCycleReference(v, false) },
		func() (cycle.HistoricalCycleReference, error) {
			return w.store.CreateHistoricalCycleReference(ctx, value)
		},
		func(id int64) (cycle.HistoricalCycleReference, error) {
			return w.store.GetHistoricalCycleReference(ctx, id)
		})
}

func importCycleObservation[T any](w *CycleObservationWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, valid func(T) bool, create func() (T, error), get func(int64) (T, error)) (cycle.CycleObservationReceipt, error) {
	var empty cycle.CycleObservationReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilCycleObservation(w.store) || nilCycleObservation(w.journal) {
		return empty, cycle.ErrCycleObservationUnavailable
	}
	key, payload, id, ok := cycleObservationIdentity(value)
	if !ok || !valid(value) || id != 0 || source != hex.EncodeToString(key[:]) {
		return empty, cycle.ErrCycleObservationInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, cycle.ErrCycleObservationInvalid
	}
	receipt, found, err := w.journal.LoadCycleObservation(ctx, kind, source)
	if err != nil {
		return empty, cycleObservationError(err)
	}
	if found {
		if !validCycleObservationReceipt(receipt, kind, source, payload) {
			return empty, cycle.ErrCycleObservationConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, cycleObservationError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, cycle.ErrCycleObservationConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, cycleObservationError(err)
	}
	_, _, targetID, ok := cycleObservationIdentity(actual)
	if !ok || targetID < 1 {
		return empty, cycle.ErrCycleObservationConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, cycle.ErrCycleObservationConflict
	}
	receipt = cycle.CycleObservationReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordCycleObservation(ctx, receipt); err != nil {
		return empty, cycleObservationError(err)
	}
	return receipt, nil
}

// HistoricalCycleMetricDigest binds every persisted typed value. In particular
// the raw limitations bytes deliberately remain bytes: nil, empty, null and
// valid scalar/array/object values are not normalized into each other.
func HistoricalCycleMetricDigest(value cycle.HistoricalCycleMetric) ([32]byte, error) {
	value = normalizeCycleMetric(value)
	if !validCycleMetric(value, true) {
		return [32]byte{}, cycle.ErrCycleObservationInvalid
	}
	return cycleObservationDigest(cycleMetricKind, struct {
		ID, SourceID                                            int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest [32]byte
		RunSourceID                                             int64
		MetricKey, Label                                        string
		Numerator, Denominator, Value                           *float64
		Unit, ObservationWindow, DataSource, DataQuality        string
		Limitations                                             []byte
		IsCausal                                                bool
		ValueStatus                                             string
		LastSnapshotSourceID                                    int64
		CreatedAt, UpdatedAt                                    time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.RunSourceID, value.MetricKey, value.Label, value.Numerator, value.Denominator, value.Value, value.Unit, value.ObservationWindow, value.DataSource, value.DataQuality, []byte(value.LimitationsJSON), value.IsCausal, value.ValueStatus, value.LastSnapshotSourceID, value.CreatedAt, value.UpdatedAt})
}

// HistoricalCycleReferenceDigest includes Href explicitly because it is
// private evidence and intentionally omitted from the public JSON form.
func HistoricalCycleReferenceDigest(value cycle.HistoricalCycleReference) ([32]byte, error) {
	value = normalizeCycleReference(value)
	if !validCycleReference(value, true) {
		return [32]byte{}, cycle.ErrCycleObservationInvalid
	}
	return cycleObservationDigest(cycleReferenceKind, struct {
		ID, SourceID                                            int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest [32]byte
		RunSourceID                                             int64
		ReferenceKey, ReferenceType, Label, SourceSystem        string
		ReferenceSourceID, Href, EvidenceHash, DataStatus       string
		LastSnapshotSourceID                                    int64
		CreatedAt, UpdatedAt                                    time.Time
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.RunSourceID, value.ReferenceKey, value.ReferenceType, value.Label, value.SourceSystem, value.ReferenceSourceID, value.Href, value.EvidenceHash, value.DataStatus, value.LastSnapshotSourceID, value.CreatedAt, value.UpdatedAt})
}

func cycleObservationDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: kind, Value: value})
	if err != nil {
		return [32]byte{}, cycle.ErrCycleObservationInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validCycleMetric(value cycle.HistoricalCycleMetric, stored bool) bool {
	return validCycleObservationIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, stored) &&
		validCycleObservationNumber(value.Numerator) && validCycleObservationNumber(value.Denominator) && validCycleObservationNumber(value.Value) &&
		validCycleObservationTime(value.CreatedAt, stored) && validCycleObservationTime(value.UpdatedAt, stored)
}

func validCycleReference(value cycle.HistoricalCycleReference, stored bool) bool {
	return validCycleObservationIdentity(value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, stored) &&
		validCycleObservationTime(value.CreatedAt, stored) && validCycleObservationTime(value.UpdatedAt, stored)
}

func validCycleObservationIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && key != ([32]byte{}) && payload != ([32]byte{}) && field != ([32]byte{})
}

func validCycleObservationNumber(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0))
}

func validCycleObservationTime(value time.Time, stored bool) bool {
	return !value.IsZero() && (!stored || value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond)))
}

func normalizeCycleMetric(value cycle.HistoricalCycleMetric) cycle.HistoricalCycleMetric {
	value.Numerator = cloneCycleObservationFloat(value.Numerator)
	value.Denominator = cloneCycleObservationFloat(value.Denominator)
	value.Value = cloneCycleObservationFloat(value.Value)
	value.LimitationsJSON = cloneCycleObservationJSON(value.LimitationsJSON)
	value.CreatedAt = normalizeCycleObservationTime(value.CreatedAt)
	value.UpdatedAt = normalizeCycleObservationTime(value.UpdatedAt)
	return value
}

func normalizeCycleReference(value cycle.HistoricalCycleReference) cycle.HistoricalCycleReference {
	value.CreatedAt = normalizeCycleObservationTime(value.CreatedAt)
	value.UpdatedAt = normalizeCycleObservationTime(value.UpdatedAt)
	return value
}

func normalizeCycleObservationTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func cloneCycleObservationFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneCycleObservationJSON(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage{}, value...)
}

func cycleObservationIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch value := value.(type) {
	case cycle.HistoricalCycleMetric:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	case cycle.HistoricalCycleReference:
		return value.SourceKeyDigest, value.SourcePayloadDigest, value.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}

func validCycleObservationReceipt(value cycle.CycleObservationReceipt, kind, source string, payload [32]byte) bool {
	return value.Kind == kind && value.SourceIdentifier == source && value.PayloadDigest == payload && value.TargetID > 0 && value.TargetDigest != ([32]byte{})
}

func cycleObservationError(err error) error {
	if errors.Is(err, cycle.ErrCycleObservationInvalid) {
		return cycle.ErrCycleObservationInvalid
	}
	if errors.Is(err, cycle.ErrCycleObservationConflict) {
		return cycle.ErrCycleObservationConflict
	}
	return cycle.ErrCycleObservationUnavailable
}

func nilCycleObservation(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
