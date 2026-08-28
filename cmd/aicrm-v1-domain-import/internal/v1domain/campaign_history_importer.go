package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1broadcasthistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1campaignhistory"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const campaignHistoryContextTable = "public/campaigns"

// CampaignHistoryWriter is the Campaign-owned immutable history boundary. The
// importer calls it only from the caller's UnitOfWork transaction.
type CampaignHistoryWriter interface {
	WriteSegment(context.Context, string, [sha256.Size]byte, campaignport.HistoricalCampaignSegment) (campaignport.CampaignHistoryReceipt, error)
	WriteMember(context.Context, string, [sha256.Size]byte, campaignport.HistoricalCampaignMember) (campaignport.CampaignHistoryReceipt, error)
	WritePlan(context.Context, string, [sha256.Size]byte, campaignport.HistoricalBroadcastPlan) (campaignport.CampaignHistoryReceipt, error)
	WriteRecipient(context.Context, string, [sha256.Size]byte, campaignport.HistoricalBroadcastRecipient) (campaignport.CampaignHistoryReceipt, error)
	WriteMessage(context.Context, string, [sha256.Size]byte, campaignport.HistoricalBroadcastMessage) (campaignport.CampaignHistoryReceipt, error)
}

// CampaignHistoryCustomerResolver returns a previously verified DM01 mapping.
// A nil result is a valid unresolved historical relation; errors are never
// downgraded to an unverified customer reference.
type CampaignHistoryCustomerResolver interface {
	ResolveHistoricalCampaignCustomer(context.Context, string) (*int64, error)
}

type CampaignHistoryTableResult struct {
	Imported, Quarantined, Replayed int
}

type CampaignHistoryImportResult struct {
	Tables map[string]CampaignHistoryTableResult
}

type CampaignHistoryImporter struct {
	archive      ArchiveSource
	uow          UnitOfWork
	writer       CampaignHistoryWriter
	resolver     CampaignHistoryCustomerResolver
	journals     map[string]campaignHistoryTerminalJournal
	archiveRunID string
}

func NewCampaignHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer CampaignHistoryWriter, resolver CampaignHistoryCustomerResolver, journals map[string]*Journal) (*CampaignHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || !validCampaignHistoryImportJournals(journals) {
		return nil, ErrInvalidScope
	}
	terminals := make(map[string]campaignHistoryTerminalJournal, len(journals))
	for tableID, journal := range journals {
		terminals[tableID] = journal
	}
	return newCampaignHistoryImporter(archive, uow, writer, resolver, terminals, journals[campaignHistorySegmentsTable].scope.ArchiveRunID)
}

// newCampaignHistoryImporter is private test wiring. Production construction
// remains tied to exact scoped migration Journals above.
func newCampaignHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer CampaignHistoryWriter, resolver CampaignHistoryCustomerResolver, journals map[string]campaignHistoryTerminalJournal, archiveRunID string) (*CampaignHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || archiveRunID == "" || len(journals) != len(campaignHistoryScopes) {
		return nil, ErrInvalidScope
	}
	for _, scope := range campaignHistoryScopes {
		if journals[scope[0]] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &CampaignHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journals: journals, archiveRunID: archiveRunID}, nil
}

