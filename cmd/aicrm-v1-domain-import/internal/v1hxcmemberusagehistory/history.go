// Package v1hxcmemberusagehistory adapts sealed V1 HXC projection rows into
// inert, per-generation historical observations. It has no current-membership,
// owner, task, queue, or Provider dependency.
package v1hxcmemberusagehistory

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const MemberUsageProjectionTableID = "public/ai_audience_hxc_member_usage_projection"

const (
	ReasonInvalidSourceEnvelope = "invalid_member_usage_source_envelope"
	ReasonInvalidSourcePayload  = "invalid_member_usage_source_payload"
	ReasonUnknownRedactedField  = "unknown_member_usage_redacted_field"
	ReasonRequiredFieldRedacted = "required_member_usage_field_redacted"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// SourceEnvelope binds a candidate to one authenticated archive row. It is
// private import evidence, never an HTTP DTO.
type SourceEnvelope struct {
	SourceOrdinal  int64    `json:"-"`
	SourceKeyHMAC  [32]byte `json:"-"`
	PayloadHMAC    [32]byte `json:"-"`
	FieldHMAC      [32]byte `json:"-"`
	RedactedFields []string `json:"-"`
}

// MemberUsageObservationFact is a generation observation, not a present V2
// entitlement, registration, owner, or usage decision. Its source identities
// stay private and are available only to a later exact resolver.
type MemberUsageObservationFact struct {
	Generation          int64           `json:"generation"`
	IsMember            bool            `json:"is_member"`
	IsRegistered        bool            `json:"is_registered"`
	RegisteredAt        *time.Time      `json:"registered_at"`
	HasRealUsage        bool            `json:"has_real_usage"`
	FirstUsedAt         *time.Time      `json:"first_used_at"`
	LastUsedAt          *time.Time      `json:"last_used_at"`
	MemberSince         *time.Time      `json:"member_since"`
	MembershipExpiresAt *time.Time      `json:"membership_expires_at"`
	MembershipTier      string          `json:"membership_tier"`
	MembershipStatus    string          `json:"membership_status"`
	MembershipSource    string          `json:"membership_source"`
	RegistrationSource  string          `json:"registration_source"`
	UsageSource         string          `json:"usage_source"`
	UpdatedAt           *time.Time      `json:"updated_at"`
	PayloadJSON         json.RawMessage `json:"-"`
	ProjectedAt         time.Time       `json:"projected_at"`
	Source              SourceEnvelope  `json:"-"`

	resolverUnionID string
	ownerUserID     string
	mobileHash      string
}

// ResolverUnionID is private input for a future exact DM01 lookup.
func (fact MemberUsageObservationFact) ResolverUnionID() string { return fact.resolverUnionID }

// LegacyOwnerUserID is a V1 source reference, never a V2 Staff ID.
func (fact MemberUsageObservationFact) LegacyOwnerUserID() string { return fact.ownerUserID }

// MobileHash is private source material and cannot establish a Customer link.
func (fact MemberUsageObservationFact) MobileHash() string { return fact.mobileHash }

type Result struct {
	Disposition Disposition
	Reason      string
	Fact        *MemberUsageObservationFact
}

type sourceJSON struct {
	Generation          int64           `json:"generation"`
	UnionID             string          `json:"unionid"`
	OwnerUserID         string          `json:"owner_userid"`
	MobileHash          string          `json:"mobile_hash"`
	IsMember            bool            `json:"is_member"`
	IsRegistered        bool            `json:"is_registered"`
	RegisteredAt        *time.Time      `json:"registered_at"`
	HasRealUsage        bool            `json:"has_real_usage"`
	FirstUsedAt         *time.Time      `json:"first_used_at"`
	LastUsedAt          *time.Time      `json:"last_used_at"`
	MemberSince         *time.Time      `json:"member_since"`
	MembershipExpiresAt *time.Time      `json:"membership_expires_at"`
	MembershipTier      string          `json:"membership_tier"`
	MembershipStatus    string          `json:"membership_status"`
	MembershipSource    string          `json:"membership_source"`
	RegistrationSource  string          `json:"registration_source"`
	UsageSource         string          `json:"usage_source"`
	UpdatedAt           *time.Time      `json:"updated_at"`
	PayloadJSON         json.RawMessage `json:"payload_json"`
	ProjectedAt         time.Time       `json:"projected_at"`
}

var requiredFields = []string{
	"generation", "unionid", "owner_userid", "mobile_hash", "is_member", "is_registered", "registered_at", "has_real_usage", "first_used_at", "last_used_at", "member_since", "membership_expires_at", "membership_tier", "membership_status", "membership_source", "registration_source", "usage_source", "updated_at", "payload_json", "projected_at",
}

var nullableFields = map[string]bool{
	"registered_at": true, "first_used_at": true, "last_used_at": true, "member_since": true, "membership_expires_at": true, "updated_at": true, "payload_json": true,
}

// AdaptMemberUsageObservation verifies one exact archive row and preserves the
// original generation as an immutable observation. expectedOrdinal is supplied
// by the stream owner so a reordered row cannot become a candidate.
func AdaptMemberUsageObservation(row v1archive.ArchivedRow, sourceHMACKey []byte, expectedOrdinal int64) Result {
	if expectedOrdinal < 1 || len(sourceHMACKey) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != MemberUsageProjectionTableID || row.SourceOrdinal != expectedOrdinal || zeroDigest(row.SourceKeyHMAC) || zeroDigest(row.PayloadHMAC) || zeroDigest(row.FieldHMAC) || !json.Valid(row.Payload) {
		return quarantine(ReasonInvalidSourceEnvelope)
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(row.Payload, &fields) != nil || fields == nil || !hasExactShape(fields) {
		return quarantine(ReasonInvalidSourcePayload)
	}
	roots, valid := validRedactions(row.RedactedFields)
	if !valid {
		return quarantine(ReasonUnknownRedactedField)
	}
	if len(roots) != 0 {
		return quarantine(ReasonRequiredFieldRedacted)
	}
	var source sourceJSON
	if json.Unmarshal(row.Payload, &source) != nil || !json.Valid(source.PayloadJSON) || source.ProjectedAt.IsZero() {
		return quarantine(ReasonInvalidSourcePayload)
	}
	sourceKeyJSON, err := json.Marshal([]any{source.Generation, source.OwnerUserID, source.UnionID})
	if err != nil {
		return quarantine(ReasonInvalidSourceEnvelope)
	}
	key, keyErr := v1archive.SourceKeyHMAC(sourceHMACKey, strings.TrimPrefix(MemberUsageProjectionTableID, "public/"), sourceKeyJSON)
	payload, payloadErr := v1archive.PayloadHMAC(sourceHMACKey, strings.TrimPrefix(MemberUsageProjectionTableID, "public/"), row.Payload)
	field, fieldErr := v1archive.FieldHMAC(sourceHMACKey, strings.TrimPrefix(MemberUsageProjectionTableID, "public/"), row.RedactedFields)
	if keyErr != nil || payloadErr != nil || fieldErr != nil || key != row.SourceKeyHMAC || payload != row.PayloadHMAC || field != row.FieldHMAC {
		return quarantine(ReasonInvalidSourceEnvelope)
	}
	fact := MemberUsageObservationFact{
		Generation: source.Generation, IsMember: source.IsMember, IsRegistered: source.IsRegistered, RegisteredAt: utcMicro(source.RegisteredAt),
		HasRealUsage: source.HasRealUsage, FirstUsedAt: utcMicro(source.FirstUsedAt), LastUsedAt: utcMicro(source.LastUsedAt), MemberSince: utcMicro(source.MemberSince),
		MembershipExpiresAt: utcMicro(source.MembershipExpiresAt), MembershipTier: source.MembershipTier, MembershipStatus: source.MembershipStatus,
		MembershipSource: source.MembershipSource, RegistrationSource: source.RegistrationSource, UsageSource: source.UsageSource, UpdatedAt: utcMicro(source.UpdatedAt),
		PayloadJSON: append(json.RawMessage(nil), source.PayloadJSON...), ProjectedAt: source.ProjectedAt.UTC().Truncate(time.Microsecond),
		Source:          SourceEnvelope{SourceOrdinal: row.SourceOrdinal, SourceKeyHMAC: row.SourceKeyHMAC, PayloadHMAC: row.PayloadHMAC, FieldHMAC: row.FieldHMAC, RedactedFields: append([]string(nil), row.RedactedFields...)},
		resolverUnionID: source.UnionID, ownerUserID: source.OwnerUserID, mobileHash: source.MobileHash,
	}
	return Result{Disposition: DispositionCandidate, Fact: &fact}
}

func hasExactShape(fields map[string]json.RawMessage) bool {
	if len(fields) != len(requiredFields) {
		return false
	}
	for _, field := range requiredFields {
		value, found := fields[field]
		if !found || len(value) == 0 || (!nullableFields[field] && string(value) == "null") {
			return false
		}
	}
	return true
}

func validRedactions(paths []string) (map[string]struct{}, bool) {
	roots := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		root := strings.SplitN(path, ".", 2)[0]
		if root == "" || !contains(requiredFields, root) {
			return nil, false
		}
		if _, duplicate := roots[path]; duplicate {
			return nil, false
		}
		roots[path] = struct{}{}
	}
	return roots, true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func utcMicro(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC().Truncate(time.Microsecond)
	return &copy
}

func zeroDigest(value [32]byte) bool { return value == [32]byte{} }

func quarantine(reason string) Result {
	return Result{Disposition: DispositionQuarantine, Reason: reason}
}
