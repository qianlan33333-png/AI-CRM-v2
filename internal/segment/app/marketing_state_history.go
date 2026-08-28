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
	marketingStateSnapshotKind = "marketing_state_snapshot"
	marketingStateChangeKind   = "marketing_state_change"
	valueSegmentSnapshotKind   = "value_segment_snapshot"
	valueSegmentChangeKind     = "value_segment_change"
)

// MarketingStateHistoryWriter only composes the caller's Store and Journal.
// Both must already participate in the caller transaction.
type MarketingStateHistoryWriter struct {
	store   segment.MarketingStateHistoryStore
	journal segment.MarketingStateHistoryJournal
}

func NewMarketingStateHistoryWriter(store segment.MarketingStateHistoryStore, journal segment.MarketingStateHistoryJournal) (*MarketingStateHistoryWriter, error) {
	if nilMarketingState(store) || nilMarketingState(journal) {
		return nil, segment.ErrMarketingStateHistoryUnavailable
	}
	return &MarketingStateHistoryWriter{store: store, journal: journal}, nil
}

func (w *MarketingStateHistoryWriter) ImportMarketingStateSnapshot(ctx context.Context, source string, value segment.HistoricalMarketingStateSnapshot) (segment.MarketingStateHistoryReceipt, error) {
	value = normalizeMarketingStateSnapshot(value)
	return importMarketingState(w, ctx, marketingStateSnapshotKind, source, value, HistoricalMarketingStateSnapshotDigest, withMarketingStateSnapshotID, func() (segment.HistoricalMarketingStateSnapshot, error) {
		return w.store.CreateHistoricalMarketingStateSnapshot(ctx, value)
	}, func(id int64) (segment.HistoricalMarketingStateSnapshot, error) {
		return w.store.GetHistoricalMarketingStateSnapshot(ctx, id)
	})
}

func (w *MarketingStateHistoryWriter) ImportMarketingStateChange(ctx context.Context, source string, value segment.HistoricalMarketingStateChange) (segment.MarketingStateHistoryReceipt, error) {
	value = normalizeMarketingStateChange(value)
	return importMarketingState(w, ctx, marketingStateChangeKind, source, value, HistoricalMarketingStateChangeDigest, withMarketingStateChangeID, func() (segment.HistoricalMarketingStateChange, error) {
		return w.store.CreateHistoricalMarketingStateChange(ctx, value)
	}, func(id int64) (segment.HistoricalMarketingStateChange, error) {
		return w.store.GetHistoricalMarketingStateChange(ctx, id)
	})
}

func (w *MarketingStateHistoryWriter) ImportValueSegmentSnapshot(ctx context.Context, source string, value segment.HistoricalValueSegmentSnapshot) (segment.MarketingStateHistoryReceipt, error) {
	value = normalizeValueSegmentSnapshot(value)
	return importMarketingState(w, ctx, valueSegmentSnapshotKind, source, value, HistoricalValueSegmentSnapshotDigest, withValueSegmentSnapshotID, func() (segment.HistoricalValueSegmentSnapshot, error) {
		return w.store.CreateHistoricalValueSegmentSnapshot(ctx, value)
	}, func(id int64) (segment.HistoricalValueSegmentSnapshot, error) {
		return w.store.GetHistoricalValueSegmentSnapshot(ctx, id)
	})
}

func (w *MarketingStateHistoryWriter) ImportValueSegmentChange(ctx context.Context, source string, value segment.HistoricalValueSegmentChange) (segment.MarketingStateHistoryReceipt, error) {
	value = normalizeValueSegmentChange(value)
	return importMarketingState(w, ctx, valueSegmentChangeKind, source, value, HistoricalValueSegmentChangeDigest, withValueSegmentChangeID, func() (segment.HistoricalValueSegmentChange, error) {
		return w.store.CreateHistoricalValueSegmentChange(ctx, value)
	}, func(id int64) (segment.HistoricalValueSegmentChange, error) {
		return w.store.GetHistoricalValueSegmentChange(ctx, id)
	})
}

