package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const (
	messageHistoryImportVersion = "v1-message-history-a1"
	messageHistoryTableID       = "public/archived_messages"
	messageHistoryTargetTable   = "wecom_v1_message_history"
)

var _ wecomport.MessageHistoryJournal = (*Journal)(nil)

// ValidateMessageHistoryImportScope pins this journal to the one immutable V1
// message source and its read-only WeCom projection.
func (journal *Journal) ValidateMessageHistoryImportScope(archiveRunID string) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() ||
		journal.scope.ImportVersion != messageHistoryImportVersion || journal.scope.ArchiveRunID != archiveRunID ||
		journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != messageHistoryTableID ||
		journal.scope.TargetDomain != "wecom" || journal.scope.TargetTable != messageHistoryTargetTable {
		return ErrInvalidScope
	}
	return nil
}

func (journal *Journal) LoadMessageHistory(ctx context.Context, sourceIdentifier string) (wecomport.MessageHistoryReceipt, bool, error) {
	if journal == nil || ctx == nil || journal.ValidateMessageHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return wecomport.MessageHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return wecomport.MessageHistoryReceipt{}, found, err
	}
	receipt, err := messageHistoryReceiptFromTerminal(sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *Journal) RecordMessageHistory(ctx context.Context, receipt wecomport.MessageHistoryReceipt) error {
	if journal == nil || ctx == nil || journal.ValidateMessageHistoryImportScope(journal.scope.ArchiveRunID) != nil {
		return ErrInvalidScope
	}
	terminal, err := messageHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.Record(ctx, terminal)
}

func messageHistoryReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (wecomport.MessageHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	targetID, targetErr := positiveID(terminal.TargetID)
	if err != nil || targetErr != nil || sourceKey == ([sha256.Size]byte{}) || sourceKey != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || len(terminal.Metadata) != 0 || strconv.FormatInt(targetID, 10) != terminal.TargetID {
		return wecomport.MessageHistoryReceipt{}, ErrConflict
	}
	return wecomport.MessageHistoryReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest,
		TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func messageHistoryTerminalFromReceipt(receipt wecomport.MessageHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}
