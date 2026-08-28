// Package v1broadcasthistory classifies sealed V1 cloud broadcast facts. It
// never creates a plan, recipient, message, queue item, or Provider action.
package v1broadcasthistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	PlansTableID      = "public/cloud_broadcast_plans"
	RecipientsTableID = "public/cloud_broadcast_plan_recipients"
	MessagesTableID   = "public/cloud_broadcast_plan_recipient_messages"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"

	ReferenceUnavailable HistoricalReference = "unavailable"
	ReferencePending     HistoricalReference = "requires_crosswalk"
	SourceParentObserved HistoricalReference = "source_parent_observed"
	SourceParentPending  HistoricalReference = "source_parent_pending"
)

// HistoricalReference only reports migration readiness; no source ID is ever
// treated as a V2 ID.
type HistoricalReference string

type PlanFact struct {
	SourceID                                                            int64
	SegmentSourceID, CampaignSourceID                                   *int64
	PlanID, TraceID, SessionID, Operator, Intent, ContentStrategy       string
	ContentTemplate                                                     string `json:"-"`
	MaxRecipients, CandidateCount, SkippedCount                         int64
	RequiresManualCopy                                                  bool
	CommitBatchID                                                       string
	CommitSendRecordSourceID                                            *int64
	CommittedAt                                                         *time.Time
	CommittedBy, OriginalStatus, ErrorMessage, DisplayName, OwnerUserID string `json:"-"`
	ExpiresAt                                                           *time.Time
	CreatedAt, UpdatedAt                                                time.Time
	SelectionDigest, PersonalizationDigest, ExplanationDigest           [sha256.Size]byte
	VariantsDigest, CopyWorkorderRunIDsDigest, SimulateSummaryDigest    [sha256.Size]byte
	OriginalReviewStatus, OriginalRunStatus                             string
	SegmentReference, CampaignReference, OwnerReference                 HistoricalReference
}

type RecipientFact struct {
	SourceID                                         int64
	BroadcastJobSourceID                             *int64
	PlanID, OwnerUserID, DisplayName                 string `json:"-"`
	PlannedMessageCount                              int64
	OriginalApprovalStatus, OriginalSendStatus       string
	ApprovedBy                                       string `json:"-"`
	ApprovedAt                                       *time.Time
	RejectedBy, RejectReason, LastError              string `json:"-"`
	RejectedAt                                       *time.Time
	CreatedAt, UpdatedAt                             time.Time
	UnionID                                          string `json:"-"`
	PlanReference, CustomerReference, OwnerReference HistoricalReference
}

type MessageFact struct {
	SourceID, RecipientSourceID             int64
	PlanID                                  string
	SequenceIndex, DayOffset                int64
	SendTime, ContentText                   string `json:"-"`
	OriginalStatus                          string
	SentAt                                  *time.Time
	LastError                               string `json:"-"`
	CreatedAt, UpdatedAt                    time.Time
	UnionID                                 string `json:"-"`
	ContentPayloadDigest, AttachmentsDigest [sha256.Size]byte
	PlanReference, RecipientReference       HistoricalReference
	CustomerReference                       HistoricalReference
}

type PlanResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PlanFact
}

type RecipientResult struct {
	Disposition Disposition
	Reason      string
	Fact        *RecipientFact
}

type MessageResult struct {
	Disposition Disposition
	Reason      string
	Fact        *MessageFact
}

type History struct {
	Plans      []PlanResult
	Recipients []RecipientResult
	Messages   []MessageResult
}

