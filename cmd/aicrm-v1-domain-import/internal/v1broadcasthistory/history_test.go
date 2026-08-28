package v1broadcasthistory

import (
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestAdaptHistoryPreservesOriginalFactsWithoutExecution(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.FixedZone("V1", 8*60*60))
	segment, campaign, job := int64(41), int64(42), int64(43)
	plans := []v1archive.ArchivedRow{broadcastRow(t, PlansTableID, 1, map[string]any{
		"id": 1, "plan_id": "plan-1", "trace_id": "trace", "session_id": "session", "operator": "operator", "intent": "intent", "segment_id": segment, "campaign_id": campaign,
		"selection_json": map[string]any{}, "content_strategy": "legacy", "content_template": "private body", "personalization_json": []any{}, "max_recipients": -5, "candidate_count": -4, "skipped_count": -3,
		"explanation_json": map[string]any{}, "variants_json": []any{}, "copy_workorder_run_ids": []any{}, "requires_manual_copy": true, "simulate_summary_json": map[string]any{}, "commit_batch_id": "batch", "commit_send_record_id": nil,
		"committed_at": nil, "committed_by": "", "approval_token_hash": "[REDACTED]", "status": "unknown_status", "error_message": "old error", "expires_at": nil, "created_at": at, "updated_at": at,
		"display_name": "private display", "owner_userid": "owner-1", "review_status": "legacy_review", "run_status": "legacy_run",
	}, "approval_token_hash")}
	recipients := []v1archive.ArchivedRow{broadcastRow(t, RecipientsTableID, 1, map[string]any{
		"id": 2, "plan_id": "plan-1", "owner_userid": "owner-1", "display_name": "private recipient", "planned_message_count": -2, "approval_status": "legacy_approval", "send_status": "unknown_after_dispatch",
		"approved_by": "approver", "approved_at": at, "rejected_by": "", "rejected_at": nil, "reject_reason": "", "broadcast_job_id": job, "last_error": "old error", "created_at": at, "updated_at": at, "unionid": "union-1",
	})}
	messages := []v1archive.ArchivedRow{broadcastRow(t, MessagesTableID, 1, map[string]any{
		"id": 3, "plan_id": "plan-1", "recipient_id": 2, "sequence_index": -1, "day_offset": -2, "send_time": "legacy-time", "content_text": "private body", "content_payload_json": map[string]any{"content_text": "private body"},
		"attachments_json": []any{}, "status": "failed_retryable", "sent_at": at, "last_error": "old error", "created_at": at, "updated_at": at, "unionid": "union-1",
	})}

	history := AdaptHistory(plans, recipients, messages)
	if len(history.Plans) != 1 || len(history.Recipients) != 1 || len(history.Messages) != 1 ||
		history.Plans[0].Disposition != DispositionCandidate || history.Recipients[0].Disposition != DispositionCandidate || history.Messages[0].Disposition != DispositionCandidate {
		t.Fatal("source_rows_not_conserved_as_history_candidates")
	}
	plan := history.Plans[0].Fact
	recipient := history.Recipients[0].Fact
	message := history.Messages[0].Fact
	if plan == nil || recipient == nil || message == nil || plan.SegmentSourceID == nil || *plan.SegmentSourceID != segment || plan.CampaignSourceID == nil || *plan.CampaignSourceID != campaign ||
		plan.OriginalStatus != "unknown_status" || plan.OriginalReviewStatus != "legacy_review" || plan.OriginalRunStatus != "legacy_run" || plan.MaxRecipients != -5 || plan.OwnerReference != ReferencePending ||
		recipient.BroadcastJobSourceID == nil || *recipient.BroadcastJobSourceID != job || recipient.OriginalSendStatus != "unknown_after_dispatch" || recipient.CustomerReference != ReferencePending || recipient.PlanReference != SourceParentObserved ||
		message.OriginalStatus != "failed_retryable" || message.SequenceIndex != -1 || message.DayOffset != -2 || message.PlanReference != SourceParentObserved || message.RecipientReference != SourceParentObserved || message.CustomerReference != ReferencePending ||
		!message.CreatedAt.Equal(at) || message.ContentText != "private body" {
		t.Fatalf("historical_fact_changed: plan=%#v recipient=%#v message=%#v", plan, recipient, message)
	}
}