func (importer *CampaignHistoryImporter) Import(ctx context.Context, archiveRunID string) (CampaignHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil ||
		len(importer.journals) != len(campaignHistoryScopes) || archiveRunID == "" || archiveRunID != importer.archiveRunID {
		return CampaignHistoryImportResult{}, ErrInvalidScope
	}
	contextRows, err := importer.readRows(ctx, archiveRunID, campaignHistoryContextTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}
	segments, err := importer.readRows(ctx, archiveRunID, campaignHistorySegmentsTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}
	members, err := importer.readRows(ctx, archiveRunID, campaignHistoryMembersTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}
	plans, err := importer.readRows(ctx, archiveRunID, campaignHistoryPlansTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}
	recipients, err := importer.readRows(ctx, archiveRunID, campaignHistoryRecipientsTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}
	messages, err := importer.readRows(ctx, archiveRunID, campaignHistoryMessagesTable)
	if err != nil {
		return CampaignHistoryImportResult{}, err
	}

	campaigns := v1campaignhistory.AdaptHistory(payloads(contextRows), payloads(segments), payloads(members))
	broadcast := v1broadcasthistory.AdaptHistory(archivedRows(plans), archivedRows(recipients), archivedRows(messages))
	if len(campaigns.Segments) != len(segments) || len(campaigns.Members) != len(members) ||
		len(broadcast.Plans) != len(plans) || len(broadcast.Recipients) != len(recipients) || len(broadcast.Messages) != len(messages) {
		return CampaignHistoryImportResult{}, ErrConflict
	}

	result := newCampaignHistoryImportResult()
	segmentTargets := make(map[int64]int64, len(segments))
	for index, decision := range campaigns.Segments {
		target, err := importer.importSegment(ctx, segments[index], decision, &result)
		if err != nil {
			return CampaignHistoryImportResult{}, err
		}
		if target.sourceID > 0 {
			if old, found := segmentTargets[target.sourceID]; found && old != target.targetID {
				return CampaignHistoryImportResult{}, ErrConflict
			}
			segmentTargets[target.sourceID] = target.targetID
		}
	}
	for index, decision := range campaigns.Members {
		if err := importer.importMember(ctx, members[index], decision, segmentTargets, &result); err != nil {
			return CampaignHistoryImportResult{}, err
		}
	}
	planTargets := make(map[string]int64, len(plans))
	for index, decision := range broadcast.Plans {
		target, err := importer.importPlan(ctx, plans[index], decision, &result)
		if err != nil {
			return CampaignHistoryImportResult{}, err
		}
		if target.sourceID != "" {
			if old, found := planTargets[target.sourceID]; found && old != target.targetID {
				return CampaignHistoryImportResult{}, ErrConflict
			}
			planTargets[target.sourceID] = target.targetID
		}
	}
	recipientTargets := make(map[int64]campaignHistoryTarget, len(recipients))
	for index, decision := range broadcast.Recipients {
		target, err := importer.importRecipient(ctx, recipients[index], decision, planTargets, &result)
		if err != nil {
			return CampaignHistoryImportResult{}, err
		}
		if target.sourceID != 0 {
			if old, found := recipientTargets[target.sourceID]; found && old != target {
				return CampaignHistoryImportResult{}, ErrConflict
			}
			recipientTargets[target.sourceID] = target
		}
	}
	for index, decision := range broadcast.Messages {
		if err := importer.importMessage(ctx, messages[index], decision, planTargets, recipientTargets, &result); err != nil {
			return CampaignHistoryImportResult{}, err
		}
	}
	return result, nil
}

type campaignHistoryArchiveRow struct{ archive v1archive.ArchivedRow }

func (importer *CampaignHistoryImporter) readRows(ctx context.Context, archiveRunID, tableID string) ([]campaignHistoryArchiveRow, error) {
	rows := make([]campaignHistoryArchiveRow, 0)
	seen := map[[sha256.Size]byte]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if !validCampaignHistoryArchiveRow(row, tableID, expectedOrdinal) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, duplicate := seen[row.SourceKeyHMAC]; duplicate {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, campaignHistoryArchiveRow{archive: row})
		return nil
	})
	return rows, err
}

func validCampaignHistoryArchiveRow(row v1archive.ArchivedRow, tableID string, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == tableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func payloads(rows []campaignHistoryArchiveRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = rows[index].archive.Payload
	}
	return values
}

func archivedRows(rows []campaignHistoryArchiveRow) []v1archive.ArchivedRow {
	values := make([]v1archive.ArchivedRow, len(rows))
	for index := range rows {
		values[index] = rows[index].archive
	}
	return values
}

type campaignHistoryTarget struct {
	sourceID int64
	targetID int64
}

type campaignHistoryPlanTarget struct {
	sourceID string
	targetID int64
}

