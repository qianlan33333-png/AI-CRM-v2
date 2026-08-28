package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// HXCHistoryWriter is the HXC-owned boundary for immutable observations. It
// must use the caller transaction and must never create current HXC work.
type HXCHistoryWriter interface {
	ImportMeta(context.Context, string, hxcport.HistoricalHXCMeta) (hxcport.HXCHistoryReceipt, error)
	ImportSnapshot(context.Context, string, hxcport.HistoricalHXCSnapshot) (hxcport.HXCHistoryReceipt, error)
	ImportActivation(context.Context, string, hxcport.HistoricalHXCActivation) (hxcport.HXCHistoryReceipt, error)
	ImportLead(context.Context, string, hxcport.HistoricalHXCLead) (hxcport.HXCHistoryReceipt, error)
	ImportBatch(context.Context, string, hxcport.HistoricalHXCBatch) (hxcport.HXCHistoryReceipt, error)
}

// HXCHistoryCustomerResolver returns only a verified DM01 customer. A nil
// result is an allowed unresolved historical link; errors never fall back to a
// guessed ID.
type HXCHistoryCustomerResolver interface {
	ResolveHXCHistoryCustomer(context.Context, string) (*int64, error)
}

type HXCHistoryTableResult struct{ Imported, Archived, Quarantined, Replayed int }

type HXCHistoryImportResult struct {
	Tables map[string]HXCHistoryTableResult
}

func (result HXCHistoryImportResult) SourceCount() int {
	total := 0
	for _, value := range result.Tables {
		total += value.Imported + value.Archived + value.Quarantined
	}
	return total
}

type HXCHistoryImporter struct {
	archive      ArchiveSource
	uow          UnitOfWork
	writer       HXCHistoryWriter
	resolver     HXCHistoryCustomerResolver
	journal      HXCHistoryImportJournal
	archiveRunID string
}

func NewHXCHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer HXCHistoryWriter, resolver HXCHistoryCustomerResolver, journal HXCHistoryImportJournal) (*HXCHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &HXCHistoryImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journal: journal}, nil
}

