package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const (
	marketingStateHistoryImportVersion = "v1-marketing-state-history-a1"
	marketingStateHistoryDomain        = "segment"

	marketingStateSnapshotKind = "marketing_state_snapshot"
	marketingStateChangeKind   = "marketing_state_change"
	valueSegmentSnapshotKind   = "value_segment_snapshot"
	valueSegmentChangeKind     = "value_segment_change"

	marketingStateSnapshotTable = "public/customer_marketing_state_current"
	marketingStateChangeTable   = "public/customer_marketing_state_history"
	valueSegmentSnapshotTable   = "public/customer_value_segment_current"
	valueSegmentChangeTable     = "public/customer_value_segment_history"

	marketingStateSnapshotTarget = "segment_v1_marketing_state_snapshots"
	marketingStateChangeTarget   = "segment_v1_marketing_state_changes"
	valueSegmentSnapshotTarget   = "segment_v1_value_segment_snapshots"
	valueSegmentChangeTarget     = "segment_v1_value_segment_changes"
)

var marketingStateHistoryScopes = [...]struct{ kind, table, target string }{
	{marketingStateSnapshotKind, marketingStateSnapshotTable, marketingStateSnapshotTarget},
	{marketingStateChangeKind, marketingStateChangeTable, marketingStateChangeTarget},
	{valueSegmentSnapshotKind, valueSegmentSnapshotTable, valueSegmentSnapshotTarget},
	{valueSegmentChangeKind, valueSegmentChangeTable, valueSegmentChangeTarget},
}

type MarketingStateHistoryImportJournal interface {
	segmentport.MarketingStateHistoryJournal
	LoadMarketingStateHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordMarketingStateHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateMarketingStateHistoryImportScope(string) error
}

type MarketingStateHistoryJournal struct{ journals map[string]*Journal }

var _ MarketingStateHistoryImportJournal = (*MarketingStateHistoryJournal)(nil)

func NewMarketingStateHistoryJournal(snapshot, change, valueSnapshot, valueChange *Journal) (*MarketingStateHistoryJournal, error) {
	journals := map[string]*Journal{
		marketingStateSnapshotKind: snapshot,
		marketingStateChangeKind:   change,
		valueSegmentSnapshotKind:   valueSnapshot,
		valueSegmentChangeKind:     valueChange,
	}
	if !validMarketingStateHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &MarketingStateHistoryJournal{journals: journals}, nil
}

func (journal *MarketingStateHistoryJournal) ValidateMarketingStateHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validMarketingStateHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range marketingStateHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *MarketingStateHistoryJournal) LoadMarketingStateHistory(ctx context.Context, kind, source string) (segmentport.MarketingStateHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadMarketingStateHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return segmentport.MarketingStateHistoryReceipt{}, found, err
	}
	receipt, err := marketingStateHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *MarketingStateHistoryJournal) RecordMarketingStateHistory(ctx context.Context, receipt segmentport.MarketingStateHistoryReceipt) error {
	terminal, err := marketingStateHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordMarketingStateHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *MarketingStateHistoryJournal) LoadMarketingStateHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
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

func (journal *MarketingStateHistoryJournal) RecordMarketingStateHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *MarketingStateHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil || !validMarketingStateHistoryKind(kind) || !validMarketingStateHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func marketingStateHistoryReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (segmentport.MarketingStateHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validMarketingStateHistoryKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return segmentport.MarketingStateHistoryReceipt{}, ErrConflict
	}
	return segmentport.MarketingStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func marketingStateHistoryTerminalFromReceipt(receipt segmentport.MarketingStateHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validMarketingStateHistoryKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validMarketingStateHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(marketingStateHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range marketingStateHistoryScopes {
		journal := journals[scope.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != marketingStateHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != scope.table ||
			journal.scope.TargetDomain != marketingStateHistoryDomain || journal.scope.TargetTable != scope.target {
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

func validMarketingStateHistoryKind(kind string) bool {
	for _, scope := range marketingStateHistoryScopes {
		if scope.kind == kind {
			return true
		}
	}
	return false
}
