// Package v1wecomcontacthistory classifies V1 WeCom external-contact rows as
// inert historical candidates. It has no target store, customer resolver,
// Provider, queue, or execution dependency.
package v1wecomcontacthistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	ExternalContactEventLogsTableID   = "public/wecom_external_contact_event_logs"
	ExternalContactFollowUsersTableID = "public/wecom_external_contact_follow_users"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest is a private comparison value, not a public identifier. Short
// source values may be enumerable; do not expose these unkeyed field digests.
// Source text, XML, and JSON remain only in the authenticated archive.
type OpaqueDigest [sha256.Size]byte

// SourceEnvelope binds a fact to its immutable V1 archive record. It is not a
// V2 identity or a command key.
type SourceEnvelope struct {
	SourceKeyDigest OpaqueDigest
	PayloadDigest   OpaqueDigest
	FieldDigest     OpaqueDigest
}

// ExternalContactEventLogFact is an inert V1 callback-processing record.
// EventTime is deliberately an integer because this package has no evidence
// for its unit or epoch. All identity, payload, and error values are digests.
type ExternalContactEventLogFact struct {
	SourceID                       int64
	Source                         SourceEnvelope
	CorpIDDigest                   OpaqueDigest
	EventType                      string
	ChangeType                     string
	ExternalUserIDDigest           OpaqueDigest
	UserIDDigest                   OpaqueDigest
	EventTime                      *int64
	EventKeyDigest                 OpaqueDigest
	PayloadXMLDigest               OpaqueDigest
	PayloadJSONDigest              OpaqueDigest
	ProcessStatus                  string
	RetryCount                     int32
	ErrorMessageDigest             OpaqueDigest
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
	IdentitySyncStatus             string
	IdentitySyncErrorCodeDigest    OpaqueDigest
	IdentitySyncErrorMessageDigest OpaqueDigest
	IdentitySyncResponseDigest     OpaqueDigest
}

