package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

// MessageHistoryWriter persists only the caller-provided, non-executable V1
// projection and its same-transaction receipt. It has no queue, event, or
// Provider dependency.
type MessageHistoryWriter struct {
	store   wecomport.MessageHistoryStore
	journal wecomport.MessageHistoryJournal
}

func NewMessageHistoryWriter(store wecomport.MessageHistoryStore, journal wecomport.MessageHistoryJournal) *MessageHistoryWriter {
	return &MessageHistoryWriter{store: store, journal: journal}
}

func (writer *MessageHistoryWriter) Write(ctx context.Context, sourceIdentifier string, payloadDigest [32]byte, value wecomport.HistoricalMessage) (wecomport.MessageHistoryReceipt, error) {
	empty := wecomport.MessageHistoryReceipt{}
	if writer == nil || ctx == nil || isNilDependency(writer.store) || isNilDependency(writer.journal) {
		return empty, wecomport.ErrMessageHistoryUnavailable
	}
	if sourceIdentifier == "" || !validMessageHistoryText(sourceIdentifier) || payloadDigest == ([32]byte{}) || value.ID != 0 || value.SourcePayloadDigest != payloadDigest || !validMessageHistory(value, false) {
		return empty, wecomport.ErrMessageHistoryInvalid
	}
	value = normalizeHistoricalMessage(value)
	if _, err := HistoricalMessageDigest(withHistoricalMessageID(value, 1)); err != nil {
		return empty, wecomport.ErrMessageHistoryInvalid
	}

	receipt, found, err := writer.journal.LoadMessageHistory(ctx, sourceIdentifier)
	if err != nil {
		return empty, messageHistoryError(err)
	}
	if found {
		if receipt.SourceIdentifier != sourceIdentifier || receipt.PayloadDigest != payloadDigest || receipt.TargetID < 1 || receipt.TargetDigest == ([32]byte{}) {
			return empty, wecomport.ErrMessageHistoryConflict
		}
		actual, err := writer.store.GetHistoricalMessage(ctx, receipt.TargetID)
		if err != nil {
			return empty, messageHistoryError(err)
		}
		actualDigest, actualErr := HistoricalMessageDigest(actual)
		expectedDigest, expectedErr := HistoricalMessageDigest(withHistoricalMessageID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actual.ID != receipt.TargetID || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, wecomport.ErrMessageHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}

	actual, err := writer.store.CreateHistoricalMessage(ctx, value)
	if err != nil {
		return empty, messageHistoryError(err)
	}
	actualDigest, actualErr := HistoricalMessageDigest(actual)
	expectedDigest, expectedErr := HistoricalMessageDigest(withHistoricalMessageID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, wecomport.ErrMessageHistoryConflict
	}
	receipt = wecomport.MessageHistoryReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: payloadDigest, TargetID: actual.ID, TargetDigest: actualDigest}
	if err = writer.journal.RecordMessageHistory(ctx, receipt); err != nil {
		return empty, messageHistoryError(err)
	}
	return receipt, nil
}

// HistoricalMessageDigest covers every persisted history field, including the
// generated target ID, so replay and reconciliation detect target drift.
func HistoricalMessageDigest(value wecomport.HistoricalMessage) ([32]byte, error) {
	if !validMessageHistory(value, true) {
		return [32]byte{}, wecomport.ErrMessageHistoryInvalid
	}
	encoded, err := json.Marshal(normalizeHistoricalMessage(value))
	if err != nil {
		return [32]byte{}, wecomport.ErrMessageHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validMessageHistory(value wecomport.HistoricalMessage, requireID bool) bool {
	if (requireID && value.ID < 1) || (!requireID && value.ID != 0) || value.SourceID < 1 ||
		(value.CustomerID != nil && *value.CustomerID < 1) || value.SourcePayloadDigest == ([32]byte{}) || value.CreatedAt.IsZero() ||
		!validMessageHistoryText(value.ChatType) || !validMessageHistoryText(value.MessageType) || !validMessageHistoryText(value.OriginalSendTime) {
		return false
	}
	if value.ContentMasked != nil && !validMessageHistoryText(*value.ContentMasked) {
		return false
	}
	switch value.SendTimeBasis {
	case "civil_unzoned":
		return value.SentAt == nil && validMessageHistoryCivilTime(value.OriginalSendTime)
	case "explicit_offset":
		if value.SentAt == nil || value.SentAt.IsZero() {
			return false
		}
		parsed, ok := parseMessageHistoryExplicitTime(value.OriginalSendTime)
		return ok && parsed.Equal(value.SentAt.UTC().Truncate(time.Microsecond))
	default:
		return false
	}
}

func validMessageHistoryText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validMessageHistoryCivilTime(value string) bool {
	const layout = "2006-01-02 15:04:05"
	parsed, err := time.Parse(layout, value)
	return err == nil && parsed.Year() > 0 && parsed.Format(layout) == value
}

func parseMessageHistoryExplicitTime(value string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999Z07",
		"2006-01-02 15:04:05.999999999Z0700",
	} {
		if parsed, err := time.Parse(layout, value); err == nil && !parsed.IsZero() {
			return parsed.UTC().Truncate(time.Microsecond), true
		}
	}
	return time.Time{}, false
}

func normalizeHistoricalMessage(value wecomport.HistoricalMessage) wecomport.HistoricalMessage {
	value.CreatedAt = value.CreatedAt.UTC().Truncate(time.Microsecond)
	if value.SentAt != nil {
		sent := value.SentAt.UTC().Truncate(time.Microsecond)
		value.SentAt = &sent
	}
	if value.Sequence != nil {
		sequence := *value.Sequence
		value.Sequence = &sequence
	}
	if value.CustomerID != nil {
		customerID := *value.CustomerID
		value.CustomerID = &customerID
	}
	if value.ContentMasked != nil {
		content := *value.ContentMasked
		value.ContentMasked = &content
	}
	return value
}

func withHistoricalMessageID(value wecomport.HistoricalMessage, id int64) wecomport.HistoricalMessage {
	value.ID = id
	return normalizeHistoricalMessage(value)
}

func messageHistoryError(err error) error {
	switch {
	case errors.Is(err, wecomport.ErrMessageHistoryInvalid):
		return wecomport.ErrMessageHistoryInvalid
	case errors.Is(err, wecomport.ErrMessageHistoryConflict):
		return wecomport.ErrMessageHistoryConflict
	default:
		return wecomport.ErrMessageHistoryUnavailable
	}
}