func importMarketingState[T any](w *MarketingStateHistoryWriter, ctx context.Context, kind, source string, value T, digest func(T) ([32]byte, error), withID func(T, int64) T, create func() (T, error), get func(int64) (T, error)) (segment.MarketingStateHistoryReceipt, error) {
	var empty segment.MarketingStateHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilMarketingState(w.store) || nilMarketingState(w.journal) {
		return empty, segment.ErrMarketingStateHistoryUnavailable
	}
	key, payload, id, ok := marketingStateIdentity(value)
	if !ok || id != 0 || key == ([32]byte{}) || payload == ([32]byte{}) || source != hex.EncodeToString(key[:]) {
		return empty, segment.ErrMarketingStateHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, segment.ErrMarketingStateHistoryInvalid
	}
	receipt, found, err := w.journal.LoadMarketingStateHistory(ctx, kind, source)
	if err != nil {
		return empty, marketingStateError(err)
	}
	if found {
		if !validMarketingStateReceipt(receipt, kind, source, payload) {
			return empty, segment.ErrMarketingStateHistoryConflict
		}
		actual, err := get(receipt.TargetID)
		if err != nil {
			return empty, marketingStateError(err)
		}
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, segment.ErrMarketingStateHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create()
	if err != nil {
		return empty, marketingStateError(err)
	}
	_, _, targetID, ok := marketingStateIdentity(actual)
	if !ok || targetID < 1 {
		return empty, segment.ErrMarketingStateHistoryConflict
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, targetID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, segment.ErrMarketingStateHistoryConflict
	}
	receipt = segment.MarketingStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetDigest: actualDigest, TargetID: targetID}
	if err := w.journal.RecordMarketingStateHistory(ctx, receipt); err != nil {
		return empty, marketingStateError(err)
	}
	return receipt, nil
}

// Digest encodings name every private fact explicitly. Marshaling a Port value
// would omit json:"-" envelope and provenance fields.
func HistoricalMarketingStateSnapshotDigest(v segment.HistoricalMarketingStateSnapshot) ([32]byte, error) {
	if !validMarketingStateSnapshot(v, true) {
		return [32]byte{}, segment.ErrMarketingStateHistoryInvalid
	}
	return marketingStateDigest(marketingStateSnapshotKind, struct {
		ID, SourceID                                                                                                                    int64
		Key, Payload, Field, External, State                                                                                            [32]byte
		Person, Batch                                                                                                                   *int64
		Automation, Main, Sub, Lifecycle, LastActivation, LastConversion, LastMessage, BatchStatus, BatchStart, BatchEnd, Trigger, Exit string
		Activated, Converted, Eligible                                                                                                  bool
		Entered, Exited                                                                                                                 *time.Time
		Created, Updated                                                                                                                time.Time
	}{v.ID, v.SourceID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest, v.PersonSourceID, v.LastBatchSourceID, v.AutomationKey, v.MainStage, v.SubStage, v.LifecycleStatus, v.LastActivationAt, v.LastConversionMarkedAt, v.LastMessageAt, v.LastBatchStatus, v.LastBatchWindowStart, v.LastBatchWindowEnd, v.LastTriggerMessageAt, v.ExitReason, v.Activated, v.Converted, v.EligibleForConversion, v.EnteredAt, v.ExitedAt, v.CreatedAt, v.UpdatedAt})
}
func HistoricalMarketingStateChangeDigest(v segment.HistoricalMarketingStateChange) ([32]byte, error) {
	if !validMarketingStateChange(v, true) {
		return [32]byte{}, segment.ErrMarketingStateHistoryInvalid
	}
	return marketingStateDigest(marketingStateChangeKind, struct {
		ID, SourceID                                                                                int64
		Key, Payload, Field, External, State                                                        [32]byte
		Person, Batch                                                                               *int64
		Automation, Main, Sub, Lifecycle, LastActivation, LastConversion, LastMessage, Exit, Change string
		Activated, Converted, Eligible                                                              bool
		Recorded, Created                                                                           time.Time
	}{v.ID, v.SourceID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest, v.PersonSourceID, v.BatchSourceID, v.AutomationKey, v.MainStage, v.SubStage, v.LifecycleStatus, v.LastActivationAt, v.LastConversionMarkedAt, v.LastMessageAt, v.ExitReason, v.ChangeReason, v.Activated, v.Converted, v.EligibleForConversion, v.RecordedAt, v.CreatedAt})
}
func HistoricalValueSegmentSnapshotDigest(v segment.HistoricalValueSegmentSnapshot) ([32]byte, error) {
	if !validValueSegmentSnapshot(v, true) {
		return [32]byte{}, segment.ErrMarketingStateHistoryInvalid
	}
	return marketingStateDigest(valueSegmentSnapshotKind, struct {
		ID, SourceID                                  int64
		Key, Payload, Field, External, Matched, State [32]byte
		Segment, Version, Reason                      string
		Rank, Score                                   int32
		Submission                                    *int64
		Evaluated, Computed, Created, Updated         time.Time
	}{v.ID, v.SourceID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.MatchedQuestionIDsDigest, v.StatePayloadDigest, v.Segment, v.ScoringVersion, v.ComputedReason, v.SegmentRank, v.Score, v.SubmissionSourceID, v.EvaluatedAt, v.ComputedAt, v.CreatedAt, v.UpdatedAt})
}
func HistoricalValueSegmentChangeDigest(v segment.HistoricalValueSegmentChange) ([32]byte, error) {
	if !validValueSegmentChange(v, true) {
		return [32]byte{}, segment.ErrMarketingStateHistoryInvalid
	}
	return marketingStateDigest(valueSegmentChangeKind, struct {
		ID, SourceID                                  int64
		Key, Payload, Field, External, Matched, State [32]byte
		Segment, Version, Reason                      string
		Rank, Score                                   int32
		Submission                                    *int64
		Evaluated, Recorded, Created                  time.Time
	}{v.ID, v.SourceID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.MatchedQuestionIDsDigest, v.StatePayloadDigest, v.Segment, v.ScoringVersion, v.ChangeReason, v.SegmentRank, v.Score, v.SubmissionSourceID, v.EvaluatedAt, v.RecordedAt, v.CreatedAt})
}

func marketingStateDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{kind, value})
	if err != nil {
		return [32]byte{}, segment.ErrMarketingStateHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validMarketingStateIdentity(id int64, key, payload, field [32]byte, stored bool) bool {
	return (stored && id > 0 || !stored && id == 0) && key != ([32]byte{}) && payload != ([32]byte{}) && field != ([32]byte{})
}
func validMarketingStateTime(v time.Time, stored bool) bool {
	return !v.IsZero() && (!stored || v.Location() == time.UTC && v.Equal(v.UTC().Truncate(time.Microsecond)))
}
func validOptionalMarketingStateTime(v *time.Time, stored bool) bool {
	return v == nil || validMarketingStateTime(*v, stored)
}
func validMarketingDigests(key, payload, field, external, state [32]byte) bool {
	return key != ([32]byte{}) && payload != ([32]byte{}) && field != ([32]byte{}) && external != ([32]byte{}) && state != ([32]byte{})
}
func validMarketingStateSnapshot(v segment.HistoricalMarketingStateSnapshot, stored bool) bool {
	return validMarketingStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validMarketingDigests(v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest) && validOptionalMarketingStateTime(v.EnteredAt, stored) && validOptionalMarketingStateTime(v.ExitedAt, stored) && validMarketingStateTime(v.CreatedAt, stored) && validMarketingStateTime(v.UpdatedAt, stored)
}
func validMarketingStateChange(v segment.HistoricalMarketingStateChange, stored bool) bool {
	return validMarketingStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validMarketingDigests(v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest) && validMarketingStateTime(v.RecordedAt, stored) && validMarketingStateTime(v.CreatedAt, stored)
}
func validValueSegmentSnapshot(v segment.HistoricalValueSegmentSnapshot, stored bool) bool {
	return validMarketingStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validMarketingDigests(v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest) && v.MatchedQuestionIDsDigest != ([32]byte{}) && validMarketingStateTime(v.EvaluatedAt, stored) && validMarketingStateTime(v.ComputedAt, stored) && validMarketingStateTime(v.CreatedAt, stored) && validMarketingStateTime(v.UpdatedAt, stored)
}
func validValueSegmentChange(v segment.HistoricalValueSegmentChange, stored bool) bool {
	return validMarketingStateIdentity(v.ID, v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, stored) && validMarketingDigests(v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest, v.ExternalUserIDDigest, v.StatePayloadDigest) && v.MatchedQuestionIDsDigest != ([32]byte{}) && validMarketingStateTime(v.EvaluatedAt, stored) && validMarketingStateTime(v.RecordedAt, stored) && validMarketingStateTime(v.CreatedAt, stored)
}

func normalizeMarketingStateTime(v time.Time) time.Time { return v.UTC().Truncate(time.Microsecond) }
func normalizeOptionalMarketingStateTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	t := normalizeMarketingStateTime(*v)
	return &t
}
func normalizeMarketingStateSnapshot(v segment.HistoricalMarketingStateSnapshot) segment.HistoricalMarketingStateSnapshot {
	v.EnteredAt, v.ExitedAt = normalizeOptionalMarketingStateTime(v.EnteredAt), normalizeOptionalMarketingStateTime(v.ExitedAt)
	v.CreatedAt, v.UpdatedAt = normalizeMarketingStateTime(v.CreatedAt), normalizeMarketingStateTime(v.UpdatedAt)
	return v
}
func normalizeMarketingStateChange(v segment.HistoricalMarketingStateChange) segment.HistoricalMarketingStateChange {
	v.RecordedAt, v.CreatedAt = normalizeMarketingStateTime(v.RecordedAt), normalizeMarketingStateTime(v.CreatedAt)
	return v
}
func normalizeValueSegmentSnapshot(v segment.HistoricalValueSegmentSnapshot) segment.HistoricalValueSegmentSnapshot {
	v.EvaluatedAt, v.ComputedAt, v.CreatedAt, v.UpdatedAt = normalizeMarketingStateTime(v.EvaluatedAt), normalizeMarketingStateTime(v.ComputedAt), normalizeMarketingStateTime(v.CreatedAt), normalizeMarketingStateTime(v.UpdatedAt)
	return v
}
func normalizeValueSegmentChange(v segment.HistoricalValueSegmentChange) segment.HistoricalValueSegmentChange {
	v.EvaluatedAt, v.RecordedAt, v.CreatedAt = normalizeMarketingStateTime(v.EvaluatedAt), normalizeMarketingStateTime(v.RecordedAt), normalizeMarketingStateTime(v.CreatedAt)
	return v
}