func (importer *CampaignHistoryImporter) importSegment(ctx context.Context, row campaignHistoryArchiveRow, decision v1campaignhistory.Result[v1campaignhistory.SegmentFact], result *CampaignHistoryImportResult) (campaignHistoryTarget, error) {
	if campaignHistoryRedacted(row.archive, campaignHistorySegmentsTable) {
		return importer.quarantine(ctx, campaignHistorySegmentsTable, row, "campaign_history_segment_required_field_redacted", result)
	}
	if decision.Disposition != v1campaignhistory.Candidate || decision.Fact == nil {
		return importer.quarantine(ctx, campaignHistorySegmentsTable, row, campaignHistoryReason(decision.Reason, "campaign_history_segment_invalid"), result)
	}
	fact := *decision.Fact
	target := campaignHistoryTarget{}
	replayed, imported := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		target, replayed, imported = campaignHistoryTarget{}, false, false
		receipt, err := importer.writer.WriteSegment(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, campaignport.HistoricalCampaignSegment{
			SourceID: fact.SourceID, CampaignSourceID: fact.CampaignSourceID, SegmentSourceID: fact.SegmentSourceID, SourceParentState: string(fact.SourceParentState),
			Code: fact.Code, Priority: fact.Priority, Label: fact.Label, CreatedAt: utcMicrosecond(fact.CreatedAt), SourcePayloadDigest: row.archive.PayloadHMAC,
		})
		if errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistorySegmentsTable, row, "campaign_history_segment_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, campaignHistorySegmentsTable, row, receipt); err != nil {
			return err
		}
		target, replayed, imported = campaignHistoryTarget{sourceID: fact.SourceID, targetID: receipt.TargetID}, receipt.Replayed, true
		return nil
	})
	if err != nil {
		return campaignHistoryTarget{}, err
	}
	result.increment(campaignHistorySegmentsTable, imported, replayed)
	return target, nil
}

func (importer *CampaignHistoryImporter) importMember(ctx context.Context, row campaignHistoryArchiveRow, decision v1campaignhistory.Result[v1campaignhistory.MemberFact], segments map[int64]int64, result *CampaignHistoryImportResult) error {
	if campaignHistoryRedacted(row.archive, campaignHistoryMembersTable) {
		_, err := importer.quarantine(ctx, campaignHistoryMembersTable, row, "campaign_history_member_required_field_redacted", result)
		return err
	}
	if decision.Disposition != v1campaignhistory.Candidate || decision.Fact == nil {
		_, err := importer.quarantine(ctx, campaignHistoryMembersTable, row, campaignHistoryReason(decision.Reason, "campaign_history_member_invalid"), result)
		return err
	}
	fact := *decision.Fact
	replayed, imported := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		segmentID, found := segments[fact.CampaignSegmentSourceID]
		if !found || segmentID < 1 {
			var err error
			replayed, err = importer.recordQuarantine(tx, campaignHistoryMembersTable, row, "campaign_history_member_segment_unresolved")
			return err
		}
		customerID, err := importer.resolveCustomer(tx, fact.Source.UnionID)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.WriteMember(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, campaignport.HistoricalCampaignMember{
			SourceID: fact.SourceID, CampaignSourceID: fact.CampaignSourceID, CampaignSegmentSourceID: fact.CampaignSegmentSourceID, SegmentSourceID: fact.SegmentSourceID,
			MemberSourceID: fact.MemberSourceID, SegmentHistoryID: segmentID, CustomerID: customerID, JoinedAt: utcMicrosecond(fact.JoinedAt), AnchorDate: fact.AnchorDate,
			CurrentStepIndex: fact.CurrentStepIndex, NextDueAt: utcMicrosecondPtr(fact.NextDueAt), OriginalStatus: fact.Status, StopReason: fact.StopReason,
			LastStepSentAt: utcMicrosecondPtr(fact.LastStepSentAt), RetryCount: fact.RetryCount, CreatedAt: utcMicrosecond(fact.CreatedAt), UpdatedAt: utcMicrosecond(fact.UpdatedAt), SourcePayloadDigest: row.archive.PayloadHMAC,
		})
		if errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryMembersTable, row, "campaign_history_member_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, campaignHistoryMembersTable, row, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	})
	if err != nil {
		return err
	}
	result.increment(campaignHistoryMembersTable, imported, replayed)
	return nil
}

func (importer *CampaignHistoryImporter) importPlan(ctx context.Context, row campaignHistoryArchiveRow, decision v1broadcasthistory.PlanResult, result *CampaignHistoryImportResult) (campaignHistoryPlanTarget, error) {
	if campaignHistoryRedacted(row.archive, campaignHistoryPlansTable) {
		_, err := importer.quarantine(ctx, campaignHistoryPlansTable, row, "campaign_history_broadcast_plan_required_field_redacted", result)
		return campaignHistoryPlanTarget{}, err
	}
	if decision.Disposition != v1broadcasthistory.DispositionCandidate || decision.Fact == nil {
		_, err := importer.quarantine(ctx, campaignHistoryPlansTable, row, campaignHistoryReason(decision.Reason, "campaign_history_broadcast_plan_invalid"), result)
		return campaignHistoryPlanTarget{}, err
	}
	fact := *decision.Fact
	target := campaignHistoryPlanTarget{}
	replayed, imported := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		target, replayed, imported = campaignHistoryPlanTarget{}, false, false
		template, err := maskCampaignHistoryText(fact.ContentTemplate)
		if err != nil {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryPlansTable, row, "campaign_history_broadcast_plan_content_invalid")
			return terminalErr
		}
		receipt, err := importer.writer.WritePlan(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, campaignport.HistoricalBroadcastPlan{
			SourceID: fact.SourceID, SourcePlanID: fact.PlanID, CampaignSourceID: cloneInt64(fact.CampaignSourceID), SegmentSourceID: cloneInt64(fact.SegmentSourceID),
			DisplayName: fact.DisplayName, Intent: fact.Intent, ContentStrategy: fact.ContentStrategy, ContentTemplateMasked: template,
			MaxRecipients: fact.MaxRecipients, CandidateCount: fact.CandidateCount, SkippedCount: fact.SkippedCount, RequiresManualCopy: fact.RequiresManualCopy,
			OriginalStatus: fact.OriginalStatus, OriginalReviewStatus: fact.OriginalReviewStatus, OriginalRunStatus: fact.OriginalRunStatus,
			CommittedAt: utcMicrosecondPtr(fact.CommittedAt), ExpiresAt: utcMicrosecondPtr(fact.ExpiresAt), CreatedAt: utcMicrosecond(fact.CreatedAt), UpdatedAt: utcMicrosecond(fact.UpdatedAt),
			RuntimeDigest: campaignHistoryRuntimeDigest(fact), SourcePayloadDigest: row.archive.PayloadHMAC,
		})
		if errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryPlansTable, row, "campaign_history_broadcast_plan_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, campaignHistoryPlansTable, row, receipt); err != nil {
			return err
		}
		target, replayed, imported = campaignHistoryPlanTarget{sourceID: fact.PlanID, targetID: receipt.TargetID}, receipt.Replayed, true
		return nil
	})
	if err != nil {
		return campaignHistoryPlanTarget{}, err
	}
	result.increment(campaignHistoryPlansTable, imported, replayed)
	return target, nil
}

func (importer *CampaignHistoryImporter) importRecipient(ctx context.Context, row campaignHistoryArchiveRow, decision v1broadcasthistory.RecipientResult, plans map[string]int64, result *CampaignHistoryImportResult) (campaignHistoryTarget, error) {
	if campaignHistoryRedacted(row.archive, campaignHistoryRecipientsTable) {
		return importer.quarantine(ctx, campaignHistoryRecipientsTable, row, "campaign_history_broadcast_recipient_required_field_redacted", result)
	}
	if decision.Disposition != v1broadcasthistory.DispositionCandidate || decision.Fact == nil {
		return importer.quarantine(ctx, campaignHistoryRecipientsTable, row, campaignHistoryReason(decision.Reason, "campaign_history_broadcast_recipient_invalid"), result)
	}
	fact := *decision.Fact
	target := campaignHistoryTarget{}
	replayed, imported := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		target, replayed, imported = campaignHistoryTarget{}, false, false
		planID, found := plans[fact.PlanID]
		if !found || planID < 1 {
			var err error
			replayed, err = importer.recordQuarantine(tx, campaignHistoryRecipientsTable, row, "campaign_history_broadcast_recipient_plan_unresolved")
			return err
		}
		customerID, err := importer.resolveCustomer(tx, fact.UnionID)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.WriteRecipient(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, campaignport.HistoricalBroadcastRecipient{
			SourceID: fact.SourceID, PlanHistoryID: planID, CustomerID: customerID, DisplayName: fact.DisplayName, PlannedMessageCount: fact.PlannedMessageCount,
			OriginalApprovalStatus: fact.OriginalApprovalStatus, OriginalSendStatus: fact.OriginalSendStatus, ApprovedAt: utcMicrosecondPtr(fact.ApprovedAt),
			RejectedAt: utcMicrosecondPtr(fact.RejectedAt), CreatedAt: utcMicrosecond(fact.CreatedAt), UpdatedAt: utcMicrosecond(fact.UpdatedAt), SourcePayloadDigest: row.archive.PayloadHMAC,
		})
		if errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryRecipientsTable, row, "campaign_history_broadcast_recipient_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, campaignHistoryRecipientsTable, row, receipt); err != nil {
			return err
		}
		target, replayed, imported = campaignHistoryTarget{sourceID: fact.SourceID, targetID: receipt.TargetID}, receipt.Replayed, true
		return nil
	})
	if err != nil {
		return campaignHistoryTarget{}, err
	}
	result.increment(campaignHistoryRecipientsTable, imported, replayed)
	return target, nil
}

