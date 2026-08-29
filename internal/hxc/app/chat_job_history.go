package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

// HXCChatJobHistoryWriter records only sealed V1 chat-job observations with
// the existing same-transaction HXC receipt journal.
type HXCChatJobHistoryWriter struct {
	store   hxc.HXCChatJobHistoryStore
	journal hxc.HXCHistoryJournal
}

func NewHXCChatJobHistoryWriter(store hxc.HXCChatJobHistoryStore, journal hxc.HXCHistoryJournal) (*HXCChatJobHistoryWriter, error) {
	if nilHXC(store) || nilHXC(journal) {
		return nil, hxc.ErrHXCHistoryUnavailable
	}
	return &HXCChatJobHistoryWriter{store: store, journal: journal}, nil
}

func (w *HXCChatJobHistoryWriter) ImportChatJob(ctx context.Context, source string, value hxc.HistoricalHXCChatJob) (hxc.HXCHistoryReceipt, error) {
	var empty hxc.HXCHistoryReceipt
	if w == nil || ctx == nil || ctx.Err() != nil || nilHXC(w.store) || nilHXC(w.journal) {
		return empty, hxc.ErrHXCHistoryUnavailable
	}
	value = normalizeHXCChatJob(value)
	if !validHXCChatJob(value, false) || value.ID != 0 || source != hex.EncodeToString(value.SourceKeyDigest[:]) {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	if _, err := HistoricalHXCChatJobDigest(withHXCChatJobID(value, 1)); err != nil {
		return empty, hxc.ErrHXCHistoryInvalid
	}
	receipt, found, err := w.journal.LoadHXCHistory(ctx, hxc.HXCHistoryChatJob, source)
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	if found {
		if !validReceipt(receipt, hxc.HXCHistoryChatJob, source, value.SourcePayloadDigest) {
			return empty, hxc.ErrHXCHistoryConflict
		}
		actual, err := w.store.GetHistoricalHXCChatJob(ctx, receipt.TargetID)
		if err != nil {
			return empty, hxcHistoryError(err)
		}
		actualDigest, actualErr := HistoricalHXCChatJobDigest(actual)
		expectedDigest, expectedErr := HistoricalHXCChatJobDigest(withHXCChatJobID(value, receipt.TargetID))
		if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest || actualDigest != receipt.TargetDigest {
			return empty, hxc.ErrHXCHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	actual, err := w.store.CreateHistoricalHXCChatJob(ctx, value)
	if err != nil {
		return empty, hxcHistoryError(err)
	}
	if actual.ID < 1 {
		return empty, hxc.ErrHXCHistoryConflict
	}
	actualDigest, actualErr := HistoricalHXCChatJobDigest(actual)
	expectedDigest, expectedErr := HistoricalHXCChatJobDigest(withHXCChatJobID(value, actual.ID))
	if actualErr != nil || expectedErr != nil || actualDigest != expectedDigest {
		return empty, hxc.ErrHXCHistoryConflict
	}
	receipt = hxc.HXCHistoryReceipt{Kind: hxc.HXCHistoryChatJob, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetDigest: actualDigest, TargetID: actual.ID}
	if err := w.journal.RecordHXCHistory(ctx, receipt); err != nil {
		return empty, hxcHistoryError(err)
	}
	return receipt, nil
}

// HistoricalHXCChatJobDigest covers every stored field, including source
// HMACs and private values. Raw JSON stays as bytes so its source spelling is
// not rewritten by JSON marshalling before the target digest is made.
func HistoricalHXCChatJobDigest(value hxc.HistoricalHXCChatJob) ([32]byte, error) {
	value = normalizeHXCChatJob(value)
	if !validHXCChatJob(value, true) {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	encoded, err := json.Marshal(struct {
		ID, SourceID                                                   int64
		SourceKeyDigest, SourcePayloadDigest, SourceFieldDigest        [32]byte
		QueueSourceID, MemberSourceID                                  *int64
		ExternalContactID, Phone, ExternalMessageID, ExternalSessionID string
		LaohuangTaskID                                                 string
		RequestPayload, AcceptedPayload, CallbackPayload               []byte
		OriginalStatus                                                 string
		ReplyText, ErrorCode, ErrorMessage, SendChannel                string
		SendRecordSourceID                                             *int64
		SendResult                                                     []byte
		CreatedAt, UpdatedAt                                           time.Time
		FinishedAtSource                                               string
	}{value.ID, value.SourceID, value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest, value.QueueSourceID, value.MemberSourceID, value.ExternalContactID, value.Phone, value.ExternalMessageID, value.ExternalSessionID, value.LaohuangTaskID, []byte(value.RequestPayloadJSON), []byte(value.AcceptedPayloadJSON), []byte(value.CallbackPayloadJSON), value.OriginalStatus, value.ReplyText, value.ErrorCode, value.ErrorMessage, value.SendChannel, value.SendRecordSourceID, []byte(value.SendResultJSON), value.CreatedAt, value.UpdatedAt, value.FinishedAtSource})
	if err != nil {
		return [32]byte{}, hxc.ErrHXCHistoryInvalid
	}
	return sha256.Sum256(encoded), nil
}

func validHXCChatJob(value hxc.HistoricalHXCChatJob, stored bool) bool {
	return (stored && value.ID > 0 || !stored && value.ID == 0) &&
		value.SourceKeyDigest != ([32]byte{}) && value.SourcePayloadDigest != ([32]byte{}) && value.SourceFieldDigest != ([32]byte{}) &&
		json.Valid(value.RequestPayloadJSON) && json.Valid(value.AcceptedPayloadJSON) && json.Valid(value.CallbackPayloadJSON) && json.Valid(value.SendResultJSON) &&
		validTime(value.CreatedAt, stored) && validTime(value.UpdatedAt, stored)
}

func normalizeHXCChatJob(value hxc.HistoricalHXCChatJob) hxc.HistoricalHXCChatJob {
	value.QueueSourceID = cloneHXCChatJobID(value.QueueSourceID)
	value.MemberSourceID = cloneHXCChatJobID(value.MemberSourceID)
	value.SendRecordSourceID = cloneHXCChatJobID(value.SendRecordSourceID)
	value.RequestPayloadJSON = cloneHXCChatJobJSON(value.RequestPayloadJSON)
	value.AcceptedPayloadJSON = cloneHXCChatJobJSON(value.AcceptedPayloadJSON)
	value.CallbackPayloadJSON = cloneHXCChatJobJSON(value.CallbackPayloadJSON)
	value.SendResultJSON = cloneHXCChatJobJSON(value.SendResultJSON)
	value.CreatedAt = normalizeTime(value.CreatedAt)
	value.UpdatedAt = normalizeTime(value.UpdatedAt)
	return value
}

func withHXCChatJobID(value hxc.HistoricalHXCChatJob, id int64) hxc.HistoricalHXCChatJob {
	value.ID = id
	return normalizeHXCChatJob(value)
}

func cloneHXCChatJobID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneHXCChatJobJSON(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage{}, value...)
}
