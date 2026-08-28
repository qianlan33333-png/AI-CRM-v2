// Package v1customerstatehistory classifies immutable V1 customer-state rows.
// It does not resolve identities, write a V2 customer state, tag, or sync
// record, or expose the source values through an API.
package v1customerstatehistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	UserStatusCurrentTableID = "public/class_user_status_current"
	UserStatusHistoryTableID = "public/class_user_status_history"
	TermTagMappingTableID    = "public/class_term_tag_mapping"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

const (
	ReasonInvalidEnvelope = "customer_state_history_envelope_invalid"
	ReasonRedactedField   = "customer_state_history_retained_field_redacted"
	ReasonInvalidPayload  = "customer_state_history_payload_invalid"
	ReasonDuplicateSource = "customer_state_history_source_key_duplicate"
)

// SourceEnvelope binds a candidate to immutable archive material. It never
// uses a V1 numeric ID as a target identity.
type SourceEnvelope struct {
	SourceKeyDigest     [sha256.Size]byte
	SourcePayloadDigest [sha256.Size]byte
	SourceFieldDigest   [sha256.Size]byte
}

// UserStatusCurrent is a V1 snapshot, not a current V2 marketing stage or
// tag. UnionID stays private for a later DM01 resolver. Actor and sync error
// are represented only by stable digests.
type UserStatusCurrent struct {
	SignupStatus          string
	SignupLabelName       string
	CustomerNameSnapshot  string `json:"-"`
	OwnerUserIDSnapshot   string `json:"-"`
	SetByUserIDDigest     [sha256.Size]byte
	SetAt                 time.Time
	WeComTagSyncStatus    string
	WeComTagSyncErrorHash [sha256.Size]byte
	StatusFlagsDigest     [sha256.Size]byte
	CreatedAt             time.Time
	UpdatedAt             time.Time
	UnionID               string `json:"-"`
	Envelope              SourceEnvelope
}

// UserStatusHistory preserves a V1 change observation. SourceID is only the
// original historical ID and may be zero or negative because the frozen source
// schema does not authorize rewriting it.
type UserStatusHistory struct {
	SourceID              int64
	OldSignupStatus       string
	NewSignupStatus       string
	OldLabelName          string
	NewLabelName          string
	CustomerNameSnapshot  string `json:"-"`
	OwnerUserIDSnapshot   string `json:"-"`
	SetByUserIDDigest     [sha256.Size]byte
	SetAt                 time.Time
	WeComTagSyncStatus    string
	WeComTagSyncErrorHash [sha256.Size]byte
	StatusFlagsDigest     [sha256.Size]byte
	CreatedAt             time.Time
	UnionID               string `json:"-"`
	Envelope              SourceEnvelope
}

// TermTagMapping keeps old tag/group/strategy references as source facts. No
// V2 tag or strategy ID is inferred from these strings.
type TermTagMapping struct {
	SourceID       int64
	TagGroupName   string
	TagName        string
	ClassTermNo    int32
	ClassTermLabel string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StrategyID     string `json:"-"`
	GroupID        string `json:"-"`
	TagID          string `json:"-"`
	Envelope       SourceEnvelope
}

