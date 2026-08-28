package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestCampaignHistoryImporterWritesOnlyHistoryAndReplaysWithSameTransaction(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.FixedZone("V1", 8*60*60))
	archive := &campaignHistoryArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		campaignHistoryContextTable: {campaignHistoryRow(t, campaignHistoryContextTable, 1, map[string]any{
			"id": 101, "campaign_code": "campaign", "display_name": "", "intent": "", "anchor_mode": "", "anchor_date": "", "review_status": "", "run_status": "", "created_by_agent": "", "created_by_session": "", "trace_id": "", "owner_userid": "", "approved_by": "", "paused_reason": "", "approved_at": nil, "started_at": nil, "finished_at": nil, "paused_at": nil, "created_at": stamp, "updated_at": stamp,
		})},
		campaignHistorySegmentsTable: {campaignHistoryRow(t, campaignHistorySegmentsTable, 1, map[string]any{
			"id": 201, "campaign_id": 101, "segment_id": 301, "segment_code": "seg", "priority": -1, "label": "label", "created_at": stamp,
		})},
		campaignHistoryMembersTable: {campaignHistoryRow(t, campaignHistoryMembersTable, 1, map[string]any{
			"id": 401, "campaign_id": 101, "campaign_segment_id": 201, "segment_id": 301, "member_id": 501, "unionid": "union", "joined_at": stamp, "anchor_date": "", "current_step_index": -2, "next_due_at": nil, "status": "old", "stop_reason": "", "last_step_sent_at": nil, "retry_count": -3, "trace_id": "", "last_error_text": "", "created_at": stamp, "updated_at": stamp,
		})},
		campaignHistoryPlansTable: {campaignHistoryRow(t, campaignHistoryPlansTable, 1, broadcastPlanPayload(stamp))},
		campaignHistoryRecipientsTable: {campaignHistoryRow(t, campaignHistoryRecipientsTable, 1, map[string]any{
			"id": 601, "plan_id": "plan", "owner_userid": "owner", "display_name": "recipient", "planned_message_count": -1, "approval_status": "old", "send_status": "old", "approved_by": "", "approved_at": nil, "rejected_by": "", "rejected_at": nil, "reject_reason": "", "broadcast_job_id": nil, "last_error": "", "created_at": stamp, "updated_at": stamp, "unionid": "union",
		})},
		campaignHistoryMessagesTable: {campaignHistoryRow(t, campaignHistoryMessagesTable, 1, map[string]any{
			"id": 701, "plan_id": "plan", "recipient_id": 601, "sequence_index": -1, "day_offset": -2, "send_time": "09:00", "content_text": " +8613800138000\n", "content_payload_json": map[string]any{}, "attachments_json": []any{}, "status": "old", "sent_at": nil, "last_error": "", "created_at": stamp, "updated_at": stamp, "unionid": "union",
		})},
	}}
	terminals := campaignHistoryImportTerminals()
	writer := &campaignHistoryWriterFake{journals: terminals, targets: map[string]int64{}}
	resolver := &campaignHistoryResolverFake{customerID: 9}
	importer, err := newCampaignHistoryImporter(archive, campaignHistoryTxUOW{}, writer, resolver, terminals, "archive")
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		result, err := importer.Import(context.Background(), "archive")
		if err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		for _, tableID := range campaignHistorySourceTables() {
			value := result.Tables[tableID]
			if value.Imported != 1 || value.Quarantined != 0 || value.Replayed != pass {
				t.Fatalf("pass %d table %s=%#v", pass, tableID, value)
			}
		}
	}
	if writer.calls != 5 || resolver.calls != 6 || !writer.sameTransaction || !resolver.sameTransaction {
		t.Fatalf("writer/resolver calls=%d/%d tx=%v/%v", writer.calls, resolver.calls, writer.sameTransaction, resolver.sameTransaction)
	}
	if writer.message.ContentMasked != " [masked-phone]\n" || writer.message.PlanHistoryID < 1 || writer.message.RecipientHistoryID < 1 || writer.member.SegmentHistoryID < 1 {
		t.Fatalf("historical parents or mask incorrect: message=%#v member=%#v", writer.message, writer.member)
	}
	if writer.message.CustomerID == nil || *writer.message.CustomerID != 9 || writer.plan.RuntimeDigest == ([sha256.Size]byte{}) {
		t.Fatalf("customer/runtime evidence lost: %#v %#v", writer.message.CustomerID, writer.plan.RuntimeDigest)
	}
}