func withMarketingStateSnapshotID(v segment.HistoricalMarketingStateSnapshot, id int64) segment.HistoricalMarketingStateSnapshot {
	v.ID = id
	return v
}
func withMarketingStateChangeID(v segment.HistoricalMarketingStateChange, id int64) segment.HistoricalMarketingStateChange {
	v.ID = id
	return v
}
func withValueSegmentSnapshotID(v segment.HistoricalValueSegmentSnapshot, id int64) segment.HistoricalValueSegmentSnapshot {
	v.ID = id
	return v
}
func withValueSegmentChangeID(v segment.HistoricalValueSegmentChange, id int64) segment.HistoricalValueSegmentChange {
	v.ID = id
	return v
}
func marketingStateIdentity(value any) ([32]byte, [32]byte, int64, bool) {
	switch v := value.(type) {
	case segment.HistoricalMarketingStateSnapshot:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case segment.HistoricalMarketingStateChange:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case segment.HistoricalValueSegmentSnapshot:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	case segment.HistoricalValueSegmentChange:
		return v.SourceKeyDigest, v.SourcePayloadDigest, v.ID, true
	}
	return [32]byte{}, [32]byte{}, 0, false
}
func validMarketingStateReceipt(v segment.MarketingStateHistoryReceipt, kind, source string, payload [32]byte) bool {
	return v.Kind == kind && v.SourceIdentifier == source && v.PayloadDigest == payload && v.TargetID > 0 && v.TargetDigest != ([32]byte{})
}
func marketingStateError(err error) error {
	if errors.Is(err, segment.ErrMarketingStateHistoryInvalid) {
		return segment.ErrMarketingStateHistoryInvalid
	}
	if errors.Is(err, segment.ErrMarketingStateHistoryConflict) {
		return segment.ErrMarketingStateHistoryConflict
	}
	return segment.ErrMarketingStateHistoryUnavailable
}
func nilMarketingState(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}
