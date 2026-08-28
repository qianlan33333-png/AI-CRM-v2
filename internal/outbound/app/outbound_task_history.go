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

// OutboundTaskHistoryWriter persists only sealed V1 task observations and
// their same-transaction receipts. It has no current task, queue, event, or
// Provider dependency.
type OutboundTaskHistoryWriter struct {
	store   outboundport.OutboundTaskHistoryStore
	journal outboundport.OutboundTaskHistoryJournal
}

func NewOutboundTaskHistoryWriter(store outboundport.OutboundTaskHistoryStore, journal outboundport.OutboundTaskHistoryJournal) *OutboundTaskHistoryWriter {
	return &OutboundTaskHistoryWriter{store: store, journal: journal}
}

func (writer *OutboundTaskHistoryWriter) Import(ctx context.Context, source string, value outboundport.HistoricalOutboundTask) (outboundport.OutboundTaskHistoryReceipt, error) {
	empty := outboundport.OutboundTaskHistoryReceipt{}
	if writer == nil || ctx == nil || ctx.Err() != nil || outboundTaskHistoryNil(writer.store) || outboundTaskHistoryNil(writer.journal) {
		return empty, outboundport.ErrOutboundTaskHistoryUnavailable
	}
	if value.ID != 0 || value.BroadcastJobHistoryID != nil || !validOutboundTaskHistorySource(source, value.SourceKeyDigest) || !validHistoricalOutboundTask(value, false) {
		return empty, outboundport.ErrOutboundTaskHistoryInvalid
	}
	value = normalizeHistoricalOutboundTask(value)
	parentID, err := writer.resolveParent(ctx, value)
	if err != nil {
		return empty, outboundTaskHistoryError(err)
	}
	value.BroadcastJobHistoryID = parentID
	if _, err := HistoricalOutboundTaskDigest(withHistoricalOutboundTaskID(value, 1)); err != nil {
		return empty, outboundport.ErrOutboundTaskHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadOutboundTaskHistory(ctx, source)
	if err != nil {
		return empty, outboundTaskHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != value.SourcePayloadDigest || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, outboundport.ErrOutboundTaskHistoryConflict
		}
		actual, err := writer.store.GetHistoricalOutboundTask(ctx, receipt.TargetID)
		if err != nil {
			return empty, outboundTaskHistoryError(err)
		}
		actualDigest, actualErr := HistoricalOutboundTaskDigest(actual)
		expectedDigest, expectedErr := HistoricalOutboundTaskDigest(withHistoricalOutboundTaskID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, outboundport.ErrOutboundTaskHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := writer.store.CreateHistoricalOutboundTask(ctx, value)
	if err != nil {
		return empty, outboundTaskHistoryError(err)
	}
	actualDigest, actualErr := HistoricalOutboundTaskDigest(actual)
	expectedDigest, expectedErr := HistoricalOutboundTaskDigest(withHistoricalOutboundTaskID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actual.ID < 1 || actualDigest != expectedDigest {
		return empty, outboundport.ErrOutboundTaskHistoryConflict
	}
	receipt = outboundport.OutboundTaskHistoryReceipt{SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err := writer.journal.RecordOutboundTaskHistory(ctx, receipt); err != nil {
		return empty, outboundTaskHistoryError(err)
	}
	return receipt, nil
}

func (writer *OutboundTaskHistoryWriter) resolveParent(ctx context.Context, value outboundport.HistoricalOutboundTask) (*int64, error) {
	if value.LegacyBroadcastJobID == nil {
		return nil, nil
	}
	parents, err := writer.store.LookupOutboundTaskHistoryParents(ctx, *value.LegacyBroadcastJobID)
	if err != nil {
		return nil, err
	}
	if len(parents) != 1 {
		return nil, nil
	}
	parent := parents[0]
	if parent.ID < 1 || parent.SourceID != *value.LegacyBroadcastJobID || parent.LegacyOutboundTaskID == nil || *parent.LegacyOutboundTaskID != value.SourceID {
		return nil, nil
	}
	id := parent.ID
	return &id, nil
}

// HistoricalOutboundTaskDigest covers every persisted field, including
// private field digests, nullable source values, roots, and generated IDs.
func HistoricalOutboundTaskDigest(value outboundport.HistoricalOutboundTask) ([32]byte, error) {
	value = normalizeHistoricalOutboundTask(value)
	if !validHistoricalOutboundTask(value, true) {
		return [32]byte{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	encoded, err := json.Marshal([]any{
		value.ID, value.SourceID, value.TaskType, value.Status, value.CreatedAt, value.BroadcastJobHistoryID,
		value.RequestPayloadDigest, value.ResponsePayloadDigest, value.WeComTaskIDDigest, value.TraceIDDigest,
		value.LegacyBroadcastJobID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.RedactedRoots,
	})
	if err != nil {
		return [32]byte{}, outboundport.ErrOutboundTaskHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHistoricalOutboundTask(value outboundport.HistoricalOutboundTask, stored bool) bool {
	if (stored && value.ID < 1) || (!stored && value.ID != 0) || value.CreatedAt.IsZero() ||
		!validOutboundTaskHistoryText(value.TaskType) || !validOutboundTaskHistoryText(value.Status) ||
		value.RequestPayloadDigest == ([32]byte{}) || value.ResponsePayloadDigest == ([32]byte{}) || value.TraceIDDigest == ([32]byte{}) ||
		value.SourceKeyDigest == ([32]byte{}) || value.SourcePayloadDigest == ([32]byte{}) || value.SourceFieldDigest == ([32]byte{}) {
		return false
	}
	if value.WeComTaskIDDigest != nil && *value.WeComTaskIDDigest == ([32]byte{}) {
		return false
	}
	if value.BroadcastJobHistoryID != nil && *value.BroadcastJobHistoryID < 1 {
		return false
	}
	for _, root := range value.RedactedRoots {
		if !validOutboundTaskHistoryText(root) {
			return false
		}
	}
	return true
}

func normalizeHistoricalOutboundTask(value outboundport.HistoricalOutboundTask) outboundport.HistoricalOutboundTask {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	value.BroadcastJobHistoryID = outboundTaskHistoryID(value.BroadcastJobHistoryID)
	value.LegacyBroadcastJobID = outboundTaskHistoryID(value.LegacyBroadcastJobID)
	if value.WeComTaskIDDigest != nil {
		copy := *value.WeComTaskIDDigest
		value.WeComTaskIDDigest = &copy
	}
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	sort.Strings(value.RedactedRoots)
	return value
}

func withHistoricalOutboundTaskID(value outboundport.HistoricalOutboundTask, id int64) outboundport.HistoricalOutboundTask {
	value.ID = id
	return normalizeHistoricalOutboundTask(value)
}

func outboundTaskHistoryID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validOutboundTaskHistorySource(source string, key [32]byte) bool {
	if key == ([32]byte{}) || len(source) != hex.EncodedLen(sha256.Size) || source != strings.ToLower(source) {
		return false
	}
	decoded, err := hex.DecodeString(source)
	return err == nil && len(decoded) == sha256.Size && string(decoded) == string(key[:])
}

func validOutboundTaskHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func outboundTaskHistoryNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func outboundTaskHistoryError(err error) error {
	switch {
	case errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid):
		return outboundport.ErrOutboundTaskHistoryInvalid
	case errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict):
		return outboundport.ErrOutboundTaskHistoryConflict
	default:
		return outboundport.ErrOutboundTaskHistoryUnavailable
	}
}
