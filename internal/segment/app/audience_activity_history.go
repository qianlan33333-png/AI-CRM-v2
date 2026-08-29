package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// AudienceActivityHistoryWriter records frozen V1 activity facts in the
// caller transaction. It has no current Segment, event, queue, or Provider
// dependency.
type AudienceActivityHistoryWriter struct {
	store   segmentport.AudienceActivityHistoryStore
	journal segmentport.AudienceActivityHistoryJournal
}

func NewAudienceActivityHistoryWriter(store segmentport.AudienceActivityHistoryStore, journal segmentport.AudienceActivityHistoryJournal) *AudienceActivityHistoryWriter {
	return &AudienceActivityHistoryWriter{store: store, journal: journal}
}

func (writer *AudienceActivityHistoryWriter) WriteRun(ctx context.Context, source string, payload [32]byte, value segmentport.HistoricalAudienceActivityRun) (segmentport.AudienceActivityHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	value = normalizeAudienceActivityRun(value)
	if _, err := HistoricalAudienceActivityRunDigest(withAudienceActivityRunID(value, 1)); err != nil {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	if err := writer.validateRunParents(ctx, value); err != nil {
		return segmentport.AudienceActivityHistoryReceipt{}, err
	}
	return writeAudienceActivityHistory(ctx, writer.journal, "package_runs", source, payload, value,
		func(v segmentport.HistoricalAudienceActivityRun) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceActivityRun, id int64) segmentport.HistoricalAudienceActivityRun {
			v.ID = id
			return normalizeAudienceActivityRun(v)
		},
		HistoricalAudienceActivityRunDigest, writer.store.CreateHistoricalAudienceActivityRun, writer.store.GetHistoricalAudienceActivityRun)
}

