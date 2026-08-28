package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	customerStateHistoryImportVersion = "v1-customer-state-history-a1"
	customerStateHistoryDomain        = "contact"

	customerStateHistorySnapshotKind = "customer_status_snapshot"
	customerStateHistoryChangeKind   = "customer_status_change"
	customerStateHistoryTermKind     = "class_term_tag_mapping"

	customerStateHistorySnapshotTable = "public/class_user_status_current"
	customerStateHistoryChangeTable   = "public/class_user_status_history"
	customerStateHistoryTermTable     = "public/class_term_tag_mapping"

	customerStateHistorySnapshotTarget = "contact_v1_customer_status_snapshots"
	customerStateHistoryChangeTarget   = "contact_v1_customer_status_changes"
	customerStateHistoryTermTarget     = "contact_v1_class_term_tag_history"
)

var customerStateHistoryScopes = [...]struct{ kind, table, target string }{
	{customerStateHistorySnapshotKind, customerStateHistorySnapshotTable, customerStateHistorySnapshotTarget},
	{customerStateHistoryChangeKind, customerStateHistoryChangeTable, customerStateHistoryChangeTarget},
	{customerStateHistoryTermKind, customerStateHistoryTermTable, customerStateHistoryTermTarget},
}

// CustomerStateHistoryImportJournal combines owner receipts and generic
// terminal receipts. Both use the caller transaction.
type CustomerStateHistoryImportJournal interface {
	contactport.CustomerStateHistoryJournal
	LoadCustomerStateHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordCustomerStateHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateCustomerStateHistoryImportScope(string) error
}

type CustomerStateHistoryJournal struct{ journals map[string]*Journal }

var _ CustomerStateHistoryImportJournal = (*CustomerStateHistoryJournal)(nil)

func NewCustomerStateHistoryJournal(snapshot, change, term *Journal) (*CustomerStateHistoryJournal, error) {
	values := map[string]*Journal{
		customerStateHistorySnapshotKind: snapshot,
		customerStateHistoryChangeKind:   change,
		customerStateHistoryTermKind:     term,
	}
	if !validCustomerStateHistoryJournals(values) {
		return nil, ErrInvalidScope
	}
	return &CustomerStateHistoryJournal{journals: values}, nil
}

func (journal *CustomerStateHistoryJournal) ValidateCustomerStateHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validCustomerStateHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range customerStateHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *CustomerStateHistoryJournal) LoadCustomerStateHistory(ctx context.Context, kind, source string) (contactport.CustomerStateHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadCustomerStateHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return contactport.CustomerStateHistoryReceipt{}, found, err
	}
	receipt, err := customerStateHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *CustomerStateHistoryJournal) RecordCustomerStateHistory(ctx context.Context, receipt contactport.CustomerStateHistoryReceipt) error {
	terminal, err := customerStateHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordCustomerStateHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *CustomerStateHistoryJournal) LoadCustomerStateHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
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

func (journal *CustomerStateHistoryJournal) RecordCustomerStateHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *CustomerStateHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil || !validCustomerStateHistoryKind(kind) || !validCustomerStateHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func customerStateHistoryReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (contactport.CustomerStateHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validCustomerStateHistoryKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return contactport.CustomerStateHistoryReceipt{}, ErrConflict
	}
	return contactport.CustomerStateHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func customerStateHistoryTerminalFromReceipt(receipt contactport.CustomerStateHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validCustomerStateHistoryKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validCustomerStateHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(customerStateHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range customerStateHistoryScopes {
		journal := journals[scope.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != customerStateHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != scope.table ||
			journal.scope.TargetDomain != customerStateHistoryDomain || journal.scope.TargetTable != scope.target {
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

func validCustomerStateHistoryKind(kind string) bool {
	for _, scope := range customerStateHistoryScopes {
		if scope.kind == kind {
			return true
		}
	}
	return false
}