type planJSON struct {
	ID                  int64           `json:"id"`
	PlanID              string          `json:"plan_id"`
	TraceID             string          `json:"trace_id"`
	SessionID           string          `json:"session_id"`
	Operator            string          `json:"operator"`
	Intent              string          `json:"intent"`
	SegmentID           *int64          `json:"segment_id"`
	CampaignID          *int64          `json:"campaign_id"`
	SelectionJSON       json.RawMessage `json:"selection_json"`
	ContentStrategy     string          `json:"content_strategy"`
	ContentTemplate     string          `json:"content_template"`
	PersonalizationJSON json.RawMessage `json:"personalization_json"`
	MaxRecipients       int64           `json:"max_recipients"`
	CandidateCount      int64           `json:"candidate_count"`
	SkippedCount        int64           `json:"skipped_count"`
	ExplanationJSON     json.RawMessage `json:"explanation_json"`
	VariantsJSON        json.RawMessage `json:"variants_json"`
	CopyWorkorderRunIDs json.RawMessage `json:"copy_workorder_run_ids"`
	RequiresManualCopy  bool            `json:"requires_manual_copy"`
	SimulateSummaryJSON json.RawMessage `json:"simulate_summary_json"`
	CommitBatchID       string          `json:"commit_batch_id"`
	CommitSendRecordID  *int64          `json:"commit_send_record_id"`
	CommittedAt         *time.Time      `json:"committed_at"`
	CommittedBy         string          `json:"committed_by"`
	ApprovalTokenHash   string          `json:"approval_token_hash"`
	Status              string          `json:"status"`
	ErrorMessage        string          `json:"error_message"`
	ExpiresAt           *time.Time      `json:"expires_at"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	DisplayName         string          `json:"display_name"`
	OwnerUserID         string          `json:"owner_userid"`
	ReviewStatus        string          `json:"review_status"`
	RunStatus           string          `json:"run_status"`
}

type recipientJSON struct {
	ID                  int64      `json:"id"`
	PlanID              string     `json:"plan_id"`
	OwnerUserID         string     `json:"owner_userid"`
	DisplayName         string     `json:"display_name"`
	PlannedMessageCount int64      `json:"planned_message_count"`
	ApprovalStatus      string     `json:"approval_status"`
	SendStatus          string     `json:"send_status"`
	ApprovedBy          string     `json:"approved_by"`
	ApprovedAt          *time.Time `json:"approved_at"`
	RejectedBy          string     `json:"rejected_by"`
	RejectedAt          *time.Time `json:"rejected_at"`
	RejectReason        string     `json:"reject_reason"`
	BroadcastJobID      *int64     `json:"broadcast_job_id"`
	LastError           string     `json:"last_error"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	UnionID             string     `json:"unionid"`
}