// ExternalContactFollowUserFact preserves one source relation row. UserID,
// OperUserID, and IsPrimary are source facts only: they never select a V2
// owner and multiple rows for one external contact are intentionally kept.
type ExternalContactFollowUserFact struct {
	SourceID             int64
	Source               SourceEnvelope
	CorpIDDigest         OpaqueDigest
	ExternalUserIDDigest OpaqueDigest
	UserIDDigest         OpaqueDigest
	RelationStatus       string
	IsPrimary            bool
	RemarkDigest         OpaqueDigest
	DescriptionDigest    OpaqueDigest
	AddWay               *int32
	State                string
	OperUserIDDigest     OpaqueDigest
	CreateTime           *int64
	RawFollowUserDigest  OpaqueDigest
	FirstSeenAt          time.Time
	LastSeenAt           time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Result[T any] struct {
	SourceID    int64
	Disposition Disposition
	Reason      string
	Fact        *T
}

// History keeps the two source tables separate. It is only a candidate view;
// neither slice can write current customer, owner, or Provider state.
type History struct {
	EventLogs   []Result[ExternalContactEventLogFact]
	FollowUsers []Result[ExternalContactFollowUserFact]
}

func (h History) SourceCount() int { return len(h.EventLogs) + len(h.FollowUsers) }

func (h History) TerminalCount() int {
	return terminalCount(h.EventLogs) + terminalCount(h.FollowUsers)
}

func terminalCount[T any](rows []Result[T]) int {
	count := 0
	for _, row := range rows {
		if row.Disposition == DispositionCandidate || row.Disposition == DispositionQuarantine {
			count++
		}
	}
	return count
}

type eventLogJSON struct {
	ID                       int64           `json:"id"`
	CorpID                   string          `json:"corp_id"`
	EventType                string          `json:"event_type"`
	ChangeType               string          `json:"change_type"`
	ExternalUserID           string          `json:"external_userid"`
	UserID                   string          `json:"user_id"`
	EventTime                *int64          `json:"event_time"`
	EventKey                 string          `json:"event_key"`
	PayloadXML               string          `json:"payload_xml"`
	PayloadJSON              json.RawMessage `json:"payload_json"`
	ProcessStatus            string          `json:"process_status"`
	RetryCount               int32           `json:"retry_count"`
	ErrorMessage             string          `json:"error_message"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
	IdentitySyncStatus       string          `json:"identity_sync_status"`
	IdentitySyncErrorCode    string          `json:"identity_sync_error_code"`
	IdentitySyncErrorMessage string          `json:"identity_sync_error_message"`
	IdentitySyncResponseJSON json.RawMessage `json:"identity_sync_response_json"`
}

type followUserJSON struct {
	ID             int64           `json:"id"`
	CorpID         string          `json:"corp_id"`
	ExternalUserID string          `json:"external_userid"`
	UserID         string          `json:"user_id"`
	RelationStatus string          `json:"relation_status"`
	IsPrimary      bool            `json:"is_primary"`
	Remark         string          `json:"remark"`
	Description    string          `json:"description"`
	AddWay         *int32          `json:"add_way"`
	State          string          `json:"state"`
	OperUserID     string          `json:"oper_userid"`
	CreateTime     *int64          `json:"createtime"`
	RawFollowUser  json.RawMessage `json:"raw_follow_user"`
	FirstSeenAt    time.Time       `json:"first_seen_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

var eventLogFields = []string{
	"id", "corp_id", "event_type", "change_type", "external_userid", "user_id", "event_time", "event_key", "payload_xml", "payload_json", "process_status", "retry_count", "error_message", "created_at", "updated_at", "identity_sync_status", "identity_sync_error_code", "identity_sync_error_message", "identity_sync_response_json",
}

var followUserFields = []string{
	"id", "corp_id", "external_userid", "user_id", "relation_status", "is_primary", "remark", "description", "add_way", "state", "oper_userid", "createtime", "raw_follow_user", "first_seen_at", "last_seen_at", "created_at", "updated_at",
}

// AdaptHistory accepts only authenticated archive rows. The archive ordinal is
// intentionally the input order, while source IDs remain signed V1 values.
func AdaptHistory(eventLogs, followUsers []v1archive.ArchivedRow) History {
	history := History{
		EventLogs:   make([]Result[ExternalContactEventLogFact], len(eventLogs)),
		FollowUsers: make([]Result[ExternalContactFollowUserFact], len(followUsers)),
	}
	for i, row := range eventLogs {
		history.EventLogs[i] = adaptEventLog(row, int64(i+1))
	}
	for i, row := range followUsers {
		history.FollowUsers[i] = adaptFollowUser(row, int64(i+1))
	}
	quarantineDuplicateIDs(history.EventLogs, "wecom_external_contact_event_logs_source_ambiguous")
	quarantineDuplicateIDs(history.FollowUsers, "wecom_external_contact_follow_users_source_ambiguous")
	return history
}

func adaptEventLog(row v1archive.ArchivedRow, ordinal int64) Result[ExternalContactEventLogFact] {
	fields, source, reason := archiveFields(row, ExternalContactEventLogsTableID, ordinal)
	if reason != "" {
		return quarantine[ExternalContactEventLogFact](0, reason)
	}
	var value eventLogJSON
	if !decodeExact(fields, row.Payload, &value, eventLogFields, []string{"event_time", "payload_json", "identity_sync_response_json"}) {
		return quarantine[ExternalContactEventLogFact](sourceID(fields), "wecom_external_contact_event_logs_shape_invalid")
	}
	return candidate(ExternalContactEventLogFact{
		SourceID: value.ID, Source: source,
		CorpIDDigest: fieldDigest(ExternalContactEventLogsTableID, "corp_id", fields["corp_id"]),
		EventType:    value.EventType, ChangeType: value.ChangeType,
		ExternalUserIDDigest: fieldDigest(ExternalContactEventLogsTableID, "external_userid", fields["external_userid"]),
		UserIDDigest:         fieldDigest(ExternalContactEventLogsTableID, "user_id", fields["user_id"]),
		EventTime:            cloneInt64(value.EventTime),
		EventKeyDigest:       fieldDigest(ExternalContactEventLogsTableID, "event_key", fields["event_key"]),
		PayloadXMLDigest:     fieldDigest(ExternalContactEventLogsTableID, "payload_xml", fields["payload_xml"]),
		PayloadJSONDigest:    fieldDigest(ExternalContactEventLogsTableID, "payload_json", fields["payload_json"]),
		ProcessStatus:        value.ProcessStatus, RetryCount: value.RetryCount,
		ErrorMessageDigest: fieldDigest(ExternalContactEventLogsTableID, "error_message", fields["error_message"]),
		CreatedAt:          value.CreatedAt, UpdatedAt: value.UpdatedAt, IdentitySyncStatus: value.IdentitySyncStatus,
		IdentitySyncErrorCodeDigest:    fieldDigest(ExternalContactEventLogsTableID, "identity_sync_error_code", fields["identity_sync_error_code"]),
		IdentitySyncErrorMessageDigest: fieldDigest(ExternalContactEventLogsTableID, "identity_sync_error_message", fields["identity_sync_error_message"]),
		IdentitySyncResponseDigest:     fieldDigest(ExternalContactEventLogsTableID, "identity_sync_response_json", fields["identity_sync_response_json"]),
	})
}

func adaptFollowUser(row v1archive.ArchivedRow, ordinal int64) Result[ExternalContactFollowUserFact] {
	fields, source, reason := archiveFields(row, ExternalContactFollowUsersTableID, ordinal)
	if reason != "" {
		return quarantine[ExternalContactFollowUserFact](0, reason)
	}
	var value followUserJSON
	if !decodeExact(fields, row.Payload, &value, followUserFields, []string{"add_way", "createtime", "raw_follow_user"}) {
		return quarantine[ExternalContactFollowUserFact](sourceID(fields), "wecom_external_contact_follow_users_shape_invalid")
	}
	return candidate(ExternalContactFollowUserFact{
		SourceID: value.ID, Source: source,
		CorpIDDigest:         fieldDigest(ExternalContactFollowUsersTableID, "corp_id", fields["corp_id"]),
		ExternalUserIDDigest: fieldDigest(ExternalContactFollowUsersTableID, "external_userid", fields["external_userid"]),
		UserIDDigest:         fieldDigest(ExternalContactFollowUsersTableID, "user_id", fields["user_id"]),
		RelationStatus:       value.RelationStatus, IsPrimary: value.IsPrimary,
		RemarkDigest:      fieldDigest(ExternalContactFollowUsersTableID, "remark", fields["remark"]),
		DescriptionDigest: fieldDigest(ExternalContactFollowUsersTableID, "description", fields["description"]),
		AddWay:            cloneInt32(value.AddWay), State: value.State,
		OperUserIDDigest:    fieldDigest(ExternalContactFollowUsersTableID, "oper_userid", fields["oper_userid"]),
		CreateTime:          cloneInt64(value.CreateTime),
		RawFollowUserDigest: fieldDigest(ExternalContactFollowUsersTableID, "raw_follow_user", fields["raw_follow_user"]),
		FirstSeenAt:         value.FirstSeenAt, LastSeenAt: value.LastSeenAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	})
}

func archiveFields(row v1archive.ArchivedRow, table string, ordinal int64) (fields, SourceEnvelope, string) {
	zero := [sha256.Size]byte{}
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, table + "_archive_invalid"
	}
	if len(row.RedactedFields) != 0 {
		return nil, SourceEnvelope{}, table + "_source_redacted"
	}
	parsed, ok := object(row.Payload)
	if !ok {
		return nil, SourceEnvelope{}, table + "_shape_invalid"
	}
	return parsed, SourceEnvelope{SourceKeyDigest: OpaqueDigest(row.SourceKeyHMAC), PayloadDigest: OpaqueDigest(row.PayloadHMAC), FieldDigest: OpaqueDigest(row.FieldHMAC)}, ""
}

func decodeExact(source fields, payload []byte, target any, names, nullable []string) bool {
	if len(source) != len(names) {
		return false
	}
	nullableSet := make(map[string]bool, len(nullable))
	for _, name := range nullable {
		nullableSet[name] = true
	}
	for _, name := range names {
		raw, found := source[name]
		if !found || (!nullableSet[name] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

type fields map[string]json.RawMessage

func object(value []byte) (fields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	result := make(fields)
	if decoder.Decode(&result) != nil || result == nil {
		return nil, false
	}
	var extra any
	return result, errors.Is(decoder.Decode(&extra), io.EOF)
}

func sourceID(source fields) int64 {
	raw, found := source["id"]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0
	}
	var id int64
	if json.Unmarshal(raw, &id) != nil {
		return 0
	}
	return id
}

func fieldDigest(table, field string, value []byte) OpaqueDigest {
	data := append([]byte("v1-wecom-contact-history-field-v1\x00"+table+"\x00"+field+"\x00"), value...)
	return OpaqueDigest(sha256.Sum256(data))
}

func candidate[T any](fact T) Result[T] {
	return Result[T]{SourceID: factSourceID(fact), Disposition: DispositionCandidate, Fact: &fact}
}

func quarantine[T any](sourceID int64, reason string) Result[T] {
	return Result[T]{SourceID: sourceID, Disposition: DispositionQuarantine, Reason: reason}
}

func factSourceID(value any) int64 {
	switch value := value.(type) {
	case ExternalContactEventLogFact:
		return value.SourceID
	case ExternalContactFollowUserFact:
		return value.SourceID
	default:
		return 0
	}
}

func quarantineDuplicateIDs[T any](values []Result[T], reason string) {
	counts := make(map[int64]int, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			counts[value.SourceID]++
		}
	}
	for i := range values {
		if values[i].Disposition == DispositionCandidate && values[i].Fact != nil && counts[values[i].SourceID] > 1 {
			values[i] = quarantine[T](values[i].SourceID, reason)
		}
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