func (importer *HXCHistoryImporter) Import(ctx context.Context, archiveRunID string) (HXCHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil || importer.journal == nil ||
		archiveRunID == "" || importer.journal.ValidateHXCHistoryImportScope(archiveRunID) != nil {
		return HXCHistoryImportResult{}, ErrInvalidScope
	}
	rows := make(map[string][]v1archive.ArchivedRow, len(hxcHistoryTables))
	for _, table := range hxcHistoryTables {
		values, err := importer.readRows(ctx, archiveRunID, table)
		if err != nil {
			return HXCHistoryImportResult{}, err
		}
		rows[table] = values
	}
	history := v1hxchistory.AdaptHistory(
		hxcHistoryPayloads(rows[v1hxchistory.DashboardMetaTableID]),
		hxcHistoryPayloads(rows[v1hxchistory.DashboardSnapshotTableID]),
		hxcHistoryPayloads(rows[v1hxchistory.ActivationStatusTableID]),
		hxcHistoryPayloads(rows[v1hxchistory.HuangxiaocanActivationID]),
		hxcHistoryPayloads(rows[v1hxchistory.ExperienceLeadsTableID]),
		hxcHistoryPayloads(rows[v1hxchistory.ImportBatchesTableID]),
	)
	if len(history.DashboardMeta) != len(rows[v1hxchistory.DashboardMetaTableID]) || len(history.DashboardSnapshot) != len(rows[v1hxchistory.DashboardSnapshotTableID]) ||
		len(history.ActivationStatus) != len(rows[v1hxchistory.ActivationStatusTableID]) || len(history.Huangxiaocan) != len(rows[v1hxchistory.HuangxiaocanActivationID]) ||
		len(history.ExperienceLeads) != len(rows[v1hxchistory.ExperienceLeadsTableID]) || len(history.ImportBatches) != len(rows[v1hxchistory.ImportBatchesTableID]) {
		return HXCHistoryImportResult{}, ErrConflict
	}
	result := hxcHistoryResult()
	for index, decision := range history.DashboardMeta {
		if err := importer.importMeta(ctx, rows[v1hxchistory.DashboardMetaTableID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for index, decision := range history.DashboardSnapshot {
		if err := importer.importSnapshot(ctx, rows[v1hxchistory.DashboardSnapshotTableID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for index, decision := range history.ActivationStatus {
		if err := importer.importActivation(ctx, v1hxchistory.ActivationStatusTableID, rows[v1hxchistory.ActivationStatusTableID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for index, decision := range history.Huangxiaocan {
		if err := importer.importActivation(ctx, v1hxchistory.HuangxiaocanActivationID, rows[v1hxchistory.HuangxiaocanActivationID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for index, decision := range history.ExperienceLeads {
		if err := importer.importLead(ctx, rows[v1hxchistory.ExperienceLeadsTableID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for index, decision := range history.ImportBatches {
		if err := importer.importBatch(ctx, rows[v1hxchistory.ImportBatchesTableID][index], decision, &result); err != nil {
			return HXCHistoryImportResult{}, err
		}
	}
	for _, table := range []string{v1hxchistory.SendRecordsTableID, v1hxchistory.SendConfigTableID} {
		for _, row := range rows[table] {
			if err := importer.archiveOnly(ctx, table, row, &result); err != nil {
				return HXCHistoryImportResult{}, err
			}
		}
	}
	return result, nil
}

func hxcHistoryResult() HXCHistoryImportResult {
	result := HXCHistoryImportResult{Tables: make(map[string]HXCHistoryTableResult, len(hxcHistoryTables))}
	for _, table := range hxcHistoryTables {
		result.Tables[table] = HXCHistoryTableResult{}
	}
	return result
}

func (importer *HXCHistoryImporter) importMeta(ctx context.Context, row v1archive.ArchivedRow, decision v1hxchistory.Decision[v1hxchistory.DashboardMetaFact], result *HXCHistoryImportResult) error {
	if reason := hxcHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "hxc_history_meta_invalid"); reason != "" {
		return importer.quarantine(ctx, hxcport.HXCHistoryMeta, row, reason, result)
	}
	fact := *decision.Fact
	value := hxcport.HistoricalHXCMeta{HistoricalHXCIdentity: hxcHistoryIdentity(row, fact.SourceID), StartedAt: hxcHistoryTime(fact.StartedAt), FinishedAt: hxcHistoryTimePtr(fact.FinishedAt),
		Status: fact.Status, RowCount: fact.RowCount, MemberHit: fact.MemberHit, UserHit: fact.UserHit, OnlyMember: fact.OnlyMember, TriggerSource: fact.TriggerSource}
	return importer.write(ctx, hxcport.HXCHistoryMeta, row, result, func(tx context.Context) (hxcport.HXCHistoryReceipt, error) {
		return importer.writer.ImportMeta(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *HXCHistoryImporter) importSnapshot(ctx context.Context, row v1archive.ArchivedRow, decision v1hxchistory.Decision[v1hxchistory.DashboardSnapshotFact], result *HXCHistoryImportResult) error {
	if reason := hxcHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "hxc_history_snapshot_invalid"); reason != "" {
		return importer.quarantine(ctx, hxcport.HXCHistorySnapshot, row, reason, result)
	}
	fact := *decision.Fact
	return importer.write(ctx, hxcport.HXCHistorySnapshot, row, result, func(tx context.Context) (hxcport.HXCHistoryReceipt, error) {
		var customerID *int64
		var err error
		if unionID := fact.ResolverUnionID(); unionID != "" {
			customerID, err = importer.resolver.ResolveHXCHistoryCustomer(tx, unionID)
			if err != nil {
				return hxcport.HXCHistoryReceipt{}, err
			}
			if customerID != nil && *customerID < 1 {
				return hxcport.HXCHistoryReceipt{}, hxcport.ErrHXCHistoryInvalid
			}
		}
		value := hxcport.HistoricalHXCSnapshot{HistoricalHXCIdentity: hxcHistoryIdentity(row, fact.SourceID), CustomerID: copyInt64(customerID), Observation: fact.Observation,
			ObservedAt: hxcHistoryTime(fact.ObservedAt), InLeadPool: fact.InLeadPool, InPeople: fact.InPeople, InQuestionnaire: fact.InQuestionnaire, ClassTermNo: copyInt64(fact.ClassTermNo),
			ClassTermLabel: fact.ClassTermLabel, CRMHXCState: fact.CRMHXCState, CRMCreatedAt: copyString(fact.CRMCreatedAt), LastQuestionnaireAt: copyString(fact.LastQuestionnaireAt),
			HXCMemberHit: fact.HXCMemberHit, HXCUserHit: fact.HXCUserHit, FunnelState: fact.FunnelState, HXCMemberStatus: fact.HXCMemberStatus,
			HXCRegisteredAt: hxcHistoryTimePtr(fact.HXCRegisteredAt), HXCLastLoginAt: hxcHistoryTimePtr(fact.HXCLastLoginAt), MembershipType: fact.MembershipType,
			MembershipStatus: fact.MembershipStatus, MembershipEndAt: hxcHistoryTimePtr(fact.MembershipEndAt), MembershipDaysLeft: copyInt64(fact.MembershipDaysLeft),
			ConsultationUsed: copyInt64(fact.ConsultationUsed), ConsultationLimit: copyInt64(fact.ConsultationLimit), ConversationChat: fact.ConversationChat,
			ConversationConsult: fact.ConversationConsult, ConversationLesson: fact.ConversationLesson, MessagesUser: fact.MessagesUser, MessagesAI: fact.MessagesAI,
			ConsultCompleted: fact.ConsultCompleted, LastMessageAt: hxcHistoryTimePtr(fact.LastMessageAt), SubscriptionTier: fact.SubscriptionTier,
			SubscriptionExpires: hxcHistoryTimePtr(fact.SubscriptionExpires), SubscriptionQuota: copyInt64(fact.SubscriptionQuota), SubscriptionUsed: copyInt64(fact.SubscriptionUsed),
			SubscriptionPeriodStart: copyString(fact.SubscriptionPeriodStart)}
		return importer.writer.ImportSnapshot(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *HXCHistoryImporter) importActivation(ctx context.Context, table string, row v1archive.ArchivedRow, decision v1hxchistory.Decision[v1hxchistory.ActivationFact], result *HXCHistoryImportResult) error {
	kind := hxcport.HXCHistoryActivationStatus
	if table == v1hxchistory.HuangxiaocanActivationID {
		kind = hxcport.HXCHistoryHuangxiaocanActivation
	}
	if reason := hxcHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "hxc_history_activation_invalid"); reason != "" {
		return importer.quarantine(ctx, kind, row, reason, result)
	}
	fact := *decision.Fact
	if fact.SourceTable != table {
		return importer.quarantine(ctx, kind, row, "hxc_history_activation_source_table_invalid", result)
	}
	value := hxcport.HistoricalHXCActivation{HistoricalHXCIdentity: hxcHistoryIdentity(row, fact.SourceID), SourceTable: fact.SourceTable, OriginalState: fact.OriginalState,
		IsActive: fact.IsActive, LegacyImportBatchRef: copyString(fact.LegacyImportBatchRef), CreatedAt: hxcHistoryTime(fact.CreatedAt), UpdatedAt: hxcHistoryTime(fact.UpdatedAt)}
	return importer.write(ctx, kind, row, result, func(tx context.Context) (hxcport.HXCHistoryReceipt, error) {
		return importer.writer.ImportActivation(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *HXCHistoryImporter) importLead(ctx context.Context, row v1archive.ArchivedRow, decision v1hxchistory.Decision[v1hxchistory.ExperienceLeadFact], result *HXCHistoryImportResult) error {
	if reason := hxcHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "hxc_history_lead_invalid"); reason != "" {
		return importer.quarantine(ctx, hxcport.HXCHistoryLead, row, reason, result)
	}
	fact := *decision.Fact
	value := hxcport.HistoricalHXCLead{HistoricalHXCIdentity: hxcHistoryIdentity(row, fact.SourceID), OriginalType: fact.OriginalType, IsActive: fact.IsActive,
		LegacyImportBatchRef: copyString(fact.LegacyImportBatchRef), CreatedAt: hxcHistoryTime(fact.CreatedAt), UpdatedAt: hxcHistoryTime(fact.UpdatedAt)}
	return importer.write(ctx, hxcport.HXCHistoryLead, row, result, func(tx context.Context) (hxcport.HXCHistoryReceipt, error) {
		return importer.writer.ImportLead(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *HXCHistoryImporter) importBatch(ctx context.Context, row v1archive.ArchivedRow, decision v1hxchistory.Decision[v1hxchistory.ImportBatchFact], result *HXCHistoryImportResult) error {
	if reason := hxcHistoryDecisionReason(row, decision.Disposition, decision.Fact != nil, "hxc_history_batch_invalid"); reason != "" {
		return importer.quarantine(ctx, hxcport.HXCHistoryBatch, row, reason, result)
	}
	fact := *decision.Fact
	value := hxcport.HistoricalHXCBatch{HistoricalHXCIdentity: hxcHistoryIdentity(row, fact.SourceID), ImportType: fact.ImportType, TotalRows: fact.TotalRows,
		SuccessRows: fact.SuccessRows, FailedRows: fact.FailedRows, CreatedAt: hxcHistoryTime(fact.CreatedAt)}
	return importer.write(ctx, hxcport.HXCHistoryBatch, row, result, func(tx context.Context) (hxcport.HXCHistoryReceipt, error) {
		return importer.writer.ImportBatch(tx, SourceIdentifier(row.SourceKeyHMAC), value)
	})
}

func (importer *HXCHistoryImporter) write(ctx context.Context, kind string, row v1archive.ArchivedRow, result *HXCHistoryImportResult, apply func(context.Context) (hxcport.HXCHistoryReceipt, error)) error {
	imported, replayed := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		imported, replayed = false, false
		receipt, err := apply(tx)
		if errors.Is(err, hxcport.ErrHXCHistoryInvalid) {
			var quarantineErr error
			replayed, quarantineErr = importer.recordQuarantine(tx, kind, row, "hxc_history_target_invalid")
			return quarantineErr
		}
		if err != nil {
			return err
		}
		if err = importer.verifyReceipt(tx, kind, row, receipt); err != nil {
			return err
		}
		imported, replayed = true, receipt.Replayed
		return nil
	})
	if err != nil {
		return err
	}
	value := result.Tables[row.TableID]
	if imported {
		value.Imported++
	} else {
		value.Quarantined++
	}
	if replayed {
		value.Replayed++
	}
	result.Tables[row.TableID] = value
	return nil
}

func (importer *HXCHistoryImporter) quarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string, result *HXCHistoryImportResult) error {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		var err error
		replayed, err = importer.recordQuarantine(tx, kind, row, reason)
		return err
	})
	if err != nil {
		return err
	}
	value := result.Tables[row.TableID]
	value.Quarantined++
	if replayed {
		value.Replayed++
	}
	result.Tables[row.TableID] = value
	return nil
}

func (importer *HXCHistoryImporter) archiveOnly(ctx context.Context, table string, row v1archive.ArchivedRow, result *HXCHistoryImportResult) error {
	decision := v1hxchistory.ClassifyArchiveOnlyTable(table)
	if decision.Disposition != v1hxchistory.DispositionArchive || decision.Reason == "" {
		return ErrConflict
	}
	kind := hxcHistorySendRecordsKind
	if table == v1hxchistory.SendConfigTableID {
		kind = hxcHistorySendConfigKind
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		existing, found, err := importer.journal.LoadHXCHistoryTerminal(tx, kind, SourceIdentifier(row.SourceKeyHMAC))
		if err != nil {
			return err
		}
		if found {
			if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "archive" || existing.Reason != decision.Reason ||
				existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		return importer.journal.RecordHXCHistoryTerminal(tx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "archive", Reason: decision.Reason})
	})
	if err != nil {
		return err
	}
	value := result.Tables[table]
	value.Archived++
	if replayed {
		value.Replayed++
	}
	result.Tables[table] = value
	return nil
}

func (importer *HXCHistoryImporter) recordQuarantine(ctx context.Context, kind string, row v1archive.ArchivedRow, reason string) (bool, error) {
	if reason == "" {
		return false, ErrInvalidScope
	}
	existing, found, err := importer.journal.LoadHXCHistoryTerminal(ctx, kind, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason ||
			existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, importer.journal.RecordHXCHistoryTerminal(ctx, kind, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func (importer *HXCHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt hxcport.HXCHistoryReceipt) error {
	if receipt.Kind != kind || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadHXCHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func (importer *HXCHistoryImporter) readRows(ctx context.Context, archiveRunID, table string) ([]v1archive.ArchivedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	seen := map[[sha256.Size]byte]struct{}{}
	ordinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal || row.SourceKeyHMAC == ([sha256.Size]byte{}) ||
			row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return ErrConflict
		}
		ordinal++
		if _, found := seen[row.SourceKeyHMAC]; found {
			return ErrConflict
		}
		seen[row.SourceKeyHMAC] = struct{}{}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func hxcHistoryPayloads(rows []v1archive.ArchivedRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = rows[index].Payload
	}
	return values
}

func hxcHistoryDecisionReason(row v1archive.ArchivedRow, disposition v1hxchistory.Disposition, hasFact bool, fallback string) string {
	if len(row.RedactedFields) != 0 {
		return "hxc_history_business_field_redacted"
	}
	if disposition != v1hxchistory.DispositionCandidate || !hasFact {
		return fallback
	}
	return ""
}

func hxcHistoryIdentity(row v1archive.ArchivedRow, sourceID int64) hxcport.HistoricalHXCIdentity {
	return hxcport.HistoricalHXCIdentity{SourceID: sourceID, SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC}
}

func hxcHistoryTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func hxcHistoryTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := hxcHistoryTime(*value)
	return &result
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
