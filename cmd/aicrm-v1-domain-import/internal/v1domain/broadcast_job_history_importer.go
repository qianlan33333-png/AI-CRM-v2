package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1broadcastjobhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

// BroadcastJobHistoryWriter owns the immutable Outbound target. It must use
// the caller's UnitOfWork transaction and cannot enqueue a current task.
type BroadcastJobHistoryWriter interface {
	Import(context.Context, string, outboundport.HistoricalBroadcastJob) (outboundport.BroadcastJobHistoryReceipt, error)
}

// BroadcastJobHistoryImportJournal joins the owner receipt with the sealed
// import terminal. The main integration owns its concrete Journal adapter.
type BroadcastJobHistoryImportJournal interface {
	outboundport.BroadcastJobHistoryJournal
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
	ValidateBroadcastJobHistoryImportScope(string) error
}

type BroadcastJobHistoryImportResult struct{ ImportedJobs, Quarantined, Replayed int }

type BroadcastJobHistoryImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  BroadcastJobHistoryWriter
	journal BroadcastJobHistoryImportJournal
}

func NewBroadcastJobHistoryImporter(archive ArchiveSource, uow UnitOfWork, writer BroadcastJobHistoryWriter, journal BroadcastJobHistoryImportJournal) (*BroadcastJobHistoryImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil {
		return nil, ErrInvalidScope
	}
	return &BroadcastJobHistoryImporter{archive: archive, uow: uow, writer: writer, journal: journal}, nil
}