type Result[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

type currentJSON struct {
	SignupStatus    string          `json:"signup_status"`
	SignupLabelName string          `json:"signup_label_name"`
	CustomerName    string          `json:"customer_name_snapshot"`
	OwnerUserID     string          `json:"owner_userid_snapshot"`
	SetByUserID     string          `json:"set_by_userid"`
	SetAt           time.Time       `json:"set_at"`
	SyncStatus      string          `json:"wecom_tag_sync_status"`
	SyncError       string          `json:"wecom_tag_sync_error"`
	StatusFlags     json.RawMessage `json:"status_flags_json"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	UnionID         string          `json:"unionid"`
}

type historyJSON struct {
	ID              int64           `json:"id"`
	OldSignupStatus string          `json:"old_signup_status"`
	NewSignupStatus string          `json:"new_signup_status"`
	OldLabelName    string          `json:"old_label_name"`
	NewLabelName    string          `json:"new_label_name"`
	CustomerName    string          `json:"customer_name_snapshot"`
	OwnerUserID     string          `json:"owner_userid_snapshot"`
	SetByUserID     string          `json:"set_by_userid"`
	SetAt           time.Time       `json:"set_at"`
	SyncStatus      string          `json:"wecom_tag_sync_status"`
	SyncError       string          `json:"wecom_tag_sync_error"`
	StatusFlags     json.RawMessage `json:"status_flags_json"`
	CreatedAt       time.Time       `json:"created_at"`
	UnionID         string          `json:"unionid"`
}

type termTagJSON struct {
	ID             int64     `json:"id"`
	TagGroupName   string    `json:"tag_group_name"`
	TagName        string    `json:"tag_name"`
	ClassTermNo    int32     `json:"class_term_no"`
	ClassTermLabel string    `json:"class_term_label"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	StrategyID     string    `json:"strategy_id"`
	GroupID        string    `json:"group_id"`
	TagID          string    `json:"tag_id"`
}

func AdaptUserStatusCurrent(row v1archive.ArchivedRow) Result[UserStatusCurrent] {
	const fields = "signup_status signup_label_name customer_name_snapshot owner_userid_snapshot set_by_userid set_at wecom_tag_sync_status wecom_tag_sync_error status_flags_json created_at updated_at unionid"
	if !validEnvelope(row, UserStatusCurrentTableID) {
		return quarantine[UserStatusCurrent](ReasonInvalidEnvelope)
	}
	if requiredRedacted(row.RedactedFields, fields) {
		return quarantine[UserStatusCurrent](ReasonRedactedField)
	}
	var source currentJSON
	if !decode(row.Payload, &source, fields, "status_flags_json") || source.SetAt.IsZero() || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || !json.Valid(source.StatusFlags) {
		return quarantine[UserStatusCurrent](ReasonInvalidPayload)
	}
	value := UserStatusCurrent{
		SignupStatus: source.SignupStatus, SignupLabelName: source.SignupLabelName,
		CustomerNameSnapshot: source.CustomerName, OwnerUserIDSnapshot: source.OwnerUserID,
		SetByUserIDDigest: sha256.Sum256([]byte(source.SetByUserID)), SetAt: canonicalTime(source.SetAt),
		WeComTagSyncStatus: source.SyncStatus, WeComTagSyncErrorHash: sha256.Sum256([]byte(source.SyncError)),
		StatusFlagsDigest: sha256.Sum256(source.StatusFlags), CreatedAt: canonicalTime(source.CreatedAt),
		UpdatedAt: canonicalTime(source.UpdatedAt), UnionID: source.UnionID, Envelope: envelope(row),
	}
	return candidate(value)
}

func AdaptUserStatusHistory(row v1archive.ArchivedRow) Result[UserStatusHistory] {
	const fields = "id old_signup_status new_signup_status old_label_name new_label_name customer_name_snapshot owner_userid_snapshot set_by_userid set_at wecom_tag_sync_status wecom_tag_sync_error status_flags_json created_at unionid"
	if !validEnvelope(row, UserStatusHistoryTableID) {
		return quarantine[UserStatusHistory](ReasonInvalidEnvelope)
	}
	if requiredRedacted(row.RedactedFields, fields) {
		return quarantine[UserStatusHistory](ReasonRedactedField)
	}
	var source historyJSON
	if !decode(row.Payload, &source, fields, "status_flags_json") || source.SetAt.IsZero() || source.CreatedAt.IsZero() || !json.Valid(source.StatusFlags) {
		return quarantine[UserStatusHistory](ReasonInvalidPayload)
	}
	value := UserStatusHistory{
		SourceID: source.ID, OldSignupStatus: source.OldSignupStatus, NewSignupStatus: source.NewSignupStatus,
		OldLabelName: source.OldLabelName, NewLabelName: source.NewLabelName,
		CustomerNameSnapshot: source.CustomerName, OwnerUserIDSnapshot: source.OwnerUserID,
		SetByUserIDDigest: sha256.Sum256([]byte(source.SetByUserID)), SetAt: canonicalTime(source.SetAt),
		WeComTagSyncStatus: source.SyncStatus, WeComTagSyncErrorHash: sha256.Sum256([]byte(source.SyncError)),
		StatusFlagsDigest: sha256.Sum256(source.StatusFlags), CreatedAt: canonicalTime(source.CreatedAt),
		UnionID: source.UnionID, Envelope: envelope(row),
	}
	return candidate(value)
}

func AdaptTermTagMapping(row v1archive.ArchivedRow) Result[TermTagMapping] {
	const fields = "id tag_group_name tag_name class_term_no class_term_label is_active created_at updated_at strategy_id group_id tag_id"
	if !validEnvelope(row, TermTagMappingTableID) {
		return quarantine[TermTagMapping](ReasonInvalidEnvelope)
	}
	if requiredRedacted(row.RedactedFields, fields) {
		return quarantine[TermTagMapping](ReasonRedactedField)
	}
	var source termTagJSON
	if !decode(row.Payload, &source, fields) || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return quarantine[TermTagMapping](ReasonInvalidPayload)
	}
	value := TermTagMapping{SourceID: source.ID, TagGroupName: source.TagGroupName, TagName: source.TagName,
		ClassTermNo: source.ClassTermNo, ClassTermLabel: source.ClassTermLabel, IsActive: source.IsActive,
		CreatedAt: canonicalTime(source.CreatedAt), UpdatedAt: canonicalTime(source.UpdatedAt),
		StrategyID: source.StrategyID, GroupID: source.GroupID, TagID: source.TagID, Envelope: envelope(row)}
	return candidate(value)
}