type messageJSON struct {
	ID                 int64           `json:"id"`
	PlanID             string          `json:"plan_id"`
	RecipientID        int64           `json:"recipient_id"`
	SequenceIndex      int64           `json:"sequence_index"`
	DayOffset          int64           `json:"day_offset"`
	SendTime           string          `json:"send_time"`
	ContentText        string          `json:"content_text"`
	ContentPayloadJSON json.RawMessage `json:"content_payload_json"`
	AttachmentsJSON    json.RawMessage `json:"attachments_json"`
	Status             string          `json:"status"`
	SentAt             *time.Time      `json:"sent_at"`
	LastError          string          `json:"last_error"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	UnionID            string          `json:"unionid"`
}

// AdaptHistory preserves every input position. Source parent links are only
// observed within this frozen batch; V2 customer/staff/segment/campaign IDs
// remain pending for a future owner-owned crosswalk.
func AdaptHistory(plans, recipients, messages []v1archive.ArchivedRow) History {
	result := History{Plans: make([]PlanResult, len(plans)), Recipients: make([]RecipientResult, len(recipients)), Messages: make([]MessageResult, len(messages))}
	planCounts := map[string]int{}
	for index, row := range plans {
		result.Plans[index] = adaptPlan(row)
		if result.Plans[index].Fact != nil {
			planCounts[result.Plans[index].Fact.PlanID]++
		}
	}
	for index := range result.Plans {
		if result.Plans[index].Fact != nil && planCounts[result.Plans[index].Fact.PlanID] != 1 {
			result.Plans[index].Disposition, result.Plans[index].Reason = DispositionQuarantine, "broadcast_plan_id_ambiguous"
		}
	}
	knownPlans := map[string]struct{}{}
	for _, plan := range result.Plans {
		if plan.Disposition == DispositionCandidate && plan.Fact != nil {
			knownPlans[plan.Fact.PlanID] = struct{}{}
		}
	}
	recipientCounts := map[int64]int{}
	for index, row := range recipients {
		result.Recipients[index] = adaptRecipient(row)
		if result.Recipients[index].Fact != nil {
			recipientCounts[result.Recipients[index].Fact.SourceID]++
		}
	}
	knownRecipients := map[int64]RecipientFact{}
	for index := range result.Recipients {
		value := result.Recipients[index].Fact
		if value == nil || recipientCounts[value.SourceID] != 1 {
			if value != nil {
				result.Recipients[index].Disposition, result.Recipients[index].Reason = DispositionQuarantine, "broadcast_recipient_id_ambiguous"
			}
			continue
		}
		if _, found := knownPlans[value.PlanID]; found {
			value.PlanReference = SourceParentObserved
			knownRecipients[value.SourceID] = *value
			continue
		}
		// A recipient cannot be written without its immutable historical plan.
		// Keep the sealed source archive intact, but do not turn an absent V1
		// parent into a dangling formal relation.
		result.Recipients[index] = RecipientResult{Disposition: DispositionQuarantine, Reason: "broadcast_recipient_plan_unresolved"}
	}
	for index, row := range messages {
		result.Messages[index] = adaptMessage(row)
		value := result.Messages[index].Fact
		if value == nil || result.Messages[index].Disposition != DispositionCandidate {
			continue
		}
		if _, found := knownPlans[value.PlanID]; !found {
			result.Messages[index] = MessageResult{Disposition: DispositionQuarantine, Reason: "broadcast_message_plan_unresolved"}
			continue
		}
		recipient, found := knownRecipients[value.RecipientSourceID]
		if !found {
			result.Messages[index] = MessageResult{Disposition: DispositionQuarantine, Reason: "broadcast_message_recipient_unresolved"}
			continue
		}
		if recipient.PlanID != value.PlanID {
			result.Messages[index] = MessageResult{Disposition: DispositionQuarantine, Reason: "broadcast_message_recipient_plan_mismatch"}
			continue
		}
		value.PlanReference = SourceParentObserved
		value.RecipientReference = SourceParentObserved
	}
	return result
}

func adaptPlan(row v1archive.ArchivedRow) PlanResult {
	if !validRow(row, PlansTableID) {
		return PlanResult{Disposition: DispositionQuarantine, Reason: "broadcast_plan_archive_invalid"}
	}
	var source planJSON
	if !decode(row.Payload, &source, "id plan_id trace_id session_id operator intent selection_json content_strategy content_template personalization_json max_recipients candidate_count skipped_count explanation_json variants_json copy_workorder_run_ids requires_manual_copy simulate_summary_json commit_batch_id committed_by status error_message created_at updated_at display_name owner_userid review_status run_status") ||
		source.ID < 1 || source.PlanID == "" || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || !validJSON(source.SelectionJSON, source.PersonalizationJSON, source.ExplanationJSON, source.VariantsJSON, source.CopyWorkorderRunIDs, source.SimulateSummaryJSON) {
		return PlanResult{Disposition: DispositionQuarantine, Reason: "broadcast_plan_shape_invalid"}
	}
	fact := PlanFact{SourceID: source.ID, SegmentSourceID: cloneID(source.SegmentID), CampaignSourceID: cloneID(source.CampaignID), PlanID: source.PlanID, TraceID: source.TraceID, SessionID: source.SessionID, Operator: source.Operator, Intent: source.Intent,
		ContentStrategy: source.ContentStrategy, ContentTemplate: source.ContentTemplate, MaxRecipients: source.MaxRecipients, CandidateCount: source.CandidateCount, SkippedCount: source.SkippedCount,
		RequiresManualCopy: source.RequiresManualCopy, CommitBatchID: source.CommitBatchID, CommitSendRecordSourceID: cloneID(source.CommitSendRecordID), CommittedAt: cloneTime(source.CommittedAt),
		CommittedBy: source.CommittedBy, OriginalStatus: source.Status, ErrorMessage: source.ErrorMessage, ExpiresAt: cloneTime(source.ExpiresAt), CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		DisplayName: source.DisplayName, OwnerUserID: optionalText(row, "owner_userid", source.OwnerUserID), OriginalReviewStatus: source.ReviewStatus, OriginalRunStatus: source.RunStatus,
		SelectionDigest: sha256.Sum256(source.SelectionJSON), PersonalizationDigest: sha256.Sum256(source.PersonalizationJSON), ExplanationDigest: sha256.Sum256(source.ExplanationJSON),
		VariantsDigest: sha256.Sum256(source.VariantsJSON), CopyWorkorderRunIDsDigest: sha256.Sum256(source.CopyWorkorderRunIDs), SimulateSummaryDigest: sha256.Sum256(source.SimulateSummaryJSON),
		SegmentReference: optionalIDReference(source.SegmentID), CampaignReference: optionalIDReference(source.CampaignID), OwnerReference: optionalTextReference(source.OwnerUserID)}
	return PlanResult{Disposition: DispositionCandidate, Fact: &fact}
}

func adaptRecipient(row v1archive.ArchivedRow) RecipientResult {
	if !validRow(row, RecipientsTableID) {
		return RecipientResult{Disposition: DispositionQuarantine, Reason: "broadcast_recipient_archive_invalid"}
	}
	var source recipientJSON
	if !decode(row.Payload, &source, "id plan_id owner_userid display_name planned_message_count approval_status send_status approved_by rejected_by reject_reason last_error created_at updated_at unionid") ||
		source.ID < 1 || source.PlanID == "" || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
		return RecipientResult{Disposition: DispositionQuarantine, Reason: "broadcast_recipient_shape_invalid"}
	}
	fact := RecipientFact{SourceID: source.ID, PlanID: source.PlanID, OwnerUserID: optionalText(row, "owner_userid", source.OwnerUserID), DisplayName: source.DisplayName,
		PlannedMessageCount: source.PlannedMessageCount, OriginalApprovalStatus: source.ApprovalStatus, OriginalSendStatus: source.SendStatus, ApprovedBy: source.ApprovedBy,
		ApprovedAt: cloneTime(source.ApprovedAt), RejectedBy: source.RejectedBy, RejectedAt: cloneTime(source.RejectedAt), RejectReason: source.RejectReason,
		LastError: source.LastError, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, UnionID: optionalText(row, "unionid", source.UnionID),
		PlanReference: SourceParentPending, CustomerReference: optionalTextReference(source.UnionID), OwnerReference: optionalTextReference(source.OwnerUserID)}
	fact.BroadcastJobSourceID = cloneID(source.BroadcastJobID)
	return RecipientResult{Disposition: DispositionCandidate, Fact: &fact}
}

func adaptMessage(row v1archive.ArchivedRow) MessageResult {
	if !validRow(row, MessagesTableID) {
		return MessageResult{Disposition: DispositionQuarantine, Reason: "broadcast_message_archive_invalid"}
	}
	var source messageJSON
	if !decode(row.Payload, &source, "id plan_id recipient_id sequence_index day_offset send_time content_text content_payload_json attachments_json status last_error created_at updated_at unionid") ||
		source.ID < 1 || source.RecipientID < 1 || source.PlanID == "" || source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() || !validJSON(source.ContentPayloadJSON, source.AttachmentsJSON) {
		return MessageResult{Disposition: DispositionQuarantine, Reason: "broadcast_message_shape_invalid"}
	}
	fact := MessageFact{SourceID: source.ID, RecipientSourceID: source.RecipientID, PlanID: source.PlanID, SequenceIndex: source.SequenceIndex, DayOffset: source.DayOffset,
		SendTime: source.SendTime, ContentText: source.ContentText, OriginalStatus: source.Status, SentAt: cloneTime(source.SentAt), LastError: source.LastError,
		CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt, UnionID: optionalText(row, "unionid", source.UnionID), ContentPayloadDigest: sha256.Sum256(source.ContentPayloadJSON),
		AttachmentsDigest: sha256.Sum256(source.AttachmentsJSON), PlanReference: SourceParentPending, RecipientReference: SourceParentPending, CustomerReference: optionalTextReference(source.UnionID)}
	return MessageResult{Disposition: DispositionCandidate, Fact: &fact}
}

func validRow(row v1archive.ArchivedRow, tableID string) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal > 0 && row.SourceKeyHMAC != ([sha256.Size]byte{}) &&
		row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func decode(payload []byte, target any, required string) bool {
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for _, field := range strings.Fields(required) {
		value, found := fields[field]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return json.Unmarshal(payload, target) == nil
}

func validJSON(values ...json.RawMessage) bool {
	for _, value := range values {
		if !json.Valid(value) || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

func optionalIDReference(value *int64) HistoricalReference {
	if value == nil {
		return ReferenceUnavailable
	}
	return ReferencePending
}

func optionalTextReference(value string) HistoricalReference {
	if value == "" || value == "[REDACTED]" {
		return ReferenceUnavailable
	}
	return ReferencePending
}

func optionalText(row v1archive.ArchivedRow, field, value string) string {
	if v1archive.IsRedacted(row, field) || value == "[REDACTED]" {
		return ""
	}
	return value
}

func cloneID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
