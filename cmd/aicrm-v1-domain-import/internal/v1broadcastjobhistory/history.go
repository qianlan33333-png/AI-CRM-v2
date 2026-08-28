// Package v1broadcastjobhistory classifies sealed V1 broadcast queue rows as
// inert Outbound history. It cannot enqueue, retry, send, or reconcile a V2
// outbound task.
package v1broadcastjobhistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const BroadcastJobsTableID = "public/broadcast_jobs"

var ErrInvalidArchiveRow = errors.New("invalid V1 broadcast job history archive row")

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

const (
	ReasonInvalidArchiveRow    = "invalid_archive_row"
	ReasonUnknownRedactedField = "unknown_redacted_field"
	ReasonRequiredRedacted     = "required_field_redacted"
	ReasonInvalidSourcePayload = "invalid_source_payload"
	ReasonInvalidBroadcastJob  = "invalid_broadcast_job"
	ReasonDuplicateSourceID    = "duplicate_source_id"
)

// OpaqueDigest records only archive material that this candidate must not
// expose or use as a V2 reference. It is a digest of the archive field bytes;
// when that field was redacted it is deliberately not a digest of the original
// V1 value.
type OpaqueDigest [sha256.Size]byte

// BroadcastJobHistory retains all 53 manifest columns as an inert historical
// observation. Source references are never V2 foreign keys. In particular,
// legacy target unionids, job IDs, execution IDs, and Provider flags cannot
// establish a V2 customer, task, receipt, or external success.
type BroadcastJobHistory struct {
	SourceID                  int64
	OriginalSourceType        string
	SourceReferenceDigest     OpaqueDigest
	SourceTable               string
	ScheduledFor              time.Time
	Priority                  int32
	BatchKeyDigest            OpaqueDigest
	OriginalStatus            string
	RequiresApproval          bool
	ApprovedByDigest          OpaqueDigest
	ApprovedAt                *time.Time
	CancelledByDigest         OpaqueDigest
	CancelledAt               *time.Time
	CancelReasonDigest        OpaqueDigest
	TargetCount               int32
	TargetSummaryDigest       OpaqueDigest
	ContentType               string
	ContentPayloadDigest      OpaqueDigest
	ContentSummaryDigest      OpaqueDigest
	AttemptCount              int32
	LastErrorDigest           OpaqueDigest
	LegacyOutboundTaskID      *int64
	SentCount                 int32
	FailedCount               int32
	TraceIDDigest             OpaqueDigest
	CreatedByDigest           OpaqueDigest
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	ClaimedAt                 *time.Time
	SentAt                    *time.Time
	ClaimTokenDigest          OpaqueDigest
	LeaseExpiresAt            *time.Time
	BusinessDomain            *string
	IdempotencyKeyDigest      *OpaqueDigest
	Channel                   *string
	TargetKind                *string
	FailureType               *string
	RetryPolicyDigest         OpaqueDigest
	MetadataDigest            OpaqueDigest
	TargetUnionIDsDigest      OpaqueDigest
	MaxAttempts               int32
	NextRetryAt               *time.Time
	DispatchStartedAt         *time.Time
	SideEffectExecuted        bool
	ProviderResultReceived    bool
	ResultSummaryDigest       OpaqueDigest
	ReconciliationRequired    bool
	CompletedAt               *time.Time
	HoldReasonDigest          OpaqueDigest
	HoldAt                    *time.Time
	LegacyExternalEffectJobID *int64
	ExecutionIDDigest         OpaqueDigest
	ExecutionOwnerDigest      OpaqueDigest

	SourceKeyDigest     [sha256.Size]byte
	SourcePayloadDigest [sha256.Size]byte
	ArchiveFieldDigest  [sha256.Size]byte
	RedactedRoots       []string
}

type Result struct {
	Disposition Disposition
	Reason      string
	Fact        *BroadcastJobHistory
}

// ArchiveReader is deliberately read-only and is satisfied by the immutable
// V2 archive reader. It cannot call an Outbound writer.
type ArchiveReader interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

type PreflightReport struct {
	SourceRows, Candidates, Quarantined int
	Reasons, RedactedRoots              map[string]int
}