func AdaptUserStatusCurrentRows(rows []v1archive.ArchivedRow) []Result[UserStatusCurrent] {
	result := make([]Result[UserStatusCurrent], len(rows))
	for index, row := range rows {
		result[index] = AdaptUserStatusCurrent(row)
	}
	quarantineDuplicates(result, rows)
	return result
}

func AdaptUserStatusHistoryRows(rows []v1archive.ArchivedRow) []Result[UserStatusHistory] {
	result := make([]Result[UserStatusHistory], len(rows))
	for index, row := range rows {
		result[index] = AdaptUserStatusHistory(row)
	}
	quarantineDuplicates(result, rows)
	return result
}

func AdaptTermTagMappingRows(rows []v1archive.ArchivedRow) []Result[TermTagMapping] {
	result := make([]Result[TermTagMapping], len(rows))
	for index, row := range rows {
		result[index] = AdaptTermTagMapping(row)
	}
	quarantineDuplicates(result, rows)
	return result
}

func quarantineDuplicates[T any](results []Result[T], rows []v1archive.ArchivedRow) {
	seen := make(map[[sha256.Size]byte]int, len(rows))
	duplicate := make(map[int]struct{})
	for index, row := range rows {
		if results[index].Candidate == nil {
			continue
		}
		if prior, found := seen[row.SourceKeyHMAC]; found {
			duplicate[prior] = struct{}{}
			duplicate[index] = struct{}{}
			continue
		}
		seen[row.SourceKeyHMAC] = index
	}
	for index := range duplicate {
		results[index] = quarantine[T](ReasonDuplicateSource)
	}
}

func validEnvelope(row v1archive.ArchivedRow, table string) bool {
	zero := [sha256.Size]byte{}
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == table && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != zero && row.PayloadHMAC != zero && row.FieldHMAC != zero &&
		utf8.Valid(row.Payload) && json.Valid(row.Payload)
}

func requiredRedacted(paths []string, fields string) bool {
	allowed := strings.Fields(fields)
	for _, path := range paths {
		for _, field := range allowed {
			if path == field || strings.HasPrefix(path, field+".") || strings.HasPrefix(path, field+"[") {
				return true
			}
		}
		// An unrecognized redaction path is also fail-closed. A candidate must
		// not assume it is detached from a retained source field.
		return true
	}
	return false
}

func decode(payload []byte, destination any, fields string, jsonNulls ...string) bool {
	if !utf8.Valid(payload) {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil || object == nil {
		return false
	}
	allowJSONNull := make(map[string]struct{}, len(jsonNulls))
	for _, field := range jsonNulls {
		allowJSONNull[field] = struct{}{}
	}
	for _, field := range strings.Fields(fields) {
		value, found := object[field]
		_, allowsNull := allowJSONNull[field]
		if !found || len(value) == 0 || (bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !allowsNull) {
			return false
		}
	}
	return json.Unmarshal(payload, destination) == nil
}

func canonicalTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func envelope(row v1archive.ArchivedRow) SourceEnvelope {
	return SourceEnvelope{SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC}
}

func candidate[T any](value T) Result[T] {
	return Result[T]{Disposition: DispositionCandidate, Candidate: &value}
}

func quarantine[T any](reason string) Result[T] {
	return Result[T]{Disposition: DispositionQuarantine, Reason: reason}
}
