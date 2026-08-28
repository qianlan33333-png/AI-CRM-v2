package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	legacyMarketingHistoryImportVersion = "v1-legacy-marketing-history-a1"
	legacyMarketingHistoryDomain        = "segment"

	legacyMarketingStateKind = "legacy_marketing_state"
	legacyMarketingValueKind = "legacy_marketing_value"

	legacyMarketingStateTable = "public/marketing_state_current"
	legacyMarketingValueTable = "public/marketing_value_segment_current"

	legacyMarketingStateTarget = "segment_v1_legacy_marketing_states"
	legacyMarketingValueTarget = "segment_v1_legacy_marketing_values"
)

var legacyMarketingHistoryScopes = [...]struct{ kind, table, target string }{
	{legacyMarketingStateKind, legacyMarketingStateTable, legacyMarketingStateTarget},
	{legacyMarketingValueKind, legacyMarketingValueTable, legacyMarketingValueTarget},
}

type LegacyMarketingHistoryImportJournal interface {
	segmentport.LegacyMarketingHistoryJournal
	LoadLegacyMarketingHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordLegacyMarketingHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateLegacyMarketingHistoryImportScope(string) error
}

type LegacyMarketingHistoryJournal struct{ journals map[string]*Journal }

var _ LegacyMarketingHistoryImportJournal = (*LegacyMarketingHistoryJournal)(nil)

func NewLegacyMarketingHistoryJournal(snapshot, valueSnapshot *Journal) (*LegacyMarketingHistoryJournal, error) {
	journals := map[string]*Journal{
		legacyMarketingStateKind: snapshot,
		legacyMarketingValueKind: valueSnapshot,
	}
	if !validLegacyMarketingHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &LegacyMarketingHistoryJournal{journals: journals}, nil
}

func (journal *LegacyMarketingHistoryJournal) ValidateLegacyMarketingHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validLegacyMarketingHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range legacyMarketingHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *LegacyMarketingHistoryJournal) LoadLegacyMarketingHistory(ctx context.Context, kind, source string) (segmentport.LegacyMarketingHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadLegacyMarketingHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return segmentport.LegacyMarketingHistoryReceipt{}, found, err
	}
	receipt, err := legacyMarketingHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *LegacyMarketingHistoryJournal) RecordLegacyMarketingHistory(ctx context.Context, receipt segmentport.LegacyMarketingHistoryReceipt) error {
	terminal, err := legacyMarketingHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordLegacyMarketingHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *LegacyMarketingHistoryJournal) LoadLegacyMarketingHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(source)
	if err != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, source)
}

func (journal *LegacyMarketingHistoryJournal) RecordLegacyMarketingHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *LegacyMarketingHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil || !validLegacyMarketingHistoryKind(kind) || !validLegacyMarketingHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func legacyMarketingHistoryReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (segmentport.LegacyMarketingHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validLegacyMarketingHistoryKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return segmentport.LegacyMarketingHistoryReceipt{}, ErrConflict
	}
	return segmentport.LegacyMarketingHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func legacyMarketingHistoryTerminalFromReceipt(receipt segmentport.LegacyMarketingHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validLegacyMarketingHistoryKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validLegacyMarketingHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(legacyMarketingHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range legacyMarketingHistoryScopes {
		journal := journals[scope.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != legacyMarketingHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != scope.table ||
			journal.scope.TargetDomain != legacyMarketingHistoryDomain || journal.scope.TargetTable != scope.target {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return run != ""
}

func validLegacyMarketingHistoryKind(kind string) bool {
	for _, scope := range legacyMarketingHistoryScopes {
		if scope.kind == kind {
			return true
		}
	}
	return false
}
