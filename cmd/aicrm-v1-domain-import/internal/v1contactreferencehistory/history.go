// Package v1contactreferencehistory adapts authenticated V1 contact-reference
// rows into private, inert facts. It does not resolve people, create or update
// Staff or Customers, or make any identity assurance claim.
package v1contactreferencehistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	ExternalContactBindingsTableID    = "public/external_contact_bindings"
	AdminWeComDirectoryMembersTableID = "public/admin_wecom_directory_members"
)

var (
	ErrArchiveRow            = errors.New("contact reference archive row invalid")
	ErrFact                  = errors.New("contact reference fact invalid")
	ErrRequiredFieldRedacted = errors.New("contact reference required source field redacted")
)

// SourceEnvelope identifies one immutable archive row. Values are HMACs, not
// V1 identifiers and not target foreign keys.
type SourceEnvelope struct {
	SourceKeyDigest [sha256.Size]byte
	PayloadDigest   [sha256.Size]byte
	FieldDigest     [sha256.Size]byte
}

// ExternalContactBindingFact is private source evidence. Person and owner
// values deliberately remain source facts; this adapter never resolves them.
type ExternalContactBindingFact struct {
	Source             SourceEnvelope `json:"-"`
	ExternalUserID     string         `json:"-"`
	PersonID           int64          `json:"-"`
	FirstBoundByUserID string         `json:"-"`
	FirstOwnerUserID   string         `json:"-"`
	LastOwnerUserID    string         `json:"-"`
	CreatedAt          time.Time      `json:"-"`
	UpdatedAt          time.Time      `json:"-"`
}

// DirectoryMemberFact preserves the complete frozen directory row. It does
// not create, activate, or otherwise alter a V2 staff account.
type DirectoryMemberFact struct {
	Source            SourceEnvelope `json:"-"`
	ID                int64          `json:"-"`
	WeComCorpID       string         `json:"-"`
	WeComUserID       string         `json:"-"`
	DisplayName       string         `json:"-"`
	DepartmentIDsJSON string         `json:"-"`
	Position          string         `json:"-"`
	WeComStatus       *int32         `json:"-"`
	IsActive          bool           `json:"-"`
	SyncedAt          time.Time      `json:"-"`
	RawPayloadJSON    string         `json:"-"`
	CreatedAt         time.Time      `json:"-"`
	UpdatedAt         time.Time      `json:"-"`
	CorpID            string         `json:"-"`
	DepartmentName    string         `json:"-"`
	Mobile            string         `json:"-"`
	AvatarURL         string         `json:"-"`
	FirstSeenAt       time.Time      `json:"-"`
	LastSyncedAt      time.Time      `json:"-"`
	UpdatedBy         string         `json:"-"`
}