func TestCampaignHistoryImporterQuarantinesMissingMemberParentWithoutWriting(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	archive := &campaignHistoryArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		campaignHistoryContextTable: {campaignHistoryRow(t, campaignHistoryContextTable, 1, map[string]any{
			"id": 101, "campaign_code": "campaign", "display_name": "", "intent": "", "anchor_mode": "", "anchor_date": "", "review_status": "", "run_status": "", "created_by_agent": "", "created_by_session": "", "trace_id": "", "owner_userid": "", "approved_by": "", "paused_reason": "", "approved_at": nil, "started_at": nil, "finished_at": nil, "paused_at": nil, "created_at": stamp, "updated_at": stamp,
		})},
		campaignHistorySegmentsTable: {},
		campaignHistoryMembersTable: {campaignHistoryRow(t, campaignHistoryMembersTable, 1, map[string]any{
			"id": 401, "campaign_id": 101, "campaign_segment_id": 201, "segment_id": 301, "member_id": 501, "unionid": "", "joined_at": stamp, "anchor_date": "", "current_step_index": 0, "next_due_at": nil, "status": "", "stop_reason": "", "last_step_sent_at": nil, "retry_count": 0, "trace_id": "", "last_error_text": "", "created_at": stamp, "updated_at": stamp,
		})},
		campaignHistoryPlansTable: {}, campaignHistoryRecipientsTable: {}, campaignHistoryMessagesTable: {},
	}}
	terminals := campaignHistoryImportTerminals()
	writer := &campaignHistoryWriterFake{journals: terminals, targets: map[string]int64{}}
	importer, err := newCampaignHistoryImporter(archive, campaignHistoryTxUOW{}, writer, &campaignHistoryResolverFake{}, terminals, "archive")
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive")
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 0 || result.Tables[campaignHistoryMembersTable].Quarantined != 1 {
		t.Fatalf("unresolved member was written: calls=%d result=%#v", writer.calls, result)
	}
	terminal := terminals[campaignHistoryMembersTable].(*campaignHistoryImportTerminal).values[SourceIdentifier(archive.rows[campaignHistoryMembersTable][0].SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != "member_campaign_segment_unresolved" {
		t.Fatalf("unexpected unresolved terminal=%#v", terminal)
	}
}

func TestNewCampaignHistoryImporterAcceptsExactTableKeyedJournals(t *testing.T) {
	journals := make(map[string]*Journal, len(campaignHistoryScopes))
	for kind, scope := range campaignHistoryScopes {
		journals[scope[0]] = campaignHistoryScopedJournal(kind, "archive")
	}
	if _, err := NewCampaignHistoryImporter(&campaignHistoryArchiveFake{}, campaignHistoryTxUOW{}, &campaignHistoryWriterFake{}, &campaignHistoryResolverFake{}, journals); err != nil {
		t.Fatalf("table keyed journals rejected: %v", err)
	}
	wrongKeys := make(map[string]*Journal, len(campaignHistoryScopes))
	for kind := range campaignHistoryScopes {
		wrongKeys[kind] = campaignHistoryScopedJournal(kind, "archive")
	}
	if _, err := NewCampaignHistoryImporter(&campaignHistoryArchiveFake{}, campaignHistoryTxUOW{}, &campaignHistoryWriterFake{}, &campaignHistoryResolverFake{}, wrongKeys); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("owner kind map accepted as source table map: %v", err)
	}
}

type campaignHistoryArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *campaignHistoryArchiveFake) EachTableRow(_ context.Context, _ string, tableID string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[tableID] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type campaignHistoryTxKey struct{}

type campaignHistoryTxUOW struct{}

func (campaignHistoryTxUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, campaignHistoryTxKey{}, "tx"))
}

type campaignHistoryImportTerminal struct{ values map[string]TerminalReceipt }

func campaignHistoryImportTerminals() map[string]campaignHistoryTerminalJournal {
	result := make(map[string]campaignHistoryTerminalJournal, len(campaignHistoryScopes))
	for _, scope := range campaignHistoryScopes {
		result[scope[0]] = &campaignHistoryImportTerminal{values: map[string]TerminalReceipt{}}
	}
	return result
}

func (journal *campaignHistoryImportTerminal) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	value, found := journal.values[source]
	return value, found, nil
}

func (journal *campaignHistoryImportTerminal) Record(_ context.Context, receipt TerminalReceipt) error {
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := journal.values[source]; found && !sameCampaignHistoryTerminal(old, receipt) {
		return ErrConflict
	}
	journal.values[source] = receipt
	return nil
}

type campaignHistoryWriterFake struct {
	journals        map[string]campaignHistoryTerminalJournal
	targets         map[string]int64
	calls           int
	sameTransaction bool
	member          campaignport.HistoricalCampaignMember
	plan            campaignport.HistoricalBroadcastPlan
	message         campaignport.HistoricalBroadcastMessage
}

func (writer *campaignHistoryWriterFake) WriteSegment(ctx context.Context, source string, digest [sha256.Size]byte, value campaignport.HistoricalCampaignSegment) (campaignport.CampaignHistoryReceipt, error) {
	return writer.write(ctx, campaignHistorySegmentsTable, source, digest, value.SourceID, func() { writer.calls++ })
}

func (writer *campaignHistoryWriterFake) WriteMember(ctx context.Context, source string, digest [sha256.Size]byte, value campaignport.HistoricalCampaignMember) (campaignport.CampaignHistoryReceipt, error) {
	writer.member = value
	return writer.write(ctx, campaignHistoryMembersTable, source, digest, value.SourceID, func() { writer.calls++ })
}