// Import processes only public.broadcast_jobs. Original statuses and Provider
// observations remain history; this path has no batch, task, event, or queue.
func (importer *BroadcastJobHistoryImporter) Import(ctx context.Context, archiveRunID string) (BroadcastJobHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil {
		return BroadcastJobHistoryImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateBroadcastJobHistoryImportScope(archiveRunID); err != nil {
		return BroadcastJobHistoryImportResult{}, err
	}
	result := BroadcastJobHistoryImportResult{}
	seenKeys, seenIDs := map[[sha256.Size]byte]struct{}{}, map[int64]struct{}{}
	expectedOrdinal := int64(1)
	err := importer.archive.EachTableRow(ctx, archiveRunID, v1broadcastjobhistory.BroadcastJobsTableID, func(row v1archive.ArchivedRow) error {
		if !validBroadcastJobHistoryArchiveRow(row, expectedOrdinal) {
			return ErrConflict
		}
		expectedOrdinal++
		if _, found := seenKeys[row.SourceKeyHMAC]; found {
			return ErrConflict
		}
		seenKeys[row.SourceKeyHMAC] = struct{}{}
		decision := v1broadcastjobhistory.AdaptHistory(row)
		if decision.Fact != nil {
			if _, found := seenIDs[decision.Fact.SourceID]; found {
				return ErrConflict
			}
			seenIDs[decision.Fact.SourceID] = struct{}{}
		}
		imported, replayed := false, false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			// UnitOfWork may retry the callback; expose only its committed result.
			imported, replayed = false, false
			if decision.Disposition != v1broadcastjobhistory.DispositionCandidate || decision.Fact == nil {
				var err error
				replayed, err = recordBroadcastJobHistoryQuarantine(tx, importer.journal, row, broadcastJobHistoryReason(decision.Reason))
				return err
			}
			value, err := broadcastJobHistoryValue(row, *decision.Fact)
			if err != nil {
				return err
			}
			receipt, err := importer.writer.Import(tx, SourceIdentifier(row.SourceKeyHMAC), value)
			if errors.Is(err, outboundport.ErrBroadcastJobHistoryInvalid) {
				replayed, err = recordBroadcastJobHistoryQuarantine(tx, importer.journal, row, "broadcast_job_target_invalid")
				return err
			}
			if err != nil {
				return err
			}
			if err = verifyBroadcastJobHistoryReceipt(tx, importer.journal, row, receipt); err != nil {
				return err
			}
			imported, replayed = true, receipt.Replayed
			return nil
		}); err != nil {
			return err
		}
		if imported {
			result.ImportedJobs++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

func validBroadcastJobHistoryArchiveRow(row v1archive.ArchivedRow, ordinal int64) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == v1broadcastjobhistory.BroadcastJobsTableID && row.SourceOrdinal == ordinal &&
		row.SourceKeyHMAC != ([sha256.Size]byte{}) && row.PayloadHMAC != ([sha256.Size]byte{}) && row.FieldHMAC != ([sha256.Size]byte{}) && json.Valid(row.Payload)
}

func broadcastJobHistoryValue(row v1archive.ArchivedRow, fact v1broadcastjobhistory.BroadcastJobHistory) (outboundport.HistoricalBroadcastJob, error) {
	if fact.SourceKeyDigest != row.SourceKeyHMAC || fact.SourcePayloadDigest != row.PayloadHMAC || fact.ArchiveFieldDigest != row.FieldHMAC {
		return outboundport.HistoricalBroadcastJob{}, ErrConflict
	}
	return outboundport.HistoricalBroadcastJob{
		SourceID: fact.SourceID, OriginalSourceType: fact.OriginalSourceType, SourceReferenceDigest: [32]byte(fact.SourceReferenceDigest), SourceTable: fact.SourceTable,
		ScheduledFor: fact.ScheduledFor, Priority: fact.Priority, BatchKeyDigest: [32]byte(fact.BatchKeyDigest), OriginalStatus: fact.OriginalStatus, RequiresApproval: fact.RequiresApproval,
		ApprovedByDigest: [32]byte(fact.ApprovedByDigest), ApprovedAt: fact.ApprovedAt, CancelledByDigest: [32]byte(fact.CancelledByDigest), CancelledAt: fact.CancelledAt, CancelReasonDigest: [32]byte(fact.CancelReasonDigest),
		TargetCount: fact.TargetCount, TargetSummaryDigest: [32]byte(fact.TargetSummaryDigest), ContentType: fact.ContentType, ContentPayloadDigest: [32]byte(fact.ContentPayloadDigest), ContentSummaryDigest: [32]byte(fact.ContentSummaryDigest),
		AttemptCount: fact.AttemptCount, LastErrorDigest: [32]byte(fact.LastErrorDigest), LegacyOutboundTaskID: fact.LegacyOutboundTaskID, SentCount: fact.SentCount, FailedCount: fact.FailedCount,
		TraceIDDigest: [32]byte(fact.TraceIDDigest), CreatedByDigest: [32]byte(fact.CreatedByDigest), CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt, ClaimedAt: fact.ClaimedAt, SentAt: fact.SentAt,
		ClaimTokenDigest: [32]byte(fact.ClaimTokenDigest), LeaseExpiresAt: fact.LeaseExpiresAt, BusinessDomain: fact.BusinessDomain, IdempotencyKeyDigest: copyBroadcastJobHistoryDigest(fact.IdempotencyKeyDigest), Channel: fact.Channel, TargetKind: fact.TargetKind, FailureType: fact.FailureType,
		RetryPolicyDigest: [32]byte(fact.RetryPolicyDigest), MetadataDigest: [32]byte(fact.MetadataDigest), TargetUnionIDsDigest: [32]byte(fact.TargetUnionIDsDigest), MaxAttempts: fact.MaxAttempts,
		NextRetryAt: fact.NextRetryAt, DispatchStartedAt: fact.DispatchStartedAt, SideEffectExecuted: fact.SideEffectExecuted, ProviderResultReceived: fact.ProviderResultReceived,
		ResultSummaryDigest: [32]byte(fact.ResultSummaryDigest), ReconciliationRequired: fact.ReconciliationRequired, CompletedAt: fact.CompletedAt, HoldReasonDigest: [32]byte(fact.HoldReasonDigest), HoldAt: fact.HoldAt,
		LegacyExternalEffectJobID: fact.LegacyExternalEffectJobID, ExecutionIDDigest: [32]byte(fact.ExecutionIDDigest), ExecutionOwnerDigest: [32]byte(fact.ExecutionOwnerDigest),
		SourceKeyDigest: row.SourceKeyHMAC, SourcePayloadDigest: row.PayloadHMAC, SourceFieldDigest: row.FieldHMAC, RedactedRoots: append([]string{}, fact.RedactedRoots...),
	}, nil
}

func copyBroadcastJobHistoryDigest(value *v1broadcastjobhistory.OpaqueDigest) *[32]byte {
	if value == nil {
		return nil
	}
	copy := [32]byte(*value)
	return &copy
}

func recordBroadcastJobHistoryQuarantine(ctx context.Context, journal BroadcastJobHistoryImportJournal, row v1archive.ArchivedRow, reason string) (bool, error) {
	if journal == nil || reason == "" {
		return false, ErrInvalidScope
	}
	source := SourceIdentifier(row.SourceKeyHMAC)
	existing, found, err := journal.LoadTerminal(ctx, source)
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != ([sha256.Size]byte{}) || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.Record(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyBroadcastJobHistoryReceipt(ctx context.Context, journal BroadcastJobHistoryImportJournal, row v1archive.ArchivedRow, receipt outboundport.BroadcastJobHistoryReceipt) error {
	if journal == nil || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := journal.LoadTerminal(ctx, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func broadcastJobHistoryReason(reason string) string {
	if reason == "" {
		return "invalid_broadcast_job_history"
	}
	return reason
}
