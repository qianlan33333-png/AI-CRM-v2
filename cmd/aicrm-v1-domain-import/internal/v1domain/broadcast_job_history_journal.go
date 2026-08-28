package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

const (
	broadcastJobHistoryImportVersion = "v1-broadcast-job-history-a1"
	broadcastJobHistoryTableID       = "public/broadcast_jobs"
	broadcastJobHistoryTargetTable   = "outbound_v1_broadcast_job_history"
)

var _ outboundport.BroadcastJobHistoryJournal = (*Journal)(nil)

// ValidateBroadcastJobHistoryImportScope pins this journal to the one immutable V1
// broadcast job source and its inert Outbound history.
func (journal *Journal) ValidateBroadcastJobHistoryImportScope(archiveRunID string) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() ||
		journal.scope.ImportVersion != broadcastJobHistoryImportVersion || journal.scope.ArchiveRunID != archiveRunID ||
		journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != broadcastJobHistoryTableID ||
		journal.scope.TargetDomain != "outbound" || journal.scope.TargetTable != broadcastJobHistoryTargetTable {
		return ErrInvalidScope
	}
	return nil
}

func (journal *Journal) LoadBroadcastJobHistory(ctx context.Context, sourceIdentifier string) (outboundport.BroadcastJobHistoryReceipt, bool, error) {
	if journal == nil || ctx == nil || journal.ValidateBroadcastJobHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return outboundport.BroadcastJobHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return outboundport.BroadcastJobHistoryReceipt{}, found, err
	}
	receipt, err := broadcastJobHistoryReceiptFromTerminal(sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *Journal) RecordBroadcastJobHistory(ctx context.Context, receipt outboundport.BroadcastJobHistoryReceipt) error {
	if journal == nil || ctx == nil || journal.ValidateBroadcastJobHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return ErrInvalidScope
	}
	terminal, err := broadcastJobHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.Record(ctx, terminal)
}

func broadcastJobHistoryReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (outboundport.BroadcastJobHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	targetID, targetErr := positiveID(terminal.TargetID)
	if err != nil || targetErr != nil || sourceKey == ([sha256.Size]byte{}) || sourceKey != terminal.SourceKeyDigest || sourceIdentifier != SourceIdentifier(sourceKey) ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 || strconv.FormatInt(targetID, 10) != terminal.TargetID {
		return outboundport.BroadcastJobHistoryReceipt{}, ErrConflict
	}
	return outboundport.BroadcastJobHistoryReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest,
		TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func broadcastJobHistoryTerminalFromReceipt(receipt outboundport.BroadcastJobHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(sourceKey) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
