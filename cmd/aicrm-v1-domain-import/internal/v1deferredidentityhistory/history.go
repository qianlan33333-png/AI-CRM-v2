// Package v1deferredidentityhistory adapts authenticated V1 identity evidence
// into inert facts. It does not select rows, create Customers, bind identities,
// merge records, or write a target store.
package v1deferredidentityhistory

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	PeopleTableID                = "public/people"
	IdentityConflictsTableID     = "public/crm_user_identity_conflicts"
	ExternalContactIdentityMapID = "public/wecom_external_contact_identity_map"
)

var (
	ErrArchiveRow = errors.New("deferred identity archive row invalid")
	ErrFact       = errors.New("deferred identity fact invalid")
)

// OpaqueDigest is a keyed, non-recoverable comparison value. It is never a
// source identifier and cannot be used to attach a fact to a Customer.
type OpaqueDigest [sha256.Size]byte

// SourceEnvelope is immutable archive provenance. Its three values are
// preserved separately from the aggregate private digest.
type SourceEnvelope struct {
	SourceKeyDigest OpaqueDigest
	PayloadDigest   OpaqueDigest
	FieldDigest     OpaqueDigest
}

// PersonFact preserves an unbound legacy people row. Mobile and third-party
// values are unavailable except as keyed digests.
type PersonFact struct {
	SourceID               int64
	Source                 SourceEnvelope
	MobileDigest           OpaqueDigest
	ThirdPartyUserIDDigest OpaqueDigest
	CreatedAt              time.Time
	UpdatedAt              time.Time
	PrivateDigest          OpaqueDigest
	RedactedRoots          []string
}

// ConflictFact preserves a legacy identity conflict without exposing any
// identity candidate, JSON, source key, or resolution note.
type ConflictFact struct {
	SourceID               int64
	Source                 SourceEnvelope
	ConflictType           string
	SourceType             string
	Status                 string
	ResolutionStatus       string
	UnionIDDigest          OpaqueDigest
	CandidateUnionIDDigest OpaqueDigest
	ExternalUserIDDigest   OpaqueDigest
	OpenIDDigest           OpaqueDigest
	MobileDigest           OpaqueDigest
	SourceKeyDigest        OpaqueDigest
	PayloadJSONDigest      OpaqueDigest
	SourcePayloadDigest    OpaqueDigest
	ResolutionNoteDigest   OpaqueDigest
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ResolvedAt             *time.Time
	PrivateDigest          OpaqueDigest
	RedactedRoots          []string
}

// MissingRootIdentityFact is one unbound external-contact evidence row. It is
// intentionally not a DM01 selector result: callers must separately prove the
// fixed missing_customer_root selection before persisting or presenting it.
type MissingRootIdentityFact struct {
	SourceID             int64
	Source               SourceEnvelope
	Type                 *int32
	Status               string
	CorpIDDigest         OpaqueDigest
	ExternalUserIDDigest OpaqueDigest
	UnionIDDigest        OpaqueDigest
	OpenIDDigest         OpaqueDigest
	FollowUserIDDigest   OpaqueDigest
	NameDigest           OpaqueDigest
	AvatarDigest         OpaqueDigest
	GenderDigest         *OpaqueDigest
	RawProfileDigest     OpaqueDigest
	FirstSeenAt          time.Time
	LastSeenAt           time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	PrivateDigest        OpaqueDigest
	RedactedRoots        []string
}

type personJSON struct {
	ID               int64     `json:"id"`
	Mobile           string    `json:"mobile"`
	ThirdPartyUserID string    `json:"third_party_user_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type conflictJSON struct {
	ID                int64           `json:"id"`
	ConflictType      string          `json:"conflict_type"`
	UnionID           string          `json:"unionid"`
	CandidateUnionID  string          `json:"candidate_unionid"`
	ExternalUserID    string          `json:"external_userid"`
	OpenID            string          `json:"openid"`
	Mobile            string          `json:"mobile"`
	SourceType        string          `json:"source_type"`
	SourceKey         string          `json:"source_key"`
	PayloadJSON       json.RawMessage `json:"payload_json"`
	SourcePayloadJSON json.RawMessage `json:"source_payload_json"`
	Status            string          `json:"status"`
	ResolutionStatus  string          `json:"resolution_status"`
	ResolutionNote    string          `json:"resolution_note"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ResolvedAt        *time.Time      `json:"resolved_at"`
}