func (importer *CampaignHistoryImporter) importMessage(ctx context.Context, row campaignHistoryArchiveRow, decision v1broadcasthistory.MessageResult, plans map[string]int64, recipients map[int64]campaignHistoryTarget, result *CampaignHistoryImportResult) error {
	if campaignHistoryRedacted(row.archive, campaignHistoryMessagesTable) {
		_, err := importer.quarantine(ctx, campaignHistoryMessagesTable, row, "campaign_history_broadcast_message_required_field_redacted", result)
		return err
	}
	if decision.Disposition != v1broadcasthistory.DispositionCandidate || decision.Fact == nil {
		_, err := importer.quarantine(ctx, campaignHistoryMessagesTable, row, campaignHistoryReason(decision.Reason, "campaign_history_broadcast_message_invalid"), result)
		return err
	}
	fact := *decision.Fact
	replayed, imported := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, imported = false, false
		planID, found := plans[fact.PlanID]
		if !found || planID < 1 {
			var err error
			replayed, err = importer.recordQuarantine(tx, campaignHistoryMessagesTable, row, "campaign_history_broadcast_message_plan_unresolved")
			return err
		}
		recipient, found := recipients[fact.RecipientSourceID]
		if !found || recipient.targetID < 1 {
			var err error
			replayed, err = importer.recordQuarantine(tx, campaignHistoryMessagesTable, row, "campaign_history_broadcast_message_recipient_unresolved")
			return err
		}
		customerID, err := importer.resolveCustomer(tx, fact.UnionID)
		if err != nil {
			return err
		}
		content, err := maskCampaignHistoryText(fact.ContentText)
		if err != nil {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryMessagesTable, row, "campaign_history_broadcast_message_content_invalid")
			return terminalErr
		}
		receipt, err := importer.writer.WriteMessage(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, campaignport.HistoricalBroadcastMessage{
			SourceID: fact.SourceID, PlanHistoryID: planID, RecipientHistoryID: recipient.targetID, CustomerID: customerID, SequenceIndex: fact.SequenceIndex,
			DayOffset: fact.DayOffset, OriginalSendTime: fact.SendTime, ContentMasked: content, OriginalStatus: fact.OriginalStatus, SentAt: utcMicrosecondPtr(fact.SentAt),
			CreatedAt: utcMicrosecond(fact.CreatedAt), UpdatedAt: utcMicrosecond(fact.UpdatedAt), ContentPayloadDigest: fact.ContentPayloadDigest,
			AttachmentsDigest: fact.AttachmentsDigest, SourcePayloadDigest: row.archive.PayloadHMAC,
		})
		if errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			var terminalErr error
			replayed, terminalErr = importer.recordQuarantine(tx, campaignHistoryMessagesTable, row, "campaign_history_broadcast_message_target_invalid")
			return terminalErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, campaignHistoryMessagesTable, row, receipt); err != nil {
			return err
		}
		replayed, imported = receipt.Replayed, true
		return nil
	})
	if err != nil {
		return err
	}
	result.increment(campaignHistoryMessagesTable, imported, replayed)
	return nil
}

func (importer *CampaignHistoryImporter) resolveCustomer(ctx context.Context, unionID string) (*int64, error) {
	if unionID == "" {
		return nil, nil
	}
	resolved, err := importer.resolver.ResolveHistoricalCampaignCustomer(ctx, unionID)
	if err != nil || resolved == nil {
		return resolved, err
	}
	if *resolved < 1 {
		return nil, ErrConflict
	}
	value := *resolved
	return &value, nil
}

func (importer *CampaignHistoryImporter) quarantine(ctx context.Context, tableID string, row campaignHistoryArchiveRow, reason string, result *CampaignHistoryImportResult) (campaignHistoryTarget, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = importer.recordQuarantine(tx, tableID, row, reason)
		return err
	})
	if err != nil {
		return campaignHistoryTarget{}, err
	}
	result.increment(tableID, false, replayed)
	return campaignHistoryTarget{}, nil
}