func (writer *campaignHistoryWriterFake) WritePlan(ctx context.Context, source string, digest [sha256.Size]byte, value campaignport.HistoricalBroadcastPlan) (campaignport.CampaignHistoryReceipt, error) {
	writer.plan = value
	return writer.write(ctx, campaignHistoryPlansTable, source, digest, value.SourceID, func() { writer.calls++ })
}

func (writer *campaignHistoryWriterFake) WriteRecipient(ctx context.Context, source string, digest [sha256.Size]byte, value campaignport.HistoricalBroadcastRecipient) (campaignport.CampaignHistoryReceipt, error) {
	return writer.write(ctx, campaignHistoryRecipientsTable, source, digest, value.SourceID, func() { writer.calls++ })
}

func (writer *campaignHistoryWriterFake) WriteMessage(ctx context.Context, source string, digest [sha256.Size]byte, value campaignport.HistoricalBroadcastMessage) (campaignport.CampaignHistoryReceipt, error) {
	writer.message = value
	return writer.write(ctx, campaignHistoryMessagesTable, source, digest, value.SourceID, func() { writer.calls++ })
}

func (writer *campaignHistoryWriterFake) write(ctx context.Context, tableID, source string, payload [sha256.Size]byte, sourceID int64, onCreate func()) (campaignport.CampaignHistoryReceipt, error) {
	if ctx.Value(campaignHistoryTxKey{}) != "tx" {
		writer.sameTransaction = false
		return campaignport.CampaignHistoryReceipt{}, errors.New("missing tx context")
	}
	writer.sameTransaction = true
	journal := writer.journals[tableID]
	terminal, found, err := journal.LoadTerminal(ctx, source)
	if err != nil {
		return campaignport.CampaignHistoryReceipt{}, err
	}
	if found {
		if terminal.PayloadDigest != payload || terminal.Disposition != "import" {
			return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryConflict
		}
		targetID, err := strconv.ParseInt(terminal.TargetID, 10, 64)
		if err != nil {
			return campaignport.CampaignHistoryReceipt{}, err
		}
		return campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: terminal.TargetDigest, Replayed: true}, nil
	}
	targetID := int64(len(writer.targets) + 1)
	writer.targets[tableID+"/"+source] = targetID
	targetDigest := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", tableID, sourceID)))
	if err := journal.Record(ctx, TerminalReceipt{SourceKeyDigest: mustCampaignHistorySource(source), PayloadDigest: payload, Disposition: "import", TargetID: strconv.FormatInt(targetID, 10), TargetDigest: targetDigest}); err != nil {
		return campaignport.CampaignHistoryReceipt{}, err
	}
	onCreate()
	return campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: targetDigest}, nil
}

type campaignHistoryResolverFake struct {
	customerID      int64
	calls           int
	sameTransaction bool
}

func (resolver *campaignHistoryResolverFake) ResolveHistoricalCampaignCustomer(ctx context.Context, unionID string) (*int64, error) {
	if ctx.Value(campaignHistoryTxKey{}) != "tx" {
		resolver.sameTransaction = false
		return nil, errors.New("missing tx context")
	}
	resolver.sameTransaction = true
	resolver.calls++
	if unionID == "" || resolver.customerID == 0 {
		return nil, nil
	}
	value := resolver.customerID
	return &value, nil
}

func campaignHistoryRow(t *testing.T, tableID string, ordinal int64, value map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal, Payload: payload, RedactedFields: redacted,
		SourceKeyHMAC: sha256.Sum256([]byte(tableID + "/source/" + fmt.Sprint(ordinal))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(tableID + "/fields/" + fmt.Sprint(ordinal)))}
}

func broadcastPlanPayload(stamp time.Time) map[string]any {
	return map[string]any{
		"id": 801, "plan_id": "plan", "trace_id": "", "session_id": "", "operator": "", "intent": "intent", "segment_id": nil, "campaign_id": nil,
		"selection_json": map[string]any{}, "content_strategy": "strategy", "content_template": "13800138000", "personalization_json": map[string]any{}, "max_recipients": -1, "candidate_count": -2, "skipped_count": -3,
		"explanation_json": map[string]any{}, "variants_json": []any{}, "copy_workorder_run_ids": []any{}, "requires_manual_copy": true, "simulate_summary_json": map[string]any{}, "commit_batch_id": "", "commit_send_record_id": nil,
		"committed_at": nil, "committed_by": "", "approval_token_hash": "", "status": "old", "error_message": "", "expires_at": nil, "created_at": stamp, "updated_at": stamp, "display_name": "plan name", "owner_userid": "owner", "review_status": "old", "run_status": "old",
	}
}

func campaignHistorySourceTables() []string {
	return []string{campaignHistorySegmentsTable, campaignHistoryMembersTable, campaignHistoryPlansTable, campaignHistoryRecipientsTable, campaignHistoryMessagesTable}
}

func mustCampaignHistorySource(value string) [sha256.Size]byte {
	digest, err := ParseSourceIdentifier(value)
	if err != nil {
		panic(err)
	}
	return digest
}