type identityMapJSON struct {
	ID             int64           `json:"id"`
	CorpID         string          `json:"corp_id"`
	ExternalUserID string          `json:"external_userid"`
	UnionID        string          `json:"unionid"`
	OpenID         string          `json:"openid"`
	FollowUserID   string          `json:"follow_user_userid"`
	Name           string          `json:"name"`
	Type           *int32          `json:"type"`
	Avatar         string          `json:"avatar"`
	Gender         *int32          `json:"gender"`
	Status         string          `json:"status"`
	RawProfile     json.RawMessage `json:"raw_profile"`
	FirstSeenAt    time.Time       `json:"first_seen_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

var personFields = []string{"id", "mobile", "third_party_user_id", "created_at", "updated_at"}
var conflictFields = []string{"id", "conflict_type", "unionid", "candidate_unionid", "external_userid", "openid", "mobile", "source_type", "source_key", "payload_json", "source_payload_json", "status", "resolution_status", "resolution_note", "created_at", "updated_at", "resolved_at"}
var identityMapFields = []string{"id", "corp_id", "external_userid", "unionid", "openid", "follow_user_userid", "name", "type", "avatar", "gender", "status", "raw_profile", "first_seen_at", "last_seen_at", "created_at", "updated_at"}

// AdaptPerson verifies the archive row and projects only timestamps plus
// opaque identity evidence. Signed and zero V1 IDs remain source facts.
func AdaptPerson(row v1archive.ArchivedRow, archiveHMACKey []byte) (PersonFact, error) {
	fields, source, roots, err := archiveFields(row, PeopleTableID, archiveHMACKey)
	if err != nil {
		return PersonFact{}, err
	}
	var value personJSON
	if err := decodeExact(fields, row.Payload, &value, personFields, nil, nil); err != nil || !validTimes(value.CreatedAt, value.UpdatedAt) {
		return PersonFact{}, ErrFact
	}
	return PersonFact{
		SourceID: value.ID, Source: source,
		MobileDigest:           fieldDigest(archiveHMACKey, PeopleTableID, "mobile", fields["mobile"]),
		ThirdPartyUserIDDigest: fieldDigest(archiveHMACKey, PeopleTableID, "third_party_user_id", fields["third_party_user_id"]),
		CreatedAt:              normalizeTime(value.CreatedAt), UpdatedAt: normalizeTime(value.UpdatedAt),
		PrivateDigest: payloadDigest(archiveHMACKey, PeopleTableID, row.Payload), RedactedRoots: roots,
	}, nil
}

// AdaptConflict preserves conflict metadata while all identity values, JSON,
// source keys, and notes remain opaque.
func AdaptConflict(row v1archive.ArchivedRow, archiveHMACKey []byte) (ConflictFact, error) {
	fields, source, roots, err := archiveFields(row, IdentityConflictsTableID, archiveHMACKey)
	if err != nil {
		return ConflictFact{}, err
	}
	var value conflictJSON
	if err := decodeExact(fields, row.Payload, &value, conflictFields, map[string]bool{"resolved_at": true}, map[string]bool{"payload_json": true, "source_payload_json": true}); err != nil ||
		!safeText(value.ConflictType, value.SourceType, value.Status, value.ResolutionStatus) || !validTimes(value.CreatedAt, value.UpdatedAt) || !validOptionalTime(value.ResolvedAt) {
		return ConflictFact{}, ErrFact
	}
	return ConflictFact{
		SourceID: value.ID, Source: source, ConflictType: value.ConflictType, SourceType: value.SourceType, Status: value.Status, ResolutionStatus: value.ResolutionStatus,
		UnionIDDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "unionid", fields["unionid"]), CandidateUnionIDDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "candidate_unionid", fields["candidate_unionid"]),
		ExternalUserIDDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "external_userid", fields["external_userid"]), OpenIDDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "openid", fields["openid"]),
		MobileDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "mobile", fields["mobile"]), SourceKeyDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "source_key", fields["source_key"]),
		PayloadJSONDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "payload_json", fields["payload_json"]), SourcePayloadDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "source_payload_json", fields["source_payload_json"]),
		ResolutionNoteDigest: fieldDigest(archiveHMACKey, IdentityConflictsTableID, "resolution_note", fields["resolution_note"]),
		CreatedAt:            normalizeTime(value.CreatedAt), UpdatedAt: normalizeTime(value.UpdatedAt), ResolvedAt: normalizeTimePointer(value.ResolvedAt),
		PrivateDigest: payloadDigest(archiveHMACKey, IdentityConflictsTableID, row.Payload), RedactedRoots: roots,
	}, nil
}

// AdaptMissingRootIdentity adapts a single map row without asserting that it
// belongs to the two-row DM01 missing-root subset.
func AdaptMissingRootIdentity(row v1archive.ArchivedRow, archiveHMACKey []byte) (MissingRootIdentityFact, error) {
	fields, source, roots, err := archiveFields(row, ExternalContactIdentityMapID, archiveHMACKey)
	if err != nil {
		return MissingRootIdentityFact{}, err
	}
	var value identityMapJSON
	if err := decodeExact(fields, row.Payload, &value, identityMapFields, map[string]bool{"type": true, "gender": true}, map[string]bool{"raw_profile": true}); err != nil ||
		!safeText(value.Status) || !validTimes(value.FirstSeenAt, value.LastSeenAt, value.CreatedAt, value.UpdatedAt) {
		return MissingRootIdentityFact{}, ErrFact
	}
	result := MissingRootIdentityFact{
		SourceID: value.ID, Source: source, Type: cloneInt32(value.Type), Status: value.Status,
		CorpIDDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "corp_id", fields["corp_id"]), ExternalUserIDDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "external_userid", fields["external_userid"]),
		UnionIDDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "unionid", fields["unionid"]), OpenIDDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "openid", fields["openid"]),
		FollowUserIDDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "follow_user_userid", fields["follow_user_userid"]), NameDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "name", fields["name"]),
		AvatarDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "avatar", fields["avatar"]), RawProfileDigest: fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "raw_profile", fields["raw_profile"]),
		FirstSeenAt: normalizeTime(value.FirstSeenAt), LastSeenAt: normalizeTime(value.LastSeenAt), CreatedAt: normalizeTime(value.CreatedAt), UpdatedAt: normalizeTime(value.UpdatedAt),
		PrivateDigest: payloadDigest(archiveHMACKey, ExternalContactIdentityMapID, row.Payload), RedactedRoots: roots,
	}
	if value.Gender != nil {
		digest := fieldDigest(archiveHMACKey, ExternalContactIdentityMapID, "gender", fields["gender"])
		result.GenderDigest = &digest
	}
	return result, nil
}

func archiveFields(row v1archive.ArchivedRow, tableID string, key []byte) (map[string]json.RawMessage, SourceEnvelope, []string, error) {
	zero := [sha256.Size]byte{}
	if len(key) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 ||
		row.SourceKeyHMAC == zero || row.PayloadHMAC == zero || row.FieldHMAC == zero || !json.Valid(row.Payload) {
		return nil, SourceEnvelope{}, nil, ErrArchiveRow
	}
	canonical, roots, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) || !sameStrings(roots, row.RedactedFields) {
		return nil, SourceEnvelope{}, nil, ErrArchiveRow
	}
	name := strings.TrimPrefix(tableID, "public/")
	payload, err := v1archive.PayloadHMAC(key, name, row.Payload)
	if err != nil || !hmac.Equal(payload[:], row.PayloadHMAC[:]) {
		return nil, SourceEnvelope{}, nil, ErrArchiveRow
	}
	field, err := v1archive.FieldHMAC(key, name, row.RedactedFields)
	if err != nil || !hmac.Equal(field[:], row.FieldHMAC[:]) {
		return nil, SourceEnvelope{}, nil, ErrArchiveRow
	}
	fields := make(map[string]json.RawMessage)
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, SourceEnvelope{}, nil, ErrFact
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, SourceEnvelope{}, nil, ErrFact
	}
	value, found := fields["id"]
	var id int64
	if !found || json.Unmarshal(value, &id) != nil {
		return nil, SourceEnvelope{}, nil, ErrFact
	}
	source, err := v1archive.SourceKeyHMAC(key, name, []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil || !hmac.Equal(source[:], row.SourceKeyHMAC[:]) {
		return nil, SourceEnvelope{}, nil, ErrArchiveRow
	}
	return fields, SourceEnvelope{SourceKeyDigest: OpaqueDigest(row.SourceKeyHMAC), PayloadDigest: OpaqueDigest(row.PayloadHMAC), FieldDigest: OpaqueDigest(row.FieldHMAC)}, append([]string{}, roots...), nil
}

func decodeExact(fields map[string]json.RawMessage, payload []byte, target any, names []string, nullable, jsonMayBeNull map[string]bool) error {
	if len(fields) != len(names) {
		return ErrFact
	}
	for _, name := range names {
		raw, found := fields[name]
		if !found || !json.Valid(raw) || (!nullable[name] && !jsonMayBeNull[name] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
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

func fieldDigest(key []byte, table, field string, raw []byte) OpaqueDigest {
	return keyedDigest(key, "field", table, []byte(field), raw)
}

// payloadDigest covers the authenticated archived payload as retained. If the
// archive redacted a value, this is deliberately not a claim about its former
// plaintext; RedactedRoots preserves that boundary.
func payloadDigest(key []byte, table string, payload []byte) OpaqueDigest {
	return keyedDigest(key, "private-payload", table, payload)
}

func keyedDigest(key []byte, purpose, table string, values ...[]byte) OpaqueDigest {
	mac := hmac.New(sha256.New, key)
	for _, value := range append([][]byte{[]byte("aicrm/v1-deferred-identity-history/" + purpose + "/v1"), []byte(table)}, values...) {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(value)
	}
	var result OpaqueDigest
	copy(result[:], mac.Sum(nil))
	return result
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

func validOptionalTime(value *time.Time) bool { return value == nil || !value.IsZero() }

func normalizeTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func normalizeTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := normalizeTime(*value)
	return &copy
}

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
