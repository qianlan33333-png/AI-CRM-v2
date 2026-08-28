package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

const outboundTaskHistoryTableID = "public/outbound_tasks"

var _ outboundport.OutboundTaskHistoryJournal = (*Journal)(nil)

// ValidateOutboundTaskHistoryImportScope pins these receipts to inert V1 task
// history. It deliberately cannot target the current outbound task table.
func (journal *Journal) ValidateOutboundTaskHistoryImportScope(archiveRunID string) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() ||
		journal.scope.ImportVersion != outboundTaskHistoryImportVersion || journal.scope.ArchiveRunID != archiveRunID ||
		journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != outboundTaskHistoryTableID ||
		journal.scope.TargetDomain != "outbound" || journal.scope.TargetTable != outboundTaskHistoryTargetTable {
		return ErrInvalidScope
	}
	return nil
}

func (journal *Journal) LoadOutboundTaskHistory(ctx context.Context, sourceIdentifier string) (outboundport.OutboundTaskHistoryReceipt, bool, error) {
	if journal == nil || ctx == nil || journal.ValidateOutboundTaskHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return outboundport.OutboundTaskHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return outboundport.OutboundTaskHistoryReceipt{}, found, err
	}
	receipt, err := outboundTaskHistoryReceiptFromTerminal(sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *Journal) RecordOutboundTaskHistory(ctx context.Context, receipt outboundport.OutboundTaskHistoryReceipt) error {
	if journal == nil || ctx == nil || journal.ValidateOutboundTaskHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return ErrInvalidScope
	}
	terminal, err := outboundTaskHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.Record(ctx, terminal)
}

func outboundTaskHistoryReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (outboundport.OutboundTaskHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	targetID, targetErr := positiveID(terminal.TargetID)
	if err != nil || targetErr != nil || sourceKey == ([sha256.Size]byte{}) || sourceKey != terminal.SourceKeyDigest || sourceIdentifier != SourceIdentifier(sourceKey) ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 || strconv.FormatInt(targetID, 10) != terminal.TargetID {
		return outboundport.OutboundTaskHistoryReceipt{}, ErrConflict
	}
	return outboundport.OutboundTaskHistoryReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest, TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func outboundTaskHistoryTerminalFromReceipt(receipt outboundport.OutboundTaskHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(sourceKey) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
