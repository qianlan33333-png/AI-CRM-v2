package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	DeferredIdentityHistoryImportVersion = "v1-deferred-identity-history-a1"
	DeferredIdentityHistoryDomain        = "contact"

	DeferredPersonHistoryKind   = "deferred_person"
	DeferredConflictHistoryKind = "deferred_identity_conflict"
	MissingRootIdentityKind     = "missing_root_identity"

	DeferredPersonHistoryTarget   = "contact_v1_deferred_person_history"
	DeferredConflictHistoryTarget = "contact_v1_deferred_identity_conflict_history"
	MissingRootIdentityTarget     = "contact_v1_missing_root_identity_history"
)

type deferredIdentityHistoryScope struct{ kind, table, target string }

var deferredIdentityHistoryScopes = [...]deferredIdentityHistoryScope{
	{DeferredPersonHistoryKind, v1deferredidentityhistory.PeopleTableID, DeferredPersonHistoryTarget},
	{DeferredConflictHistoryKind, v1deferredidentityhistory.IdentityConflictsTableID, DeferredConflictHistoryTarget},
	{MissingRootIdentityKind, v1deferredidentityhistory.ExternalContactIdentityMapID, MissingRootIdentityTarget},
}

// DeferredIdentityHistoryImportJournal joins the Contact-owned writer receipt
// and generic terminal receipt under the caller transaction.
type DeferredIdentityHistoryImportJournal interface {
	contactport.DeferredIdentityHistoryJournal
	LoadDeferredIdentityHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordDeferredIdentityHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateDeferredIdentityHistoryImportScope(string) error
}

type DeferredIdentityHistoryJournal struct{ journals map[string]*Journal }

var _ DeferredIdentityHistoryImportJournal = (*DeferredIdentityHistoryJournal)(nil)

func NewDeferredIdentityHistoryJournal(people, conflicts, missingRoots *Journal) (*DeferredIdentityHistoryJournal, error) {
	values := map[string]*Journal{
		DeferredPersonHistoryKind:   people,
		DeferredConflictHistoryKind: conflicts,
		MissingRootIdentityKind:     missingRoots,
	}
	if !validDeferredIdentityHistoryJournals(values) {
		return nil, ErrInvalidScope
	}
	return &DeferredIdentityHistoryJournal{journals: values}, nil
}

func (journal *DeferredIdentityHistoryJournal) ValidateDeferredIdentityHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validDeferredIdentityHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range deferredIdentityHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *DeferredIdentityHistoryJournal) LoadDeferredIdentityHistory(ctx context.Context, kind, source string) (contactport.DeferredIdentityHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadDeferredIdentityHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return contactport.DeferredIdentityHistoryReceipt{}, found, err
	}
	receipt, err := deferredIdentityHistoryReceipt(kind, source, terminal)
	if err != nil {
		return contactport.DeferredIdentityHistoryReceipt{}, false, err
	}
	return receipt, true, nil
}

func (journal *DeferredIdentityHistoryJournal) RecordDeferredIdentityHistory(ctx context.Context, receipt contactport.DeferredIdentityHistoryReceipt) error {
	terminal, err := deferredIdentityHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return journal.RecordDeferredIdentityHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *DeferredIdentityHistoryJournal) LoadDeferredIdentityHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
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

func (journal *DeferredIdentityHistoryJournal) RecordDeferredIdentityHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *DeferredIdentityHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil || !validDeferredIdentityHistoryKind(kind) || !validDeferredIdentityHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func deferredIdentityHistoryReceipt(kind, source string, terminal TerminalReceipt) (contactport.DeferredIdentityHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validDeferredIdentityHistoryKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return contactport.DeferredIdentityHistoryReceipt{}, ErrConflict
	}
	return contactport.DeferredIdentityHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func deferredIdentityHistoryTerminal(receipt contactport.DeferredIdentityHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validDeferredIdentityHistoryKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validDeferredIdentityHistoryKind(kind string) bool {
	for _, scope := range deferredIdentityHistoryScopes {
		if scope.kind == kind {
			return true
		}
	}
	return false
}

func validDeferredIdentityHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(deferredIdentityHistoryScopes) {
		return false
	}
	var archiveRun string
	for _, expected := range deferredIdentityHistoryScopes {
		journal := journals[expected.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != DeferredIdentityHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != expected.table || journal.scope.TargetDomain != DeferredIdentityHistoryDomain || journal.scope.TargetTable != expected.target {
			return false
		}
		if archiveRun == "" {
			archiveRun = journal.scope.ArchiveRunID
		} else if archiveRun != journal.scope.ArchiveRunID {
			return false
		}
	}
	return archiveRun != ""
}
