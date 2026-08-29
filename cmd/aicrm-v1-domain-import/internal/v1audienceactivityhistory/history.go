// Package v1audienceactivityhistory adapts frozen V1 Audience activity rows
// into non-executable historical facts. It does not read current Audience
// state, resolve identities, or call a Provider.
package v1audienceactivityhistory

import (
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	PackageRunsTableID  = "public/ai_audience_package_run"
	MemberEventsTableID = "public/ai_audience_member_event"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest changes when a private source field changes, but cannot be
// used to recover that field. The encrypted source archive retains the row.
type OpaqueDigest [sha256.Size]byte

type PackageRunFact struct {
	SourceID          int64        `json:"-"`
	PackageSourceID   int64        `json:"-"`
	VersionSourceID   *int64       `json:"-"`
	RunType           string       `json:"run_type"`
	OriginalStatus    string       `json:"original_status"`
	RefreshStartedAt  time.Time    `json:"refresh_started_at"`
	RefreshFinishedAt *time.Time   `json:"refresh_finished_at"`
	LastWatermarkAt   *time.Time   `json:"last_watermark_at"`
	NextWatermarkAt   *time.Time   `json:"next_watermark_at"`
	ReturnedCount     int32        `json:"returned_count"`
	EnteredCount      int32        `json:"entered_count"`
	UpdatedCount      int32        `json:"updated_count"`
	ExitedCount       int32        `json:"exited_count"`
	MemberEventCount  int32        `json:"member_event_count"`
	DurationMS        int32        `json:"duration_ms"`
	CreatedAt         time.Time    `json:"created_at"`
	PrivateDigest     OpaqueDigest `json:"-"`
}

type MemberEventFact struct {
	SourceID        int64        `json:"-"`
	PackageSourceID int64        `json:"-"`
	RunSourceID     *int64       `json:"-"`
	MemberSourceID  *int64       `json:"-"`
	EventType       string       `json:"event_type"`
	IdentityKind    string       `json:"-"`
	OccurredAt      time.Time    `json:"occurred_at"`
	CreatedAt       time.Time    `json:"created_at"`
	PrivateDigest   OpaqueDigest `json:"-"`
}

type PackageRunResult struct {
	Source      SourceEnvelope
	Disposition Disposition
	Reason      string
	Fact        *PackageRunFact
}

type MemberEventResult struct {
	Source      SourceEnvelope
	Disposition Disposition
	Reason      string
	Fact        *MemberEventFact
}

type History struct {
	PackageRuns  []PackageRunResult
	MemberEvents []MemberEventResult
}

func AdaptHistory(runs, events []json.RawMessage) History {
	history := History{PackageRuns: make([]PackageRunResult, len(runs)), MemberEvents: make([]MemberEventResult, len(events))}
	for index, raw := range runs {
		history.PackageRuns[index] = adaptPackageRun(raw)
	}
	runIDs := uniqueRunIDs(history.PackageRuns)
	for index, raw := range events {
		history.MemberEvents[index] = adaptMemberEvent(raw)
		fact := history.MemberEvents[index].Fact
		if fact != nil && fact.RunSourceID != nil {
			if _, found := runIDs[*fact.RunSourceID]; !found {
				history.MemberEvents[index] = MemberEventResult{Disposition: DispositionQuarantine, Reason: "audience_activity_event_run_unresolved"}
			}
		}
	}
	uniqueEventIDs(history.MemberEvents)
	return history
}

func RequiredFields(table string) []string {
	switch table {
	case PackageRunsTableID:
		return []string{"id", "package_id", "version_id", "run_type", "status", "refresh_started_at", "refresh_finished_at", "last_watermark_at", "next_watermark_at", "returned_count", "entered_count", "updated_count", "exited_count", "member_event_count", "duration_ms", "error_message", "created_at"}
	case MemberEventsTableID:
		return []string{"id", "package_id", "run_id", "member_current_id", "event_type", "identity_type", "identity_value", "unionid", "mobile_hash", "owner_userid", "event_source_key", "payload_hash", "payload_json", "internal_event_id", "idempotency_key", "occurred_at", "created_at"}
	default:
		return nil
	}
}

func RequiredFieldRedacted(table string, row v1archive.ArchivedRow) bool {
	for _, field := range RequiredFields(table) {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func adaptPackageRun(raw json.RawMessage) PackageRunResult {
	fields, ok := object(raw)
	if !ok {
		return invalidRun()
	}
	id, idOK := required[int64](fields, "id")
	packageID, packageOK := required[int64](fields, "package_id")
	versionID, versionOK := optional[int64](fields, "version_id")
	runType, typeOK := required[string](fields, "run_type")
	status, statusOK := required[string](fields, "status")
	started, startedOK := required[time.Time](fields, "refresh_started_at")
	finished, finishedOK := optional[time.Time](fields, "refresh_finished_at")
	last, lastOK := optional[time.Time](fields, "last_watermark_at")
	next, nextOK := optional[time.Time](fields, "next_watermark_at")
	returned, returnedOK := required[int32](fields, "returned_count")
	entered, enteredOK := required[int32](fields, "entered_count")
	updated, updatedOK := required[int32](fields, "updated_count")
	exited, exitedOK := required[int32](fields, "exited_count")
	events, eventsOK := required[int32](fields, "member_event_count")
	duration, durationOK := required[int32](fields, "duration_ms")
	created, createdOK := required[time.Time](fields, "created_at")
	private, privateOK := opaque(fields, "error_message")
	if !idOK || id < 1 || !packageOK || packageID < 1 || !versionOK || !typeOK || !statusOK || !startedOK || started.IsZero() || !finishedOK || !lastOK || !nextOK || !returnedOK || !enteredOK || !updatedOK || !exitedOK || !eventsOK || !durationOK || !createdOK || created.IsZero() || !privateOK || (versionID != nil && *versionID < 1) {
		return invalidRun()
	}
	return PackageRunResult{Disposition: DispositionCandidate, Fact: &PackageRunFact{SourceID: id, PackageSourceID: packageID, VersionSourceID: versionID, RunType: runType, OriginalStatus: status, RefreshStartedAt: started, RefreshFinishedAt: finished, LastWatermarkAt: last, NextWatermarkAt: next, ReturnedCount: returned, EnteredCount: entered, UpdatedCount: updated, ExitedCount: exited, MemberEventCount: events, DurationMS: duration, CreatedAt: created, PrivateDigest: private}}
}

func adaptMemberEvent(raw json.RawMessage) MemberEventResult {
	fields, ok := object(raw)
	if !ok {
		return invalidEvent()
	}
	id, idOK := required[int64](fields, "id")
	packageID, packageOK := required[int64](fields, "package_id")
	runID, runOK := optional[int64](fields, "run_id")
	memberID, memberOK := optional[int64](fields, "member_current_id")
	eventType, eventTypeOK := required[string](fields, "event_type")
	identityKind, identityKindOK := required[string](fields, "identity_type")
	occurred, occurredOK := required[time.Time](fields, "occurred_at")
	created, createdOK := required[time.Time](fields, "created_at")
	private, privateOK := opaque(fields, "identity_value", "unionid", "mobile_hash", "owner_userid", "event_source_key", "payload_hash", "payload_json", "internal_event_id", "idempotency_key")
	if !idOK || id < 1 || !packageOK || packageID < 1 || !runOK || !memberOK || !eventTypeOK || !identityKindOK || !occurredOK || occurred.IsZero() || !createdOK || created.IsZero() || !privateOK || (runID != nil && *runID < 1) || (memberID != nil && *memberID < 1) {
		return invalidEvent()
	}
	return MemberEventResult{Disposition: DispositionCandidate, Fact: &MemberEventFact{SourceID: id, PackageSourceID: packageID, RunSourceID: runID, MemberSourceID: memberID, EventType: eventType, IdentityKind: identityKind, OccurredAt: occurred, CreatedAt: created, PrivateDigest: private}}
}

func invalidRun() PackageRunResult {
	return PackageRunResult{Disposition: DispositionQuarantine, Reason: "audience_activity_run_shape_invalid"}
}
func invalidEvent() MemberEventResult {
	return MemberEventResult{Disposition: DispositionQuarantine, Reason: "audience_activity_event_shape_invalid"}
}

func object(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var value map[string]json.RawMessage
	return value, json.Unmarshal(raw, &value) == nil && value != nil
}

func required[T any](fields map[string]json.RawMessage, name string) (T, bool) {
	var value T
	raw, found := fields[name]
	if !found || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return value, false
	}
	return value, true
}

func optional[T any](fields map[string]json.RawMessage, name string) (*T, bool) {
	raw, found := fields[name]
	if !found {
		return nil, false
	}
	if string(raw) == "null" {
		return nil, true
	}
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return &value, true
}

func opaque(fields map[string]json.RawMessage, names ...string) (OpaqueDigest, bool) {
	selected := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		raw, found := fields[name]
		if !found || !json.Valid(raw) {
			return OpaqueDigest{}, false
		}
		selected[name] = raw
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return OpaqueDigest{}, false
	}
	return OpaqueDigest(sha256.Sum256(encoded)), true
}

func uniqueRunIDs(values []PackageRunResult) map[int64]struct{} {
	ids := make(map[int64]struct{})
	duplicates := make(map[int64]struct{})
	for _, value := range values {
		if value.Fact == nil {
			continue
		}
		if _, found := ids[value.Fact.SourceID]; found {
			duplicates[value.Fact.SourceID] = struct{}{}
		}
		ids[value.Fact.SourceID] = struct{}{}
	}
	for index := range values {
		if values[index].Fact != nil {
			if _, duplicate := duplicates[values[index].Fact.SourceID]; duplicate {
				values[index] = PackageRunResult{Disposition: DispositionQuarantine, Reason: "audience_activity_source_id_ambiguous"}
			}
		}
	}
	for id := range duplicates {
		delete(ids, id)
	}
	return ids
}

func uniqueEventIDs(values []MemberEventResult) {
	ids := make(map[int64]struct{})
	duplicates := make(map[int64]struct{})
	for _, value := range values {
		if value.Fact == nil {
			continue
		}
		if _, found := ids[value.Fact.SourceID]; found {
			duplicates[value.Fact.SourceID] = struct{}{}
		}
		ids[value.Fact.SourceID] = struct{}{}
	}
	for index := range values {
		if values[index].Fact != nil {
			if _, duplicate := duplicates[values[index].Fact.SourceID]; duplicate {
				values[index] = MemberEventResult{Disposition: DispositionQuarantine, Reason: "audience_activity_source_id_ambiguous"}
			}
		}
	}
}
