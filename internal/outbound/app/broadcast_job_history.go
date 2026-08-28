package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

// BroadcastJobHistoryWriter persists only immutable V1 queue observations and
// their same-transaction receipts. It has no current batch, task, event,
// queue, or Provider dependency.
type BroadcastJobHistoryWriter struct {
	store   outboundport.BroadcastJobHistoryStore
	journal outboundport.BroadcastJobHistoryJournal
}

func NewBroadcastJobHistoryWriter(store outboundport.BroadcastJobHistoryStore, journal outboundport.BroadcastJobHistoryJournal) *BroadcastJobHistoryWriter {
	return &BroadcastJobHistoryWriter{store: store, journal: journal}
}

func (writer *BroadcastJobHistoryWriter) Import(ctx context.Context, source string, value outboundport.HistoricalBroadcastJob) (outboundport.BroadcastJobHistoryReceipt, error) {
	empty := outboundport.BroadcastJobHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || broadcastJobHistoryNil(writer.store) || broadcastJobHistoryNil(writer.journal) {
		return empty, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	if !validBroadcastJobHistorySource(source, value.SourceKeyDigest) || value.ID != 0 || !validHistoricalBroadcastJob(value, false) {
		return empty, outboundport.ErrBroadcastJobHistoryInvalid
	}
	value = normalizeHistoricalBroadcastJob(value)
	if _, err := HistoricalBroadcastJobDigest(withHistoricalBroadcastJobID(value, 1)); err != nil {
		return empty, outboundport.ErrBroadcastJobHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadBroadcastJobHistory(ctx, source)
	if err != nil {
		return empty, broadcastJobHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != value.SourcePayloadDigest || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, outboundport.ErrBroadcastJobHistoryConflict
		}
		actual, err := writer.store.GetHistoricalBroadcastJob(ctx, receipt.TargetID)
		if err != nil {
			return empty, broadcastJobHistoryError(err)
		}
		actualDigest, actualErr := HistoricalBroadcastJobDigest(actual)
		expectedDigest, expectedErr := HistoricalBroadcastJobDigest(withHistoricalBroadcastJobID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, outboundport.ErrBroadcastJobHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalBroadcastJob(ctx, value)
	if err != nil {
		return empty, broadcastJobHistoryError(err)
	}
	actualDigest, actualErr := HistoricalBroadcastJobDigest(actual)
	expectedDigest, expectedErr := HistoricalBroadcastJobDigest(withHistoricalBroadcastJobID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, outboundport.ErrBroadcastJobHistoryConflict
	}
	receipt = outboundport.BroadcastJobHistoryReceipt{SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordBroadcastJobHistory(ctx, receipt); err != nil {
		return empty, broadcastJobHistoryError(err)
	}
	return receipt, nil
}

// HistoricalBroadcastJobDigest covers every persisted field including fields
// excluded from JSON APIs, generated target ID, optional values, and roots.
func HistoricalBroadcastJobDigest(value outboundport.HistoricalBroadcastJob) ([32]byte, error) {
	if !validHistoricalBroadcastJob(value, true) {
		return [32]byte{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	value = normalizeHistoricalBroadcastJob(value)
	encoded, err := json.Marshal([]any{
		value.ID, value.SourceID, value.OriginalSourceType, value.SourceReferenceDigest, value.SourceTable, value.ScheduledFor, value.Priority, value.BatchKeyDigest, value.OriginalStatus, value.RequiresApproval,
		value.ApprovedByDigest, value.ApprovedAt, value.CancelledByDigest, value.CancelledAt, value.CancelReasonDigest, value.TargetCount, value.TargetSummaryDigest, value.ContentType, value.ContentPayloadDigest, value.ContentSummaryDigest,
		value.AttemptCount, value.LastErrorDigest, value.LegacyOutboundTaskID, value.SentCount, value.FailedCount, value.TraceIDDigest, value.CreatedByDigest, value.CreatedAt, value.UpdatedAt, value.ClaimedAt,
		value.SentAt, value.ClaimTokenDigest, value.LeaseExpiresAt, value.BusinessDomain, value.IdempotencyKeyDigest, value.Channel, value.TargetKind, value.FailureType, value.RetryPolicyDigest, value.MetadataDigest,
		value.TargetUnionIDsDigest, value.MaxAttempts, value.NextRetryAt, value.DispatchStartedAt, value.SideEffectExecuted, value.ProviderResultReceived, value.ResultSummaryDigest, value.ReconciliationRequired, value.CompletedAt,
		value.HoldReasonDigest, value.HoldAt, value.LegacyExternalEffectJobID, value.ExecutionIDDigest, value.ExecutionOwnerDigest, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.RedactedRoots,
	})
	if err != nil {
		return [32]byte{}, outboundport.ErrBroadcastJobHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalBroadcastJob(value outboundport.HistoricalBroadcastJob, requireID bool) bool {
	if (requireID && value.ID < 1) || (!requireID && value.ID != 0) || value.SourceID < 1 || value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) ||
		!validBroadcastJobHistoryText(value.OriginalSourceType) || !validBroadcastJobHistoryText(value.SourceTable) || !validBroadcastJobHistoryText(value.OriginalStatus) || !validBroadcastJobHistoryText(value.ContentType) ||
		value.ScheduledFor.IsZero() || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() || !validBroadcastJobHistoryDigests(value) {
		return false
	}
	for _, candidate := range []*time.Time{value.ApprovedAt, value.CancelledAt, value.ClaimedAt, value.SentAt, value.LeaseExpiresAt, value.NextRetryAt, value.DispatchStartedAt, value.CompletedAt, value.HoldAt} {
		if candidate != nil && candidate.IsZero() {
			return false
		}
	}
	for _, candidate := range []*int64{value.LegacyOutboundTaskID, value.LegacyExternalEffectJobID} {
		if candidate != nil {
			_ = *candidate
		}
	}
	for _, candidate := range []*string{value.BusinessDomain, value.Channel, value.TargetKind, value.FailureType} {
		if candidate != nil && !validBroadcastJobHistoryText(*candidate) {
			return false
		}
	}
	if value.IdempotencyKeyDigest != nil && *value.IdempotencyKeyDigest == ([32]byte{}) {
		return false
	}
	for _, root := range value.RedactedRoots {
		if !validBroadcastJobHistoryText(root) {
			return false
		}
	}
	return true
}

func validBroadcastJobHistoryDigests(value outboundport.HistoricalBroadcastJob) bool {
	for _, digest := range [][32]byte{value.SourceReferenceDigest, value.BatchKeyDigest, value.ApprovedByDigest, value.CancelledByDigest, value.CancelReasonDigest, value.TargetSummaryDigest, value.ContentPayloadDigest, value.ContentSummaryDigest, value.LastErrorDigest, value.TraceIDDigest, value.CreatedByDigest, value.ClaimTokenDigest, value.RetryPolicyDigest, value.MetadataDigest, value.TargetUnionIDsDigest, value.ResultSummaryDigest, value.HoldReasonDigest, value.ExecutionIDDigest, value.ExecutionOwnerDigest} {
		if digest == ([32]byte{}) {
			return false
		}
	}
	return true
}

func normalizeHistoricalBroadcastJob(value outboundport.HistoricalBroadcastJob) outboundport.HistoricalBroadcastJob {
	value.ScheduledFor, value.CreatedAt, value.UpdatedAt = broadcastJobHistoryTime(value.ScheduledFor), broadcastJobHistoryTime(value.CreatedAt), broadcastJobHistoryTime(value.UpdatedAt)
	value.ApprovedAt, value.CancelledAt, value.ClaimedAt, value.SentAt, value.LeaseExpiresAt = broadcastJobHistoryTimePtr(value.ApprovedAt), broadcastJobHistoryTimePtr(value.CancelledAt), broadcastJobHistoryTimePtr(value.ClaimedAt), broadcastJobHistoryTimePtr(value.SentAt), broadcastJobHistoryTimePtr(value.LeaseExpiresAt)
	value.NextRetryAt, value.DispatchStartedAt, value.CompletedAt, value.HoldAt = broadcastJobHistoryTimePtr(value.NextRetryAt), broadcastJobHistoryTimePtr(value.DispatchStartedAt), broadcastJobHistoryTimePtr(value.CompletedAt), broadcastJobHistoryTimePtr(value.HoldAt)
	value.LegacyOutboundTaskID, value.LegacyExternalEffectJobID = broadcastJobHistoryIDPtr(value.LegacyOutboundTaskID), broadcastJobHistoryIDPtr(value.LegacyExternalEffectJobID)
	value.BusinessDomain, value.Channel, value.TargetKind, value.FailureType = broadcastJobHistoryTextPtr(value.BusinessDomain), broadcastJobHistoryTextPtr(value.Channel), broadcastJobHistoryTextPtr(value.TargetKind), broadcastJobHistoryTextPtr(value.FailureType)
	if value.IdempotencyKeyDigest != nil {
		copy := *value.IdempotencyKeyDigest
		value.IdempotencyKeyDigest = &copy
	}
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	sort.Strings(value.RedactedRoots)
	return value
}

func withHistoricalBroadcastJobID(value outboundport.HistoricalBroadcastJob, id int64) outboundport.HistoricalBroadcastJob {
	value.ID = id
	return normalizeHistoricalBroadcastJob(value)
}
func broadcastJobHistoryTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func broadcastJobHistoryTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := broadcastJobHistoryTime(*value)
	return &copy
}
func broadcastJobHistoryIDPtr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func broadcastJobHistoryTextPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func validBroadcastJobHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validBroadcastJobHistorySource(source string, key [32]byte) bool {
	if key == ([32]byte{}) || len(source) != hex.EncodedLen(sha256.Size) || source != strings.ToLower(source) {
		return false
	}
	decoded, err := hex.DecodeString(source)
	return err == nil && len(decoded) == sha256.Size && string(decoded) == string(key[:])
}

func broadcastJobHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func broadcastJobHistoryError(err error) error {
	switch {
	case errors.Is(err, outboundport.ErrBroadcastJobHistoryInvalid):
		return outboundport.ErrBroadcastJobHistoryInvalid
	case errors.Is(err, outboundport.ErrBroadcastJobHistoryConflict):
		return outboundport.ErrBroadcastJobHistoryConflict
	default:
		return outboundport.ErrBroadcastJobHistoryUnavailable
	}
}