type externalContactBindingJSON struct {
	ExternalUserID     string    `json:"external_userid"`
	PersonID           int64     `json:"person_id"`
	FirstBoundByUserID string    `json:"first_bound_by_userid"`
	FirstOwnerUserID   string    `json:"first_owner_userid"`
	LastOwnerUserID    string    `json:"last_owner_userid"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type directoryMemberJSON struct {
	ID                int64     `json:"id"`
	WeComCorpID       string    `json:"wecom_corpid"`
	WeComUserID       string    `json:"wecom_userid"`
	DisplayName       string    `json:"display_name"`
	DepartmentIDsJSON string    `json:"department_ids_json"`
	Position          string    `json:"position"`
	WeComStatus       *int32    `json:"wecom_status"`
	IsActive          bool      `json:"is_active"`
	SyncedAt          time.Time `json:"synced_at"`
	RawPayloadJSON    string    `json:"raw_payload_json"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CorpID            string    `json:"corp_id"`
	DepartmentName    string    `json:"department_name"`
	Mobile            string    `json:"mobile"`
	AvatarURL         string    `json:"avatar_url"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSyncedAt      time.Time `json:"last_synced_at"`
	UpdatedBy         string    `json:"updated_by"`
}

var externalContactBindingFields = []string{
	"external_userid", "person_id", "first_bound_by_userid", "first_owner_userid", "last_owner_userid", "created_at", "updated_at",
}

var directoryMemberFields = []string{
	"id", "wecom_corpid", "wecom_userid", "display_name", "department_ids_json", "position", "wecom_status", "is_active", "synced_at", "raw_payload_json", "created_at", "updated_at", "corp_id", "department_name", "mobile", "avatar_url", "first_seen_at", "last_synced_at", "updated_by",
}

// AdaptExternalContactBinding verifies archive integrity and preserves the
// seven manifest fields exactly (apart from UTC microsecond normalization).
func AdaptExternalContactBinding(row v1archive.ArchivedRow, archiveHMACKey []byte) (ExternalContactBindingFact, error) {
	fields, source, err := archiveFields(row, ExternalContactBindingsTableID, archiveHMACKey, func(value map[string]json.RawMessage) ([]byte, error) {
		return json.Marshal([]json.RawMessage{value["external_userid"]})
	})
	if err != nil {
		return ExternalContactBindingFact{}, err
	}
	var value externalContactBindingJSON
	if err := decodeExact(fields, row.Payload, &value, externalContactBindingFields, nil); err != nil ||
		!safeText(value.ExternalUserID, value.FirstBoundByUserID, value.FirstOwnerUserID, value.LastOwnerUserID) || !validTimes(value.CreatedAt, value.UpdatedAt) {
		return ExternalContactBindingFact{}, ErrFact
	}
	return ExternalContactBindingFact{
		Source: source, ExternalUserID: value.ExternalUserID, PersonID: value.PersonID,
		FirstBoundByUserID: value.FirstBoundByUserID, FirstOwnerUserID: value.FirstOwnerUserID, LastOwnerUserID: value.LastOwnerUserID,
		CreatedAt: normalizeTime(value.CreatedAt), UpdatedAt: normalizeTime(value.UpdatedAt),
	}, nil
}

// AdaptDirectoryMember verifies archive integrity and preserves the nineteen
// manifest fields. JSON-named source columns are TEXT and are intentionally
// not parsed or re-serialized.
func AdaptDirectoryMember(row v1archive.ArchivedRow, archiveHMACKey []byte) (DirectoryMemberFact, error) {
	fields, source, err := archiveFields(row, AdminWeComDirectoryMembersTableID, archiveHMACKey, func(value map[string]json.RawMessage) ([]byte, error) {
		return json.Marshal([]json.RawMessage{value["id"]})
	})
	if err != nil {
		return DirectoryMemberFact{}, err
	}
	var value directoryMemberJSON
	if err := decodeExact(fields, row.Payload, &value, directoryMemberFields, map[string]bool{"wecom_status": true}); err != nil ||
		!safeText(value.WeComCorpID, value.WeComUserID, value.DisplayName, value.DepartmentIDsJSON, value.Position, value.RawPayloadJSON, value.CorpID, value.DepartmentName, value.Mobile, value.AvatarURL, value.UpdatedBy) ||
		!validTimes(value.SyncedAt, value.CreatedAt, value.UpdatedAt, value.FirstSeenAt, value.LastSyncedAt) {
		return DirectoryMemberFact{}, ErrFact
	}
	return DirectoryMemberFact{
		Source: source, ID: value.ID, WeComCorpID: value.WeComCorpID, WeComUserID: value.WeComUserID, DisplayName: value.DisplayName,
		DepartmentIDsJSON: value.DepartmentIDsJSON, Position: value.Position, WeComStatus: cloneInt32(value.WeComStatus), IsActive: value.IsActive,
		SyncedAt: normalizeTime(value.SyncedAt), RawPayloadJSON: value.RawPayloadJSON, CreatedAt: normalizeTime(value.CreatedAt), UpdatedAt: normalizeTime(value.UpdatedAt),
		CorpID: value.CorpID, DepartmentName: value.DepartmentName, Mobile: value.Mobile, AvatarURL: value.AvatarURL,
		FirstSeenAt: normalizeTime(value.FirstSeenAt), LastSyncedAt: normalizeTime(value.LastSyncedAt), UpdatedBy: value.UpdatedBy,
	}, nil
}

func archiveFields(row v1archive.ArchivedRow, tableID string, key []byte, sourceKey func(map[string]json.RawMessage) ([]byte, error)) (map[string]json.RawMessage, SourceEnvelope, error) {
	zero := [sha256.Size]byte{}
	if len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	name := strings.TrimPrefix(tableID, "public/")
	payload, err := v1archive.PayloadHMAC(key, name, row.Payload)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, name, row.RedactedFields)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	if len(roots) != 0 {
		return nil, SourceEnvelope{}, ErrRequiredFieldRedacted
	}
	fields := make(map[string]json.RawMessage)
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, SourceEnvelope{}, ErrFact
	}
	keyJSON, err := sourceKey(fields)
	if err != nil {
		return nil, SourceEnvelope{}, ErrFact
	}
	source, err := v1archive.SourceKeyHMAC(key, name, keyJSON)
	if err != nil || !hmac.Equal(source[:], row.SourceKeyHMAC[:]) {
		return nil, SourceEnvelope{}, ErrArchiveRow
	}
	return fields, SourceEnvelope{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC}, nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any, names []string, nullable map[string]bool) error {
	if len(fields) != len(names) {
		return ErrFact
	}
	for _, name := range names {
		raw, found := fields[name]
		if !found || !json.Valid(raw) || (!nullable[name] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
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

func safeText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}

func validTimes(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() {
			return false
		}
	}
	return true
}

func normalizeTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

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
