package v1membergridhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	usageSnapshotRecoveryFormat     = "v1_usage_has_token_usage_recovery/v1"
	usageSnapshotRecoveryDumpSHA256 = "b4b35787e5d7a12635c2b4456df37c73fd5ebd8fb67f5d75bc0a0ba30da242c8"
	usageSnapshotRecoveryArchiveRun = "v1-full-archive-20260827"
	usageSnapshotRecoveryField      = "has_token_usage"

	// usageSnapshotRecoverySourceTable is the unqualified V1 table name used
	// by the immutable archive HMACs. It is not a V2 target table.
	usageSnapshotRecoverySourceTable = "service_period_huangyoucan_usage_snapshot"
	// usageSnapshotRecoveryHMACDomain is only an HMAC domain separator.
	usageSnapshotRecoveryHMACDomain = "service_period_huangyoucan_usage_snapshot_recovery"
)

var ErrInvalidUsageSnapshotRecovery = errors.New("invalid V1 usage snapshot recovery")

// UsageSnapshotRecoveryScope binds a supplemental boolean to exactly one
// frozen source dump and reconciled archive run. It is JSONL-safe: no raw
// source key or source payload is retained in it.
type UsageSnapshotRecoveryScope struct {
	Format       string `json:"format"`
	DumpSHA256   string `json:"dump_sha256"`
	ArchiveRunID string `json:"archive_run_id"`
	TableID      string `json:"table_id"`
	Field        string `json:"field"`
}

// FixedUsageSnapshotRecoveryScope returns the only scope supported by this
// narrow false-positive recovery. Other archive runs, tables, or fields fail
// closed rather than becoming a general recovery mechanism.
func FixedUsageSnapshotRecoveryScope() UsageSnapshotRecoveryScope {
	return UsageSnapshotRecoveryScope{
		Format:       usageSnapshotRecoveryFormat,
		DumpSHA256:   usageSnapshotRecoveryDumpSHA256,
		ArchiveRunID: usageSnapshotRecoveryArchiveRun,
		TableID:      UsageSnapshotsTableID,
		Field:        usageSnapshotRecoveryField,
	}
}

// UsageSnapshotRecoveryEntry contains the authenticated supplemental boolean.
// It intentionally never contains the raw primary key or original payload.
type UsageSnapshotRecoveryEntry struct {
	Scope               UsageSnapshotRecoveryScope `json:"scope"`
	SourceKeyHMAC       [sha256.Size]byte          `json:"source_key_hmac"`
	OriginalPayloadHMAC [sha256.Size]byte          `json:"original_payload_hmac"`
	OriginalFieldHMAC   [sha256.Size]byte          `json:"original_field_hmac"`
	HasTokenUsage       bool                       `json:"has_token_usage"`
	EntryHMAC           [sha256.Size]byte          `json:"entry_hmac"`
}

// BuildUsageSnapshotRecoveryEntry verifies a recovered V1 to_jsonb row
// against its immutable archive row before retaining the one non-secret bool.
// sourceKeyJSON and fullPayload are transient inputs and are never copied into
// the resulting entry.
func BuildUsageSnapshotRecoveryEntry(original v1archive.ArchivedRow, sourceKeyJSON, fullPayload, archiveSourceHMACKey []byte, scope UsageSnapshotRecoveryScope) (UsageSnapshotRecoveryEntry, error) {
	if !validUsageSnapshotRecoveryScope(scope) || !validRow(original, UsageSnapshotsTableID) ||
		!onlyUsageSnapshotRecoveryRedaction(original.RedactedFields) {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}

	sourceKey, err := v1archive.SourceKeyHMAC(archiveSourceHMACKey, usageSnapshotRecoverySourceTable, sourceKeyJSON)
	if err != nil || sourceKey != original.SourceKeyHMAC {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}
	redacted, paths, err := v1archive.RedactPayload(fullPayload)
	if err != nil || !onlyUsageSnapshotRecoveryRedaction(paths) || !bytes.Equal(redacted, original.Payload) {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}
	payloadHMAC, err := v1archive.PayloadHMAC(archiveSourceHMACKey, usageSnapshotRecoverySourceTable, redacted)
	if err != nil || payloadHMAC != original.PayloadHMAC {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}
	fieldHMAC, err := v1archive.FieldHMAC(archiveSourceHMACKey, usageSnapshotRecoverySourceTable, paths)
	if err != nil || fieldHMAC != original.FieldHMAC {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}
	hasTokenUsage, err := usageSnapshotRecoveryBool(fullPayload)
	if err != nil {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}

	entry := UsageSnapshotRecoveryEntry{
		Scope:               scope,
		SourceKeyHMAC:       original.SourceKeyHMAC,
		OriginalPayloadHMAC: original.PayloadHMAC,
		OriginalFieldHMAC:   original.FieldHMAC,
		HasTokenUsage:       hasTokenUsage,
	}
	entry.EntryHMAC, err = usageSnapshotRecoveryEntryHMAC(archiveSourceHMACKey, entry)
	if err != nil {
		return UsageSnapshotRecoveryEntry{}, ErrInvalidUsageSnapshotRecovery
	}
	return entry, nil
}