func TestAdaptHistoryQuarantinesMissingOrMismatchedSourceParents(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 0, time.UTC)
	recipient := broadcastRow(t, RecipientsTableID, 1, map[string]any{
		"id": 2, "plan_id": "missing-plan", "owner_userid": "[REDACTED]", "display_name": "", "planned_message_count": 1, "approval_status": "pending", "send_status": "pending", "approved_by": "", "approved_at": nil,
		"rejected_by": "", "rejected_at": nil, "reject_reason": "", "broadcast_job_id": nil, "last_error": "", "created_at": at, "updated_at": at, "unionid": "[REDACTED]",
	}, "owner_userid", "unionid")
	message := broadcastRow(t, MessagesTableID, 1, map[string]any{
		"id": 3, "plan_id": "missing-plan", "recipient_id": 99, "sequence_index": 1, "day_offset": 0, "send_time": "", "content_text": "", "content_payload_json": map[string]any{}, "attachments_json": []any{},
		"status": "pending", "sent_at": nil, "last_error": "", "created_at": at, "updated_at": at, "unionid": "",
	})
	history := AdaptHistory(nil, []v1archive.ArchivedRow{recipient}, []v1archive.ArchivedRow{message})
	if history.Recipients[0].Disposition != DispositionQuarantine || history.Recipients[0].Reason != "broadcast_recipient_plan_unresolved" || history.Recipients[0].Fact != nil ||
		history.Messages[0].Disposition != DispositionQuarantine || history.Messages[0].Reason != "broadcast_message_plan_unresolved" || history.Messages[0].Fact != nil {
		t.Fatal("missing_source_parent_was_not_quarantined")
	}

	plans := []v1archive.ArchivedRow{broadcastRow(t, PlansTableID, 2, map[string]any{
		"id": 4, "plan_id": "plan-a", "trace_id": "", "session_id": "", "operator": "", "intent": "", "segment_id": nil, "campaign_id": nil,
		"selection_json": map[string]any{}, "content_strategy": "", "content_template": "", "personalization_json": map[string]any{}, "max_recipients": 0, "candidate_count": 0, "skipped_count": 0,
		"explanation_json": map[string]any{}, "variants_json": map[string]any{}, "copy_workorder_run_ids": map[string]any{}, "requires_manual_copy": false, "simulate_summary_json": map[string]any{}, "commit_batch_id": "", "commit_send_record_id": nil,
		"committed_at": nil, "committed_by": "", "approval_token_hash": "", "status": "", "error_message": "", "expires_at": nil, "created_at": at, "updated_at": at, "display_name": "", "owner_userid": "", "review_status": "", "run_status": "",
	}), broadcastRow(t, PlansTableID, 3, map[string]any{
		"id": 7, "plan_id": "plan-b", "trace_id": "", "session_id": "", "operator": "", "intent": "", "segment_id": nil, "campaign_id": nil,
		"selection_json": map[string]any{}, "content_strategy": "", "content_template": "", "personalization_json": map[string]any{}, "max_recipients": 0, "candidate_count": 0, "skipped_count": 0,
		"explanation_json": map[string]any{}, "variants_json": map[string]any{}, "copy_workorder_run_ids": map[string]any{}, "requires_manual_copy": false, "simulate_summary_json": map[string]any{}, "commit_batch_id": "", "commit_send_record_id": nil,
		"committed_at": nil, "committed_by": "", "approval_token_hash": "", "status": "", "error_message": "", "expires_at": nil, "created_at": at, "updated_at": at, "display_name": "", "owner_userid": "", "review_status": "", "run_status": "",
	})}
	recipients := []v1archive.ArchivedRow{broadcastRow(t, RecipientsTableID, 2, map[string]any{
		"id": 5, "plan_id": "plan-a", "owner_userid": "", "display_name": "", "planned_message_count": 0, "approval_status": "", "send_status": "", "approved_by": "", "approved_at": nil,
		"rejected_by": "", "rejected_at": nil, "reject_reason": "", "broadcast_job_id": nil, "last_error": "", "created_at": at, "updated_at": at, "unionid": "",
	})}
	mismatchedMessage := broadcastRow(t, MessagesTableID, 2, map[string]any{
		"id": 6, "plan_id": "plan-b", "recipient_id": 5, "sequence_index": 0, "day_offset": 0, "send_time": "", "content_text": "", "content_payload_json": map[string]any{}, "attachments_json": map[string]any{},
		"status": "", "sent_at": nil, "last_error": "", "created_at": at, "updated_at": at, "unionid": "",
	})
	history = AdaptHistory(plans, recipients, []v1archive.ArchivedRow{mismatchedMessage})
	if history.Messages[0].Disposition != DispositionQuarantine || history.Messages[0].Reason != "broadcast_message_recipient_plan_mismatch" || history.Messages[0].Fact != nil {
		t.Fatal("message_with_mismatched_recipient_was_not_quarantined")
	}
}

func TestAdaptHistoryQuarantinesInvalidRequiredShape(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 0, time.UTC)
	plan := broadcastRow(t, PlansTableID, 1, map[string]any{
		"id": 1, "plan_id": "", "trace_id": "", "session_id": "", "operator": "", "intent": "", "segment_id": nil, "campaign_id": nil, "selection_json": map[string]any{}, "content_strategy": "", "content_template": "", "personalization_json": []any{},
		"max_recipients": 0, "candidate_count": 0, "skipped_count": 0, "explanation_json": map[string]any{}, "variants_json": []any{}, "copy_workorder_run_ids": []any{}, "requires_manual_copy": false, "simulate_summary_json": map[string]any{}, "commit_batch_id": "", "commit_send_record_id": nil,
		"committed_at": nil, "committed_by": "", "approval_token_hash": "", "status": "draft", "error_message": "", "expires_at": nil, "created_at": at, "updated_at": at, "display_name": "", "owner_userid": "", "review_status": "pending_review", "run_status": "draft",
	})
	message := broadcastRow(t, MessagesTableID, 1, map[string]any{
		"id": 3, "plan_id": "plan", "recipient_id": 0, "sequence_index": 1, "day_offset": 0, "send_time": "", "content_text": "", "content_payload_json": map[string]any{}, "attachments_json": []any{}, "status": "pending", "sent_at": nil, "last_error": "", "created_at": at, "updated_at": at, "unionid": "",
	})
	history := AdaptHistory([]v1archive.ArchivedRow{plan}, nil, []v1archive.ArchivedRow{message})
	if history.Plans[0].Disposition != DispositionQuarantine || history.Plans[0].Reason != "broadcast_plan_shape_invalid" || history.Messages[0].Disposition != DispositionQuarantine || history.Messages[0].Reason != "broadcast_message_shape_invalid" {
		t.Fatalf("invalid_required_shape_accepted=%#v", history)
	}
}

func broadcastRow(t *testing.T, tableID string, ordinal int64, payload map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	key := strconv.FormatInt(ordinal, 10)
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal, Payload: encoded, RedactedFields: redacted,
		SourceKeyHMAC: sha256.Sum256([]byte(tableID + "/source/" + key)), PayloadHMAC: sha256.Sum256(encoded), FieldHMAC: sha256.Sum256([]byte(tableID + "/fields/" + key))}
}
