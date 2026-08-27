// Package v1channel makes side-effect-free migration decisions for V1 channel
// sources. It intentionally has no database, Provider, queue, or command
// dependency.
package v1channel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Disposition string

const (
	Candidate  Disposition = "candidate"
	Archive    Disposition = "archive"
	Blocked    Disposition = "blocked"
	Quarantine Disposition = "quarantine"
)

const (
	ReasonInvalidChannelDefinition        = "invalid_channel_definition"
	ReasonMissingSourcePayload            = "missing_source_payload"
	ReasonInvalidSourcePayload            = "invalid_source_payload"
	ReasonUnsupportedChannelKind          = "unsupported_channel_kind"
	ReasonStaffMappingRequired            = "staff_mapping_required"
	ReasonHistoricalEntryProjectionNeeded = "historical_entry_projection_required"
	ReasonProviderEffectHistoryArchive    = "provider_effect_history_archive_only"
	ReasonCallbackRuntimeArchive          = "callback_runtime_archive_only"
	ReasonProviderAssetArchive            = "provider_asset_legacy_unverified"
	ReasonCallbackAliasArchive            = "callback_alias_archive_only"
	ReasonWelcomeDependencyArchive        = "welcome_dependency_archive_only"
	ReasonWelcomeExecutionArchive         = "welcome_execution_archive_only"
	ReasonUnknownChannelSourceTable       = "unknown_channel_source_table"
)

// AutomationChannelRow contains selected V1 definition fields plus source JSON
// decrypted from an already verified encrypted archive. SourcePayload is never
// copied into a candidate; it is accepted only to produce an integrity digest.
type AutomationChannelRow struct {
	SourceID      int64
	ChannelCode   string
	ChannelName   string
	ChannelType   string
	CarrierType   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	SourcePayload json.RawMessage
}

// LocalInactiveConfig is the complete source-derived configuration whitelist.
// A later Contact-owned writer may add fixed V2 defaults, but it must never use
// old QR/link/scene/callback/welcome/material/tag/staff values from V1.
type LocalInactiveConfig struct {
	SchemaVersion int    `json:"schema_version"`
	ChannelType   string `json:"channel_type"`
	CarrierType   string `json:"carrier_type"`
	ChannelCode   string `json:"channel_code"`
	ChannelName   string `json:"channel_name"`
	Status        string `json:"status"`
}

// CandidateChannel is a local inactive channel definition, not a V2 ID,
// command, receipt, event, callback, or Provider asset. SourceKey is retained
// solely for an importer journal/archive association.
type CandidateChannel struct {
	SourceKey             string
	Code                  string
	Name                  string
	Config                LocalInactiveConfig
	CreatedAt             time.Time
	UpdatedAt             time.Time
	MigrationActorID      int64
	SourcePayloadDigest   string
	SourceArchiveRetained bool
}

type Decision[T any] struct {
	Disposition Disposition
	Reason      string
	Candidate   *T
}

// ConvertAutomationChannel emits only an inactive local definition. Unsupported
// but otherwise readable channel types are archived rather than guessed.
func ConvertAutomationChannel(row AutomationChannelRow, migrationActorID int64) Decision[CandidateChannel] {
	if !validChannelRow(row) || migrationActorID < 1 {
		return quarantine[CandidateChannel](ReasonInvalidChannelDefinition)
	}
	if len(row.SourcePayload) == 0 {
		return quarantine[CandidateChannel](ReasonMissingSourcePayload)
	}
	if !json.Valid(row.SourcePayload) {
		return quarantine[CandidateChannel](ReasonInvalidSourcePayload)
	}
	if !supportedKind(row.ChannelType) || !supportedKind(row.CarrierType) {
		return archive[CandidateChannel](ReasonUnsupportedChannelKind)
	}
	digest := sha256.Sum256(row.SourcePayload)
	candidate := CandidateChannel{
		SourceKey:             "automation_channel:" + decimal(row.SourceID),
		Code:                  row.ChannelCode,
		Name:                  row.ChannelName,
		CreatedAt:             row.CreatedAt.UTC(),
		UpdatedAt:             row.UpdatedAt.UTC(),
		MigrationActorID:      migrationActorID,
		SourcePayloadDigest:   "sha256:" + hex.EncodeToString(digest[:]),
		SourceArchiveRetained: true,
		Config: LocalInactiveConfig{
			SchemaVersion: 1,
			ChannelType:   row.ChannelType,
			CarrierType:   row.CarrierType,
			ChannelCode:   row.ChannelCode,
			ChannelName:   row.ChannelName,
			Status:        "inactive",
		},
	}
	return canonical(candidate)
}

// ClassifyAuxiliaryTable makes every non-definition V1 channel source terminal
// without replaying an old callback, welcome task, asset, or Provider outcome.
func ClassifyAuxiliaryTable(table string) Decision[struct{}] {
	switch table {
	case "automation_channel_assignee":
		return blocked[struct{}](ReasonStaffMappingRequired)
	case "automation_channel_contact":
		return blocked[struct{}](ReasonHistoricalEntryProjectionNeeded)
	case "automation_channel_entry_effect_log":
		return archive[struct{}](ReasonProviderEffectHistoryArchive)
	case "automation_channel_entry_runtime":
		return archive[struct{}](ReasonCallbackRuntimeArchive)
	case "automation_channel_qrcode_asset":
		return archive[struct{}](ReasonProviderAssetArchive)
	case "automation_channel_scene_alias":
		return archive[struct{}](ReasonCallbackAliasArchive)
	case "channel_welcome_effect_dependency":
		return archive[struct{}](ReasonWelcomeDependencyArchive)
	case "channel_welcome_effect_graph":
		return archive[struct{}](ReasonWelcomeExecutionArchive)
	default:
		return quarantine[struct{}](ReasonUnknownChannelSourceTable)
	}
}

func validChannelRow(row AutomationChannelRow) bool {
	return row.SourceID > 0 && validText(row.ChannelCode, 200) && validText(row.ChannelName, 200) &&
		validText(row.ChannelType, 200) && validText(row.CarrierType, 200) &&
		!row.CreatedAt.IsZero() && !row.UpdatedAt.IsZero() && !row.UpdatedAt.Before(row.CreatedAt)
}

func validText(value string, maximum int) bool {
	return value == strings.TrimSpace(value) && value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum
}

func supportedKind(value string) bool {
	return value == "qrcode" || value == "link"
}

func canonical[T any](candidate T) Decision[T] {
	return Decision[T]{Disposition: Candidate, Candidate: &candidate}
}

func archive[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Archive, Reason: reason}
}

func blocked[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Blocked, Reason: reason}
}

func quarantine[T any](reason string) Decision[T] {
	return Decision[T]{Disposition: Quarantine, Reason: reason}
}

func decimal(value int64) string {
	return strconv.FormatInt(value, 10)
}