// AdaptUsageSnapshotRecovery validates a supplemental entry, then adapts an
// in-memory clone of the immutable row. It does not mutate original.
func AdaptUsageSnapshotRecovery(original v1archive.ArchivedRow, entry UsageSnapshotRecoveryEntry, archiveSourceHMACKey []byte) (Decision[UsageSnapshotHistory], error) {
	if !validUsageSnapshotRecoveryScope(entry.Scope) || !validRow(original, UsageSnapshotsTableID) ||
		!onlyUsageSnapshotRecoveryRedaction(original.RedactedFields) ||
		entry.SourceKeyHMAC != original.SourceKeyHMAC ||
		entry.OriginalPayloadHMAC != original.PayloadHMAC ||
		entry.OriginalFieldHMAC != original.FieldHMAC {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	payloadHMAC, err := v1archive.PayloadHMAC(archiveSourceHMACKey, usageSnapshotRecoverySourceTable, original.Payload)
	if err != nil || payloadHMAC != original.PayloadHMAC {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	fieldHMAC, err := v1archive.FieldHMAC(archiveSourceHMACKey, usageSnapshotRecoverySourceTable, original.RedactedFields)
	if err != nil || fieldHMAC != original.FieldHMAC {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	wantEntryHMAC, err := usageSnapshotRecoveryEntryHMAC(archiveSourceHMACKey, entry)
	if err != nil || wantEntryHMAC != entry.EntryHMAC {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(original.Payload, &payload); err != nil {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	value, found := payload[usageSnapshotRecoveryField]
	if !found || !bytes.Equal(value, []byte(`"[REDACTED]"`)) {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	if entry.HasTokenUsage {
		payload[usageSnapshotRecoveryField] = json.RawMessage("true")
	} else {
		payload[usageSnapshotRecoveryField] = json.RawMessage("false")
	}
	recoveredPayload, err := json.Marshal(payload)
	if err != nil {
		return Decision[UsageSnapshotHistory]{}, ErrInvalidUsageSnapshotRecovery
	}
	recovered := original
	recovered.Payload = recoveredPayload
	recovered.RedactedFields = nil
	return AdaptUsageSnapshot(recovered), nil
}

func validUsageSnapshotRecoveryScope(scope UsageSnapshotRecoveryScope) bool {
	return scope == FixedUsageSnapshotRecoveryScope()
}

func onlyUsageSnapshotRecoveryRedaction(paths []string) bool {
	return len(paths) == 1 && paths[0] == usageSnapshotRecoveryField
}

func usageSnapshotRecoveryBool(payload []byte) (bool, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(payload, &values); err != nil {
		return false, err
	}
	value, found := values[usageSnapshotRecoveryField]
	if !found {
		return false, errors.New("usage snapshot token usage missing")
	}
	if bytes.Equal(value, []byte("true")) {
		return true, nil
	}
	if bytes.Equal(value, []byte("false")) {
		return false, nil
	}
	return false, errors.New("usage snapshot token usage is not bool")
}

func usageSnapshotRecoveryEntryHMAC(key []byte, entry UsageSnapshotRecoveryEntry) ([sha256.Size]byte, error) {
	payload, err := json.Marshal(struct {
		Scope               UsageSnapshotRecoveryScope `json:"scope"`
		SourceKeyHMAC       string                     `json:"source_key_hmac"`
		OriginalPayloadHMAC string                     `json:"original_payload_hmac"`
		OriginalFieldHMAC   string                     `json:"original_field_hmac"`
		HasTokenUsage       bool                       `json:"has_token_usage"`
	}{
		Scope:               entry.Scope,
		SourceKeyHMAC:       hex.EncodeToString(entry.SourceKeyHMAC[:]),
		OriginalPayloadHMAC: hex.EncodeToString(entry.OriginalPayloadHMAC[:]),
		OriginalFieldHMAC:   hex.EncodeToString(entry.OriginalFieldHMAC[:]),
		HasTokenUsage:       entry.HasTokenUsage,
	})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode usage snapshot recovery entry: %w", err)
	}
	return v1archive.PayloadHMAC(key, usageSnapshotRecoveryHMACDomain, payload)
}
