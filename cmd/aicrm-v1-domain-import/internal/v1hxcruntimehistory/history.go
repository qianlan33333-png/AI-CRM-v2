// Package v1hxcruntimehistory decodes frozen V1 HXC sender configuration and
// send records as inert historical facts. It has no sender, queue, Provider,
// customer, staff, or outbound-task dependency.
package v1hxcruntimehistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	SenderConfigTableID = "public/user_ops_hxc_send_config"
	SendRecordsTableID  = "public/user_ops_send_records_next"
)

var ErrInvalidSource = errors.New("hxc runtime history source invalid")

// OpaqueDigest is a keyed comparison value, not source material.
type OpaqueDigest [sha256.Size]byte

// SenderConfigFact is an observed legacy sender configuration. OriginalIsActive
// never grants current sender authorization.
type SenderConfigFact struct {
	SourceID         int64        `json:"source_id"`
	Priority         int64        `json:"priority"`
	OriginalIsActive bool         `json:"original_is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	PrivateDigest    OpaqueDigest `json:"-"`
}

// SendRecordFact is one legacy aggregate send observation. Its original status
// and counts never create, retry, or prove a V2 send or Provider effect.
type SendRecordFact struct {
	SourceID            int64        `json:"source_id"`
	TaskType            string       `json:"task_type"`
	OriginalStatus      string       `json:"original_status"`
	SelectedCount       int64        `json:"selected_count"`
	EligibleCount       int64        `json:"eligible_count"`
	SentCount           int64        `json:"sent_count"`
	SkippedCount        int64        `json:"skipped_count"`
	ImageCount          int64        `json:"image_count"`
	PlannedCount        int64        `json:"planned_count"`
	QueuedCount         int64        `json:"queued_count"`
	DispatchingCount    int64        `json:"dispatching_count"`
	SucceededCount      int64        `json:"succeeded_count"`
	FailedCount         int64        `json:"failed_count"`
	BlockedCount        int64        `json:"blocked_count"`
	CancelledCount      int64        `json:"cancelled_count"`
	IncludeDoNotDisturb bool         `json:"include_do_not_disturb"`
	TargetSource        string       `json:"target_source"`
	TargetSourceID      *int64       `json:"target_source_id"`
	LastStatusSyncAt    *time.Time   `json:"last_status_sync_at"`
	CreatedAt           time.Time    `json:"created_at"`
	LastRefreshedAt     *time.Time   `json:"last_refreshed_at"`
	PrivateDigest       OpaqueDigest `json:"-"`
}

type senderConfigSource struct {
	ID           int64     `json:"id"`
	SenderUserID string    `json:"sender_userid"`
	DisplayName  string    `json:"display_name"`
	Priority     int64     `json:"priority"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type sendRecordSource struct {
	ID                   int64           `json:"id"`
	RecordKey            string          `json:"record_key"`
	TaskType             string          `json:"task_type"`
	OutboundTaskIDs      json.RawMessage `json:"outbound_task_ids_json"`
	TaskResults          json.RawMessage `json:"task_results_json"`
	SelectedCount        int64           `json:"selected_count"`
	EligibleCount        int64           `json:"eligible_count"`
	SentCount            int64           `json:"sent_count"`
	SkippedCount         int64           `json:"skipped_count"`
	SkippedReasons       json.RawMessage `json:"skipped_reasons_json"`
	IncludeDoNotDisturb  bool            `json:"include_do_not_disturb"`
	ContentPreview       string          `json:"content_preview"`
	ImageCount           int64           `json:"image_count"`
	SenderUserIDs        json.RawMessage `json:"sender_userids_json"`
	FilterSnapshot       json.RawMessage `json:"filter_snapshot_json"`
	Operator             string          `json:"operator"`
	Status               string          `json:"status"`
	StatusLabel          string          `json:"status_label"`
	LastStatusSyncAt     *time.Time      `json:"last_status_sync_at"`
	CreatedAt            time.Time       `json:"created_at"`
	TargetUnionIDs       json.RawMessage `json:"target_unionids_json"`
	IdempotencyKey       *string         `json:"idempotency_key"`
	ExecutionBackend     string          `json:"execution_backend"`
	ExternalEffectJobIDs json.RawMessage `json:"external_effect_job_ids_json"`
	ExternalEffectStatus json.RawMessage `json:"external_effect_status_summary_json"`
	PlannedCount         int64           `json:"planned_count"`
	QueuedCount          int64           `json:"queued_count"`
	DispatchingCount     int64           `json:"dispatching_count"`
	SucceededCount       int64           `json:"succeeded_count"`
	FailedCount          int64           `json:"failed_count"`
	BlockedCount         int64           `json:"blocked_count"`
	CancelledCount       int64           `json:"cancelled_count"`
	LastRefreshedAt      *time.Time      `json:"last_refreshed_at"`
	TargetSource         string          `json:"target_source"`
	TargetSourceID       *int64          `json:"target_source_id"`
}

var senderConfigFields = []string{
	"id", "sender_userid", "display_name", "priority", "is_active", "created_at", "updated_at",
}

var senderConfigPrivateFields = []string{"sender_userid", "display_name"}

var sendRecordFields = []string{
	"id", "record_key", "task_type", "outbound_task_ids_json", "task_results_json", "selected_count", "eligible_count", "sent_count", "skipped_count",
	"skipped_reasons_json", "include_do_not_disturb", "content_preview", "image_count", "sender_userids_json", "filter_snapshot_json", "operator", "status", "status_label",
	"last_status_sync_at", "created_at", "target_unionids_json", "idempotency_key", "execution_backend", "external_effect_job_ids_json", "external_effect_status_summary_json",
	"planned_count", "queued_count", "dispatching_count", "succeeded_count", "failed_count", "blocked_count", "cancelled_count", "last_refreshed_at", "target_source", "target_source_id",
}

var sendRecordNullableFields = map[string]bool{
	"last_status_sync_at": true,
	"idempotency_key":     true,
	"last_refreshed_at":   true,
	"target_source_id":    true,
}

var sendRecordPrivateFields = []string{
	"record_key", "outbound_task_ids_json", "task_results_json", "skipped_reasons_json", "content_preview", "sender_userids_json", "filter_snapshot_json", "operator",
	"status_label", "target_unionids_json", "idempotency_key", "execution_backend", "external_effect_job_ids_json", "external_effect_status_summary_json",
}

// AdaptSenderConfig preserves the seven source fields as a non-authorizing
// historical fact. Archive row provenance is verified by the caller.
func AdaptSenderConfig(payload []byte, sourceHMACKey []byte) (SenderConfigFact, error) {
	fields, err := exactFields(payload, senderConfigFields, nil)
	if err != nil {
		return SenderConfigFact{}, err
	}
	var source senderConfigSource
	if json.Unmarshal(payload, &source) != nil {
		return SenderConfigFact{}, ErrInvalidSource
	}
	private, err := privateDigest(sourceHMACKey, SenderConfigTableID, fields, senderConfigPrivateFields)
	if err != nil {
		return SenderConfigFact{}, err
	}
	return SenderConfigFact{
		SourceID: source.ID, Priority: source.Priority, OriginalIsActive: source.IsActive,
		CreatedAt: utcMicro(source.CreatedAt), UpdatedAt: utcMicro(source.UpdatedAt), PrivateDigest: private,
	}, nil
}

// AdaptSendRecord preserves one 35-field aggregate source record without
// reconstructing its task, effect, sender, customer, or content relations.
// Archive row provenance is verified by the caller.
func AdaptSendRecord(payload []byte, sourceHMACKey []byte) (SendRecordFact, error) {
	fields, err := exactFields(payload, sendRecordFields, sendRecordNullableFields)
	if err != nil {
		return SendRecordFact{}, err
	}
	var source sendRecordSource
	if json.Unmarshal(payload, &source) != nil {
		return SendRecordFact{}, ErrInvalidSource
	}
	private, err := privateDigest(sourceHMACKey, SendRecordsTableID, fields, sendRecordPrivateFields)
	if err != nil {
		return SendRecordFact{}, err
	}
	return SendRecordFact{
		SourceID: source.ID, TaskType: source.TaskType, OriginalStatus: source.Status,
		SelectedCount: source.SelectedCount, EligibleCount: source.EligibleCount, SentCount: source.SentCount, SkippedCount: source.SkippedCount,
		ImageCount: source.ImageCount, PlannedCount: source.PlannedCount, QueuedCount: source.QueuedCount, DispatchingCount: source.DispatchingCount,
		SucceededCount: source.SucceededCount, FailedCount: source.FailedCount, BlockedCount: source.BlockedCount, CancelledCount: source.CancelledCount,
		IncludeDoNotDisturb: source.IncludeDoNotDisturb, TargetSource: source.TargetSource, TargetSourceID: cloneInt64(source.TargetSourceID),
		LastStatusSyncAt: utcMicroPtr(source.LastStatusSyncAt), CreatedAt: utcMicro(source.CreatedAt), LastRefreshedAt: utcMicroPtr(source.LastRefreshedAt),
		PrivateDigest: private,
	}, nil
}

func exactFields(payload []byte, names []string, nullable map[string]bool) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	fields := map[string]json.RawMessage{}
	if decoder.Decode(&fields) != nil || fields == nil {
		return nil, ErrInvalidSource
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || len(fields) != len(names) {
		return nil, ErrInvalidSource
	}
	for _, name := range names {
		value, found := fields[name]
		if !found || !json.Valid(value) || (!nullable[name] && isNull(value)) {
			return nil, ErrInvalidSource
		}
	}
	return fields, nil
}

type privateField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

func privateDigest(key []byte, table string, fields map[string]json.RawMessage, names []string) (OpaqueDigest, error) {
	if len(key) < sha256.Size || table == "" || len(names) == 0 {
		return OpaqueDigest{}, ErrInvalidSource
	}
	tuple := make([]privateField, 0, len(names))
	for _, name := range names {
		raw, found := fields[name]
		if !found {
			return OpaqueDigest{}, ErrInvalidSource
		}
		value, err := canonicalPrivateValue(raw)
		if err != nil {
			return OpaqueDigest{}, ErrInvalidSource
		}
		tuple = append(tuple, privateField{Name: name, Value: value})
	}
	encoded, err := json.Marshal(tuple)
	if err != nil {
		return OpaqueDigest{}, ErrInvalidSource
	}
	mac := hmac.New(sha256.New, key)
	for _, part := range [][]byte{[]byte("aicrm/v1-hxc-runtime-history/private/v1"), []byte(table), encoded} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(part)
	}
	var digest OpaqueDigest
	copy(digest[:], mac.Sum(nil))
	return digest, nil
}

func canonicalPrivateValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidSource
	}
	return value, nil
}

func utcMicro(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func utcMicroPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := utcMicro(*value)
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func isNull(value json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }
