// Package v1hxcchatjobhistory decodes sealed V1 HXC chat-job rows into
// private, inert source facts. It does not restore a job, callback, send, or
// customer relationship.
package v1hxcchatjobhistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const ChatJobsTableID = "public/automation_laohuang_chat_job"

var (
	ErrArchiveRow            = errors.New("HXC chat-job archive row invalid")
	ErrFact                  = errors.New("HXC chat-job fact invalid")
	ErrRequiredFieldRedacted = errors.New("HXC chat-job required source field redacted")
)

// SourceEnvelope binds one fact to the immutable V1 archive row that supplied
// it. These are source proofs, not V2 identifiers.
type SourceEnvelope struct {
	SourceKeyDigest [sha256.Size]byte `json:"-"`
	PayloadDigest   [sha256.Size]byte `json:"-"`
	FieldDigest     [sha256.Size]byte `json:"-"`
}

// ChatJobFact preserves all 21 source columns without assigning any current
// V2 customer, queue, message, or task relation. Private source values are
// deliberately excluded from JSON serialization.
type ChatJobFact struct {
	Source SourceEnvelope `json:"-"`

	SourceID          int64  `json:"source_id"`
	QueueID           *int64 `json:"queue_id"`
	MemberID          *int64 `json:"member_id"`
	ExternalContactID string `json:"-"`
	Phone             string `json:"-"`
	ExternalMessageID string `json:"-"`
	ExternalSessionID string `json:"-"`
	LaohuangTaskID    string `json:"-"`

	RequestPayloadJSON  json.RawMessage `json:"-"`
	AcceptedPayloadJSON json.RawMessage `json:"-"`
	CallbackPayloadJSON json.RawMessage `json:"-"`
	OriginalStatus      string          `json:"status"`
	ReplyText           string          `json:"-"`
	ErrorCode           string          `json:"-"`
	ErrorMessage        string          `json:"-"`
	SendChannel         string          `json:"send_channel"`
	SendRecordID        *int64          `json:"send_record_id"`
	SendResultJSON      json.RawMessage `json:"-"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	FinishedAt          string          `json:"finished_at"`
}

type chatJobSource struct {
	ID                  int64           `json:"id"`
	QueueID             *int64          `json:"queue_id"`
	MemberID            *int64          `json:"member_id"`
	ExternalContactID   string          `json:"external_contact_id"`
	Phone               string          `json:"phone"`
	ExternalMessageID   string          `json:"external_message_id"`
	ExternalSessionID   string          `json:"external_session_id"`
	LaohuangTaskID      string          `json:"laohuang_task_id"`
	RequestPayloadJSON  json.RawMessage `json:"request_payload_json"`
	AcceptedPayloadJSON json.RawMessage `json:"accepted_payload_json"`
	CallbackPayloadJSON json.RawMessage `json:"callback_payload_json"`
	Status              string          `json:"status"`
	ReplyText           string          `json:"reply_text"`
	ErrorCode           string          `json:"error_code"`
	ErrorMessage        string          `json:"error_message"`
	SendChannel         string          `json:"send_channel"`
	SendRecordID        *int64          `json:"send_record_id"`
	SendResultJSON      json.RawMessage `json:"send_result_json"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	FinishedAt          string          `json:"finished_at"`
}

var chatJobFields = []string{
	"id", "queue_id", "member_id", "external_contact_id", "phone", "external_message_id", "external_session_id", "laohuang_task_id",
	"request_payload_json", "accepted_payload_json", "callback_payload_json", "status", "reply_text", "error_code", "error_message", "send_channel",
	"send_record_id", "send_result_json", "created_at", "updated_at", "finished_at",
}

var chatJobNullable = map[string]bool{
	"queue_id": true, "member_id": true, "send_record_id": true,
	"request_payload_json": true, "accepted_payload_json": true, "callback_payload_json": true, "send_result_json": true,
}

// AdaptChatJob authenticates and preserves one sealed V1 chat-job source row.
// FinishedAt intentionally remains source text: it is not inferred as a time.
func AdaptChatJob(row v1archive.ArchivedRow, sourceHMACKey []byte) (ChatJobFact, error) {
	fields, source, err := archiveFields(row, sourceHMACKey)
	if err != nil {
		return ChatJobFact{}, err
	}

	var value chatJobSource
	if err := decodeExact(fields, row.Payload, &value); err != nil || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() ||
		!json.Valid(value.RequestPayloadJSON) || !json.Valid(value.AcceptedPayloadJSON) || !json.Valid(value.CallbackPayloadJSON) || !json.Valid(value.SendResultJSON) {
		return ChatJobFact{}, ErrFact
	}

	return ChatJobFact{
		Source:   source,
		SourceID: value.ID, QueueID: cloneInt64(value.QueueID), MemberID: cloneInt64(value.MemberID),
		ExternalContactID: value.ExternalContactID, Phone: value.Phone, ExternalMessageID: value.ExternalMessageID,
		ExternalSessionID: value.ExternalSessionID, LaohuangTaskID: value.LaohuangTaskID,
		RequestPayloadJSON: cloneJSON(value.RequestPayloadJSON), AcceptedPayloadJSON: cloneJSON(value.AcceptedPayloadJSON),
		CallbackPayloadJSON: cloneJSON(value.CallbackPayloadJSON), OriginalStatus: value.Status, ReplyText: value.ReplyText,
		ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, SendChannel: value.SendChannel,
		SendRecordID: cloneInt64(value.SendRecordID), SendResultJSON: cloneJSON(value.SendResultJSON),
		CreatedAt: utcMicro(value.CreatedAt), UpdatedAt: utcMicro(value.UpdatedAt), FinishedAt: value.FinishedAt,
	}, nil
}

func archiveFields(row v1archive.ArchivedRow, key []byte) (map[string]json.RawMessage, SourceEnvelope, error) {
	zero := [sha256.Size]byte{}
	if len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != ChatJobsTableID || row.SourceOrdinal < 1 ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	if len(roots) != 0 {
		return nil, SourceEnvelope{}, ErrRequiredFieldRedacted
	}
	table := strings.TrimPrefix(ChatJobsTableID, "public/")
	payload, err := v1archive.PayloadHMAC(key, table, row.Payload)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, table, row.RedactedFields)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	fields, err := rawFields(row.Payload)
	if err != nil {
		return nil, SourceEnvelope{}, err
	}
	keyJSON, err := json.Marshal([]json.RawMessage{fields["id"]})
	if err != nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	sourceKey, err := v1archive.SourceKeyHMAC(key, table, keyJSON)
	if err != nil || !hmac.Equal(sourceKey[:], row.SourceKeyHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	return fields, SourceEnvelope{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC}, nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target *chatJobSource) error {
	if len(fields) != len(chatJobFields) {
		return ErrFact
	}
	for _, name := range chatJobFields {
		raw, found := fields[name]
		if !found || !json.Valid(raw) || (!chatJobNullable[name] && isNull(raw)) {
			return ErrFact
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(target); err != nil {
		return ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrFact
	}
	return nil
}

func rawFields(payload []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	fields := map[string]json.RawMessage{}
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrFact
	}
	return fields, nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneJSON(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }
func utcMicro(value time.Time) time.Time              { return value.UTC().Truncate(time.Microsecond) }
func isNull(value json.RawMessage) bool               { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