func (importer *CampaignHistoryImporter) recordQuarantine(ctx context.Context, tableID string, row campaignHistoryArchiveRow, reason string) (bool, error) {
	journal := importer.journals[tableID]
	if journal == nil || reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.archive.SourceKeyHMAC)
	existing, found, err := journal.LoadTerminal(ctx, source)
	if err != nil {
		return false, err
	}
	want := TerminalReceipt{SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC, Disposition: "quarantine", Reason: reason}
	if found {
		if !sameCampaignHistoryTerminal(existing, want) {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.Record(ctx, want)
}

func (importer *CampaignHistoryImporter) verifyReceipt(ctx context.Context, tableID string, row campaignHistoryArchiveRow, receipt campaignport.CampaignHistoryReceipt) error {
	journal := importer.journals[tableID]
	if journal == nil || receipt.SourceIdentifier != SourceIdentifier(row.archive.SourceKeyHMAC) || receipt.PayloadDigest != row.archive.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadTerminal(ctx, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.archive.SourceKeyHMAC || terminal.PayloadDigest != row.archive.PayloadHMAC || terminal.Disposition != "import" ||
		terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func newCampaignHistoryImportResult() CampaignHistoryImportResult {
	result := CampaignHistoryImportResult{Tables: make(map[string]CampaignHistoryTableResult, len(campaignHistoryScopes))}
	for _, scope := range campaignHistoryScopes {
		result.Tables[scope[0]] = CampaignHistoryTableResult{}
	}
	return result
}

func (result *CampaignHistoryImportResult) increment(tableID string, imported, replayed bool) {
	value := result.Tables[tableID]
	if imported {
		value.Imported++
	} else {
		value.Quarantined++
	}
	if replayed {
		value.Replayed++
	}
	result.Tables[tableID] = value
}

func campaignHistoryReason(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

func campaignHistoryRedacted(row v1archive.ArchivedRow, tableID string) bool {
	for _, field := range campaignHistoryRequiredFields[tableID] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

var campaignHistoryRequiredFields = map[string][]string{
	campaignHistorySegmentsTable:   {"id", "campaign_id", "segment_id", "segment_code", "priority", "label", "created_at"},
	campaignHistoryMembersTable:    {"id", "campaign_id", "campaign_segment_id", "segment_id", "member_id", "joined_at", "anchor_date", "current_step_index", "status", "stop_reason", "retry_count", "created_at", "updated_at"},
	campaignHistoryPlansTable:      {"id", "plan_id", "intent", "selection_json", "content_strategy", "content_template", "personalization_json", "max_recipients", "candidate_count", "skipped_count", "explanation_json", "variants_json", "copy_workorder_run_ids", "requires_manual_copy", "simulate_summary_json", "status", "created_at", "updated_at", "display_name", "review_status", "run_status"},
	campaignHistoryRecipientsTable: {"id", "plan_id", "display_name", "planned_message_count", "approval_status", "send_status", "created_at", "updated_at"},
	campaignHistoryMessagesTable:   {"id", "plan_id", "recipient_id", "sequence_index", "day_offset", "send_time", "content_text", "content_payload_json", "attachments_json", "status", "created_at", "updated_at"},
}

func campaignHistoryRuntimeDigest(fact v1broadcasthistory.PlanFact) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("campaign-v1-broadcast-runtime-v1\x00"))
	for _, digest := range [][sha256.Size]byte{fact.SelectionDigest, fact.PersonalizationDigest, fact.ExplanationDigest, fact.VariantsDigest, fact.CopyWorkorderRunIDsDigest, fact.SimulateSummaryDigest} {
		_, _ = hash.Write(digest[:])
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func maskCampaignHistoryText(value string) (string, error) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", campaignport.ErrCampaignHistoryInvalid
	}
	var masked strings.Builder
	masked.Grow(len(value))
	for offset := 0; offset < len(value); {
		end := offset
		if value[offset] == '+' {
			end++
		}
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		if campaignHistoryPhoneLike(value[offset:end]) {
			masked.WriteString("[masked-phone]")
			offset = end
			continue
		}
		_, width := utf8.DecodeRuneInString(value[offset:])
		masked.WriteString(value[offset : offset+width])
		offset += width
	}
	return masked.String(), nil
}

func campaignHistoryPhoneLike(value string) bool {
	digits := strings.TrimPrefix(value, "+86")
	return len(digits) == 11 && digits[0] == '1' && digits[1] >= '3' && digits[1] <= '9'
}

func utcMicrosecond(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func utcMicrosecondPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := utcMicrosecond(*value)
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