func (report PreflightReport) SortedReasons() []string { return sortedKeys(report.Reasons) }
func (report PreflightReport) SortedRedactedRoots() []string {
	return sortedKeys(report.RedactedRoots)
}

type sourceJSON struct {
	ID                     int64           `json:"id"`
	SourceType             string          `json:"source_type"`
	SourceID               string          `json:"source_id"`
	SourceTable            string          `json:"source_table"`
	ScheduledFor           time.Time       `json:"scheduled_for"`
	Priority               int32           `json:"priority"`
	BatchKey               string          `json:"batch_key"`
	Status                 string          `json:"status"`
	RequiresApproval       bool            `json:"requires_approval"`
	ApprovedBy             string          `json:"approved_by"`
	ApprovedAt             *time.Time      `json:"approved_at"`
	CancelledBy            string          `json:"cancelled_by"`
	CancelledAt            *time.Time      `json:"cancelled_at"`
	CancelReason           string          `json:"cancel_reason"`
	TargetCount            int32           `json:"target_count"`
	TargetSummary          string          `json:"target_summary"`
	ContentType            string          `json:"content_type"`
	ContentPayload         json.RawMessage `json:"content_payload"`
	ContentSummary         string          `json:"content_summary"`
	AttemptCount           int32           `json:"attempt_count"`
	LastError              string          `json:"last_error"`
	OutboundTaskID         *int64          `json:"outbound_task_id"`
	SentCount              int32           `json:"sent_count"`
	FailedCount            int32           `json:"failed_count"`
	TraceID                string          `json:"trace_id"`
	CreatedBy              string          `json:"created_by"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
	ClaimedAt              *time.Time      `json:"claimed_at"`
	SentAt                 *time.Time      `json:"sent_at"`
	ClaimToken             string          `json:"claim_token"`
	LeaseExpiresAt         *time.Time      `json:"lease_expires_at"`
	BusinessDomain         *string         `json:"business_domain"`
	IdempotencyKey         *string         `json:"idempotency_key"`
	Channel                *string         `json:"channel"`
	TargetKind             *string         `json:"target_kind"`
	FailureType            *string         `json:"failure_type"`
	RetryPolicy            json.RawMessage `json:"retry_policy_json"`
	Metadata               json.RawMessage `json:"metadata_json"`
	TargetUnionIDs         json.RawMessage `json:"target_unionids_json"`
	MaxAttempts            int32           `json:"max_attempts"`
	NextRetryAt            *time.Time      `json:"next_retry_at"`
	DispatchStartedAt      *time.Time      `json:"dispatch_started_at"`
	SideEffectExecuted     bool            `json:"side_effect_executed"`
	ProviderResultReceived bool            `json:"provider_result_received"`
	ResultSummary          json.RawMessage `json:"result_summary_json"`
	ReconciliationRequired bool            `json:"reconciliation_required"`
	CompletedAt            *time.Time      `json:"completed_at"`
	HoldReason             string          `json:"hold_reason"`
	HoldAt                 *time.Time      `json:"hold_at"`
	ExternalEffectJobID    *int64          `json:"external_effect_job_id"`
	ExecutionID            string          `json:"execution_id"`
	ExecutionOwner         string          `json:"execution_owner"`
}

var manifestFields = []string{
	"id", "source_type", "source_id", "source_table", "scheduled_for", "priority", "batch_key", "status", "requires_approval", "approved_by", "approved_at", "cancelled_by", "cancelled_at", "cancel_reason", "target_count", "target_summary", "content_type", "content_payload", "content_summary", "attempt_count", "last_error", "outbound_task_id", "sent_count", "failed_count", "trace_id", "created_by", "created_at", "updated_at", "claimed_at", "sent_at", "claim_token", "lease_expires_at", "business_domain", "idempotency_key", "channel", "target_kind", "failure_type", "retry_policy_json", "metadata_json", "target_unionids_json", "max_attempts", "next_retry_at", "dispatch_started_at", "side_effect_executed", "provider_result_received", "result_summary_json", "reconciliation_required", "completed_at", "hold_reason", "hold_at", "external_effect_job_id", "execution_id", "execution_owner",
}

var nullableFields = map[string]bool{
	"approved_at": true, "cancelled_at": true, "outbound_task_id": true, "claimed_at": true, "sent_at": true, "lease_expires_at": true, "business_domain": true, "idempotency_key": true, "channel": true, "target_kind": true, "failure_type": true, "next_retry_at": true, "dispatch_started_at": true, "completed_at": true, "hold_at": true, "external_effect_job_id": true,
}

// Only opaque material may be retained as a digest after archive redaction.
// Other redacted roots make an observed historical fact incomplete.
var opaqueRoots = map[string]bool{
	"source_id": true, "batch_key": true, "approved_by": true, "cancelled_by": true, "cancel_reason": true, "target_summary": true, "content_payload": true, "content_summary": true, "last_error": true, "trace_id": true, "created_by": true, "claim_token": true, "idempotency_key": true, "retry_policy_json": true, "metadata_json": true, "target_unionids_json": true, "result_summary_json": true, "hold_reason": true, "execution_id": true, "execution_owner": true,
}

// AdaptHistory validates an immutable archive envelope and returns a purely
// historical fact. It does not translate V1 status or booleans into V2 state.
func AdaptHistory(row v1archive.ArchivedRow) Result {
	if !validRow(row) {
		return quarantine(ReasonInvalidArchiveRow)
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(row.Payload, &fields) != nil || fields == nil || !hasManifestShape(fields) {
		return quarantine(ReasonInvalidSourcePayload)
	}
	roots, valid := redactedRoots(row.RedactedFields)
	if !valid {
		return quarantine(ReasonUnknownRedactedField)
	}
	for root := range roots {
		if !opaqueRoots[root] {
			return quarantine(ReasonRequiredRedacted)
		}
	}
	var source sourceJSON
	if json.Unmarshal(row.Payload, &source) != nil || source.ID < 1 || source.ScheduledFor.IsZero() || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || !validJSON(source.ContentPayload, source.RetryPolicy, source.Metadata, source.TargetUnionIDs, source.ResultSummary) {
		return quarantine(ReasonInvalidBroadcastJob)
	}
	fact := BroadcastJobHistory{
		SourceID: source.ID, OriginalSourceType: source.SourceType, SourceReferenceDigest: digestField(fields["source_id"]), SourceTable: source.SourceTable,
		ScheduledFor: utcMicro(source.ScheduledFor), Priority: source.Priority, BatchKeyDigest: digestField(fields["batch_key"]), OriginalStatus: source.Status,
		RequiresApproval: source.RequiresApproval, ApprovedByDigest: digestField(fields["approved_by"]), ApprovedAt: utcMicroPtr(source.ApprovedAt),
		CancelledByDigest: digestField(fields["cancelled_by"]), CancelledAt: utcMicroPtr(source.CancelledAt), CancelReasonDigest: digestField(fields["cancel_reason"]),
		TargetCount: source.TargetCount, TargetSummaryDigest: digestField(fields["target_summary"]), ContentType: source.ContentType, ContentPayloadDigest: digestField(fields["content_payload"]),
		ContentSummaryDigest: digestField(fields["content_summary"]), AttemptCount: source.AttemptCount, LastErrorDigest: digestField(fields["last_error"]), LegacyOutboundTaskID: cloneID(source.OutboundTaskID),
		SentCount: source.SentCount, FailedCount: source.FailedCount, TraceIDDigest: digestField(fields["trace_id"]), CreatedByDigest: digestField(fields["created_by"]),
		CreatedAt: utcMicro(source.CreatedAt), UpdatedAt: utcMicro(source.UpdatedAt), ClaimedAt: utcMicroPtr(source.ClaimedAt), SentAt: utcMicroPtr(source.SentAt),
		ClaimTokenDigest: digestField(fields["claim_token"]), LeaseExpiresAt: utcMicroPtr(source.LeaseExpiresAt), BusinessDomain: cloneText(source.BusinessDomain),
		Channel: cloneText(source.Channel), TargetKind: cloneText(source.TargetKind), FailureType: cloneText(source.FailureType), RetryPolicyDigest: digestField(fields["retry_policy_json"]),
		MetadataDigest: digestField(fields["metadata_json"]), TargetUnionIDsDigest: digestField(fields["target_unionids_json"]), MaxAttempts: source.MaxAttempts,
		NextRetryAt: utcMicroPtr(source.NextRetryAt), DispatchStartedAt: utcMicroPtr(source.DispatchStartedAt), SideEffectExecuted: source.SideEffectExecuted,
		ProviderResultReceived: source.ProviderResultReceived, ResultSummaryDigest: digestField(fields["result_summary_json"]), ReconciliationRequired: source.ReconciliationRequired,
		CompletedAt: utcMicroPtr(source.CompletedAt), HoldReasonDigest: digestField(fields["hold_reason"]), HoldAt: utcMicroPtr(source.HoldAt), LegacyExternalEffectJobID: cloneID(source.ExternalEffectJobID),
		ExecutionIDDigest: digestField(fields["execution_id"]), ExecutionOwnerDigest: digestField(fields["execution_owner"]), SourceKeyDigest: row.SourceKeyHMAC,
		SourcePayloadDigest: row.PayloadHMAC, ArchiveFieldDigest: row.FieldHMAC, RedactedRoots: sortedKeys(roots),
	}
	if source.IdempotencyKey != nil {
		digest := digestField(fields["idempotency_key"])
		fact.IdempotencyKeyDigest = &digest
	}
	return Result{Disposition: DispositionCandidate, Fact: &fact}
}

// Preflight streams one reconciled V2 archive table. It returns only aggregate
// counts and known manifest root names; it cannot open or query V1.
func Preflight(ctx context.Context, archive ArchiveReader, runID string) (PreflightReport, error) {
	if archive == nil || strings.TrimSpace(runID) == "" {
		return PreflightReport{}, ErrInvalidArchiveRow
	}
	report := PreflightReport{Reasons: map[string]int{}, RedactedRoots: map[string]int{}}
	seen := map[[sha256.Size]byte]bool{}
	var ordinal int64
	err := archive.EachTableRow(ctx, runID, BroadcastJobsTableID, func(row v1archive.ArchivedRow) error {
		ordinal++
		if row.SourceOrdinal != ordinal || seen[row.SourceKeyHMAC] {
			return ErrInvalidArchiveRow
		}
		seen[row.SourceKeyHMAC] = true
		report.SourceRows++
		for _, path := range row.RedactedFields {
			root := redactedRoot(path)
			if !contains(manifestFields, root) {
				root = "unknown"
			}
			report.RedactedRoots[root]++
		}
		result := AdaptHistory(row)
		if result.Disposition == DispositionCandidate {
			report.Candidates++
			return nil
		}
		report.Quarantined++
		report.Reasons[result.Reason]++
		return nil
	})
	if err != nil {
		return PreflightReport{}, err
	}
	return report, nil
}

func validRow(row v1archive.ArchivedRow) bool {
	zero := [sha256.Size]byte{}
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == BroadcastJobsTableID && row.SourceOrdinal > 0 &&
		row.SourceKeyHMAC != zero && row.PayloadHMAC != zero && row.FieldHMAC != zero && json.Valid(row.Payload)
}

func hasManifestShape(fields map[string]json.RawMessage) bool {
	for _, field := range manifestFields {
		value, found := fields[field]
		if !found || (!nullableFields[field] && isNull(value)) {
			return false
		}
	}
	return true
}

func redactedRoots(paths []string) (map[string]bool, bool) {
	roots := map[string]bool{}
	for _, path := range paths {
		root := path
		if index := strings.IndexAny(root, ".["); index >= 0 {
			root = root[:index]
		}
		if !contains(manifestFields, root) {
			return nil, false
		}
		roots[root] = true
	}
	return roots, true
}

func validJSON(values ...json.RawMessage) bool {
	for _, value := range values {
		if isNull(value) || !json.Valid(value) {
			return false
		}
	}
	return true
}

func isNull(value json.RawMessage) bool              { return strings.TrimSpace(string(value)) == "null" }
func digestField(value json.RawMessage) OpaqueDigest { return OpaqueDigest(sha256.Sum256(value)) }
func utcMicro(value time.Time) time.Time             { return value.UTC().Truncate(time.Microsecond) }
func utcMicroPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := utcMicro(*value)
	return &copy
}
func cloneID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneText(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func quarantine(reason string) Result {
	return Result{Disposition: DispositionQuarantine, Reason: reason}
}
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func redactedRoot(path string) string {
	if index := strings.IndexAny(path, ".["); index >= 0 {
		return path[:index]
	}
	return path
}
