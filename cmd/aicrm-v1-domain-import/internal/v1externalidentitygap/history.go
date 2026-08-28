// Package v1externalidentitygap adapts the V1 external-contact identities
// missing from DM01 into non-actionable, caller-owned candidates.
package v1externalidentitygap

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const TableID = "public/wecom_external_contact_identity_map"

var ErrInvalidSource = errors.New("invalid V1 external identity gap source")

type RootRoute string

const (
	// RootRouteUnbound keeps an authenticated V1 external identity without a
	// Customer binding. It must not create a Customer or current identity.
	RootRouteUnbound RootRoute = "unbound"
	// RootRouteRequiresVerifiedRoot asks a later caller-owned transaction to
	// prove the existing DM01 root and scoped-identity conflict rules.
	RootRouteRequiresVerifiedRoot RootRoute = "requires_verified_root"
)

// Fact intentionally has no JSON surface. Source-only fields remain solely in
// the encrypted archive; this carries only the data a later owner writer needs.
type Fact struct {
	SourceKeyHMAC     [sha256.Size]byte `json:"-"`
	SourcePayloadHMAC [sha256.Size]byte `json:"-"`
	SourceFieldHMAC   [sha256.Size]byte `json:"-"`
	Scope             string            `json:"-"`
	ExternalUserID    string            `json:"-"`
	UnionID           *string           `json:"-"`
	RootRoute         RootRoute         `json:"-"`
}

type sourcePayload struct {
	ID             json.Number
	CorpID         string
	ExternalUserID string
	UnionID        *string
	UpdatedAt      string
}

// Adapt authenticates one V1 archive row and returns a non-actionable fact.
// It neither reads roots nor infers a V2 customer mapping.
func Adapt(row v1archive.ArchivedRow, sourceHMACKey []byte) (Fact, error) {
	if len(sourceHMACKey) < sha256.Size || row.AdapterID != v1archive.DefaultAdapterID || row.TableID != TableID || row.SourceOrdinal < 1 || zero(row.SourceKeyHMAC) || zero(row.PayloadHMAC) || zero(row.FieldHMAC) {
		return Fact{}, invalid("envelope")
	}

	canonical, fields, err := v1archive.RedactPayload(row.Payload)
	if err != nil || !bytes.Equal(canonical, row.Payload) {
		return Fact{}, invalid("payload")
	}
	for _, field := range row.RedactedFields {
		if !allowedRedaction(field) {
			return Fact{}, invalid("redaction")
		}
	}
	if v1archive.IsRedacted(row, "id") || v1archive.IsRedacted(row, "corp_id") || v1archive.IsRedacted(row, "external_userid") || v1archive.IsRedacted(row, "unionid") || v1archive.IsRedacted(row, "updated_at") || !sameStrings(fields, row.RedactedFields) {
		return Fact{}, invalid("redaction")
	}
	payloadHMAC, err := v1archive.PayloadHMAC(sourceHMACKey, sourceTable, canonical)
	if err != nil || payloadHMAC != row.PayloadHMAC {
		return Fact{}, invalid("payload_hmac")
	}
	fieldHMAC, err := v1archive.FieldHMAC(sourceHMACKey, sourceTable, fields)
	if err != nil || fieldHMAC != row.FieldHMAC {
		return Fact{}, invalid("field_hmac")
	}

	payload, err := decodePayload(canonical)
	if err != nil {
		return Fact{}, err
	}
	id, err := strconv.ParseInt(string(payload.ID), 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != string(payload.ID) {
		return Fact{}, invalid("id")
	}
	sourceKey, err := v1archive.SourceKeyHMAC(sourceHMACKey, sourceTable, []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil || sourceKey != row.SourceKeyHMAC {
		return Fact{}, invalid("source_hmac")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt); err != nil {
		return Fact{}, invalid("updated_at")
	}

	scope := "wecom-corp:" + payload.CorpID
	normalized, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: scope, Value: payload.ExternalUserID})
	if err != nil || normalized.Scope != scope || normalized.NormalizedValue != payload.ExternalUserID || identityapp.ValidateNormalized(normalized) != nil {
		return Fact{}, invalid("identity")
	}

	fact := Fact{SourceKeyHMAC: row.SourceKeyHMAC, SourcePayloadHMAC: row.PayloadHMAC, SourceFieldHMAC: row.FieldHMAC, Scope: normalized.Scope, ExternalUserID: normalized.NormalizedValue, UnionID: payload.UnionID, RootRoute: RootRouteUnbound}
	if payload.UnionID != nil {
		fact.RootRoute = RootRouteRequiresVerifiedRoot
	}
	return fact, nil
}

const sourceTable = "wecom_external_contact_identity_map"

func decodePayload(raw []byte) (sourcePayload, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return sourcePayload{}, invalid("payload")
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return sourcePayload{}, invalid("payload")
	}
	var result sourcePayload
	if err := decodeRequired(value, "id", &result.ID); err != nil || decodeRequired(value, "corp_id", &result.CorpID) != nil || decodeRequired(value, "external_userid", &result.ExternalUserID) != nil || decodeRequired(value, "updated_at", &result.UpdatedAt) != nil {
		return sourcePayload{}, invalid("shape")
	}
	rawUnionID, found := value["unionid"]
	if !found || len(rawUnionID) == 0 {
		return sourcePayload{}, invalid("shape")
	}
	if string(rawUnionID) == "null" {
		return result, nil
	}
	var unionID string
	if err := json.Unmarshal(rawUnionID, &unionID); err != nil {
		return sourcePayload{}, invalid("unionid")
	}
	if unionID == "" {
		return result, nil
	}
	if strings.TrimSpace(unionID) != unionID || strings.IndexFunc(unionID, unicode.IsControl) >= 0 {
		return sourcePayload{}, invalid("unionid")
	}
	result.UnionID = &unionID
	return result, nil
}

func decodeRequired(values map[string]json.RawMessage, key string, target any) error {
	raw, found := values[key]
	if !found || len(raw) == 0 || string(raw) == "null" {
		return ErrInvalidSource
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	return nil
}

func allowedRedaction(path string) bool {
	root := path
	if index := strings.IndexAny(path, ".["); index >= 0 {
		root = path[:index]
	}
	switch root {
	case "openid", "follow_user_userid", "name", "type", "avatar", "gender", "status", "first_seen_at", "last_seen_at", "created_at":
		return root == path
	case "raw_profile":
		return path == root || strings.HasPrefix(path, root+".") || strings.HasPrefix(path, root+"[")
	default:
		return false
	}
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

func zero(value [sha256.Size]byte) bool { return value == [sha256.Size]byte{} }

func invalid(reason string) error { return &ValidationError{Reason: reason} }

// ValidationError contains a fixed reason only; it never includes source data.
type ValidationError struct{ Reason string }

func (err *ValidationError) Error() string {
	return "invalid V1 external identity gap source: " + err.Reason
}
func (*ValidationError) Unwrap() error { return ErrInvalidSource }