func (writer *AudienceActivityHistoryWriter) WriteMemberEvent(ctx context.Context, source string, payload [32]byte, value segmentport.HistoricalAudienceActivityMemberEvent) (segmentport.AudienceActivityHistoryReceipt, error) {
	if !writer.ready(ctx) {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryUnavailable
	}
	if value.ID != 0 {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	value = normalizeAudienceActivityEvent(value)
	if _, err := HistoricalAudienceActivityMemberEventDigest(withAudienceActivityEventID(value, 1)); err != nil {
		return segmentport.AudienceActivityHistoryReceipt{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	if err := writer.validateEventParents(ctx, value); err != nil {
		return segmentport.AudienceActivityHistoryReceipt{}, err
	}
	return writeAudienceActivityHistory(ctx, writer.journal, "member_events", source, payload, value,
		func(v segmentport.HistoricalAudienceActivityMemberEvent) int64 { return v.ID },
		func(v segmentport.HistoricalAudienceActivityMemberEvent, id int64) segmentport.HistoricalAudienceActivityMemberEvent {
			v.ID = id
			return normalizeAudienceActivityEvent(v)
		},
		HistoricalAudienceActivityMemberEventDigest, writer.store.CreateHistoricalAudienceActivityMemberEvent, writer.store.GetHistoricalAudienceActivityMemberEvent)
}

func (writer *AudienceActivityHistoryWriter) ready(ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !nilCRUD(writer.store) && !nilCRUD(writer.journal)
}

func (writer *AudienceActivityHistoryWriter) validateRunParents(ctx context.Context, value segmentport.HistoricalAudienceActivityRun) error {
	packageRef, err := writer.store.GetHistoricalAudienceActivityPackage(ctx, value.PackageHistoryID)
	if err != nil || packageRef.ID != value.PackageHistoryID || packageRef.ID < 1 {
		return audienceActivityParentError(err)
	}
	if value.VersionHistoryID == nil {
		return nil
	}
	versionRef, err := writer.store.GetHistoricalAudienceActivityVersion(ctx, *value.VersionHistoryID)
	if err != nil || versionRef.ID != *value.VersionHistoryID || versionRef.ID < 1 || versionRef.PackageHistoryID != value.PackageHistoryID {
		return audienceActivityParentError(err)
	}
	return nil
}

func (writer *AudienceActivityHistoryWriter) validateEventParents(ctx context.Context, value segmentport.HistoricalAudienceActivityMemberEvent) error {
	packageRef, err := writer.store.GetHistoricalAudienceActivityPackage(ctx, value.PackageHistoryID)
	if err != nil || packageRef.ID != value.PackageHistoryID || packageRef.ID < 1 {
		return audienceActivityParentError(err)
	}
	if value.RunHistoryID != nil {
		run, readErr := writer.store.GetHistoricalAudienceActivityRun(ctx, *value.RunHistoryID)
		if readErr != nil || run.ID != *value.RunHistoryID || run.PackageHistoryID != value.PackageHistoryID {
			return audienceActivityParentError(readErr)
		}
	}
	if value.MemberHistoryID != nil {
		memberRef, readErr := writer.store.GetHistoricalAudienceActivityMember(ctx, *value.MemberHistoryID)
		if readErr != nil || memberRef.ID != *value.MemberHistoryID || memberRef.ID < 1 || memberRef.PackageHistoryID != value.PackageHistoryID {
			return audienceActivityParentError(readErr)
		}
	}
	return nil
}

func HistoricalAudienceActivityRunDigest(value segmentport.HistoricalAudienceActivityRun) ([32]byte, error) {
	value = normalizeAudienceActivityRun(value)
	if value.ID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) || value.SourceID < 1 || value.PackageHistoryID < 1 || (value.VersionHistoryID != nil && *value.VersionHistoryID < 1) || value.PrivateDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	return audienceActivityDigest("package_runs", struct {
		ID                  int64
		SourceKeyDigest     [32]byte
		SourcePayloadDigest [32]byte
		SourceFieldDigest   [32]byte
		SourceID            int64
		PackageHistoryID    int64
		VersionHistoryID    *int64
		RunType             string
		OriginalStatus      string
		RefreshStartedAt    time.Time
		RefreshFinishedAt   *time.Time
		LastWatermarkAt     *time.Time
		NextWatermarkAt     *time.Time
		ReturnedCount       int32
		EnteredCount        int32
		UpdatedCount        int32
		ExitedCount         int32
		MemberEventCount    int32
		DurationMS          int32
		CreatedAt           time.Time
		PrivateDigest       [32]byte
	}{value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.SourceID, value.PackageHistoryID, value.VersionHistoryID, value.RunType, value.OriginalStatus, value.RefreshStartedAt, value.RefreshFinishedAt, value.LastWatermarkAt, value.NextWatermarkAt, value.ReturnedCount, value.EnteredCount, value.UpdatedCount, value.ExitedCount, value.MemberEventCount, value.DurationMS, value.CreatedAt, value.PrivateDigest})
}

func HistoricalAudienceActivityMemberEventDigest(value segmentport.HistoricalAudienceActivityMemberEvent) ([32]byte, error) {
	value = normalizeAudienceActivityEvent(value)
	if value.ID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) || value.SourceID < 1 || value.PackageHistoryID < 1 || (value.RunHistoryID != nil && *value.RunHistoryID < 1) || (value.MemberHistoryID != nil && *value.MemberHistoryID < 1) || value.PrivateDigest == ([32]byte{}) {
		return [32]byte{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	return audienceActivityDigest("member_events", struct {
		ID                  int64
		SourceKeyDigest     [32]byte
		SourcePayloadDigest [32]byte
		SourceFieldDigest   [32]byte
		SourceID            int64
		PackageHistoryID    int64
		RunHistoryID        *int64
		MemberHistoryID     *int64
		EventType           string
		IdentityKind        string
		OccurredAt          time.Time
		CreatedAt           time.Time
		PrivateDigest       [32]byte
	}{value.ID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.SourceID, value.PackageHistoryID, value.RunHistoryID, value.MemberHistoryID, value.EventType, value.IdentityKind, value.OccurredAt, value.CreatedAt, value.PrivateDigest})
}

func writeAudienceActivityHistory[T any](ctx context.Context, journal segmentport.AudienceActivityHistoryJournal, kind, source string, payload [32]byte, value T, id func(T) int64, withID func(T, int64) T, digest func(T) ([32]byte, error), create func(context.Context, T) (T, error), get func(context.Context, int64) (T, error)) (segmentport.AudienceActivityHistoryReceipt, error) {
	empty := segmentport.AudienceActivityHistoryReceipt{}
	if source == "" || payload == ([32]byte{}) {
		return empty, segmentport.ErrAudienceActivityHistoryInvalid
	}
	if _, err := digest(withID(value, 1)); err != nil {
		return empty, segmentport.ErrAudienceActivityHistoryInvalid
	}
	receipt, found, err := journal.LoadAudienceActivityHistory(ctx, kind, source)
	if err != nil {
		return empty, audienceActivityHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, segmentport.ErrAudienceActivityHistoryConflict
		}
		actual, readErr := get(ctx, receipt.TargetID)
		actualDigest, actualErr := digest(actual)
		expectedDigest, expectedErr := digest(withID(value, receipt.TargetID))
		if readErr != nil {
			return empty, audienceActivityHistoryError(readErr)
		}
		if actualErr != nil || expectedErr != nil || id(actual) != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, segmentport.ErrAudienceActivityHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := create(ctx, withID(value, 0))
	if err != nil {
		return empty, audienceActivityHistoryError(err)
	}
	actualDigest, actualErr := digest(actual)
	expectedDigest, expectedErr := digest(withID(value, id(actual)))
	if actualErr != nil || expectedErr != nil || id(actual) < 1 || actualDigest != expectedDigest {
		return empty, segmentport.ErrAudienceActivityHistoryConflict
	}
	receipt = segmentport.AudienceActivityHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id(actual), TargetDigest: actualDigest}
	if err = journal.RecordAudienceActivityHistory(ctx, kind, receipt); err != nil {
		return empty, audienceActivityHistoryError(err)
	}
	return receipt, nil
}

func normalizeAudienceActivityRun(value segmentport.HistoricalAudienceActivityRun) segmentport.HistoricalAudienceActivityRun {
	value.RefreshStartedAt = value.RefreshStartedAt.UTC().Truncate(time.Microsecond)
	value.RefreshFinishedAt = audienceActivityTime(value.RefreshFinishedAt)
	value.LastWatermarkAt = audienceActivityTime(value.LastWatermarkAt)
	value.NextWatermarkAt = audienceActivityTime(value.NextWatermarkAt)
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.VersionHistoryID = audienceActivityID(value.VersionHistoryID)
	return value
}

func withAudienceActivityRunID(value segmentport.HistoricalAudienceActivityRun, id int64) segmentport.HistoricalAudienceActivityRun {
	value.ID = id
	return normalizeAudienceActivityRun(value)
}

func normalizeAudienceActivityEvent(value segmentport.HistoricalAudienceActivityMemberEvent) segmentport.HistoricalAudienceActivityMemberEvent {
	value.OccurredAt = value.OccurredAt.UTC().Truncate(time.Microsecond)
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.RunHistoryID = audienceActivityID(value.RunHistoryID)
	value.MemberHistoryID = audienceActivityID(value.MemberHistoryID)
	return value
}

func withAudienceActivityEventID(value segmentport.HistoricalAudienceActivityMemberEvent, id int64) segmentport.HistoricalAudienceActivityMemberEvent {
	value.ID = id
	return normalizeAudienceActivityEvent(value)
}

func audienceActivityTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC().Truncate(time.Microsecond)
	return &copy
}

func audienceActivityID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func audienceActivityDigest(kind string, value any) ([32]byte, error) {
	encoded, err := json.Marshal(struct {
		Kind  string
		Value any
	}{kind, value})
	if err != nil {
		return [32]byte{}, segmentport.ErrAudienceActivityHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func audienceActivityHistoryError(err error) error {
	switch {
	case errors.Is(err, segmentport.ErrAudienceActivityHistoryInvalid):
		return segmentport.ErrAudienceActivityHistoryInvalid
	case errors.Is(err, segmentport.ErrAudienceActivityHistoryConflict):
		return segmentport.ErrAudienceActivityHistoryConflict
	default:
		return segmentport.ErrAudienceActivityHistoryUnavailable
	}
}

func audienceActivityParentError(err error) error {
	if err == nil {
		return segmentport.ErrAudienceActivityHistoryConflict
	}
	return audienceActivityHistoryError(err)
}
