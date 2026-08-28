package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

const profileCatalogHistoryImportVersion = "v1-profile-catalog-history-a1"

var profileCatalogHistoryScopes = []struct{ kind, source, domain, target string }{
	{v1profilecatalog.ProfileTemplatesKind, v1profilecatalog.ProfileTemplatesTableID, "segment", v1profilecatalog.ProfileTemplatesTargetTable},
	{v1profilecatalog.ProfileCategoriesKind, v1profilecatalog.ProfileCategoriesTableID, "segment", v1profilecatalog.ProfileCategoriesTargetTable},
	{v1profilecatalog.ProfileOptionMappingsKind, v1profilecatalog.ProfileOptionMappingsTableID, "segment", v1profilecatalog.ProfileOptionMappingsTargetTable},
	{v1profilecatalog.SignupTagRulesKind, v1profilecatalog.SignupTagRulesTableID, "contact", v1profilecatalog.SignupTagRulesTargetTable},
}

// ProfileCatalogHistoryJournal dispatches only Segment-owned history facts to
// their individual source receipts. It shares the caller transaction with the
// target writer and never records executable profile rules.
type ProfileCatalogHistoryJournal struct{ journals map[string]*Journal }

var _ segmentport.ProfileCatalogHistoryJournal = (*ProfileCatalogHistoryJournal)(nil)

func NewProfileCatalogHistoryJournal(templates, categories, mappings *Journal) (*ProfileCatalogHistoryJournal, error) {
	values := map[string]*Journal{
		v1profilecatalog.ProfileTemplatesTableID:      templates,
		v1profilecatalog.ProfileCategoriesTableID:     categories,
		v1profilecatalog.ProfileOptionMappingsTableID: mappings,
	}
	if !validProfileCatalogSegmentJournals(values) {
		return nil, ErrInvalidScope
	}
	return &ProfileCatalogHistoryJournal{journals: values}, nil
}

func (journal *ProfileCatalogHistoryJournal) LoadProfileCatalogHistory(ctx context.Context, kind, source string) (segmentport.ProfileCatalogHistoryReceipt, bool, error) {
	selected, err := journal.journalForKind(kind)
	if err != nil {
		return segmentport.ProfileCatalogHistoryReceipt{}, false, err
	}
	terminal, found, err := selected.LoadTerminal(ctx, source)
	if err != nil || !found {
		return segmentport.ProfileCatalogHistoryReceipt{}, found, err
	}
	return profileCatalogReceiptFromTerminal(kind, source, terminal)
}

func (journal *ProfileCatalogHistoryJournal) RecordProfileCatalogHistory(ctx context.Context, receipt segmentport.ProfileCatalogHistoryReceipt) error {
	selected, err := journal.journalForKind(receipt.Kind)
	if err != nil {
		return err
	}
	terminal, err := profileCatalogTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *ProfileCatalogHistoryJournal) journalForKind(kind string) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	for _, scope := range profileCatalogHistoryScopes[:3] {
		if scope.kind == kind {
			selected := journal.journals[scope.source]
			if validProfileCatalogHistoryScope(selected, scope) {
				return selected, nil
			}
		}
	}
	return nil, ErrInvalidScope
}

// SignupTagHistoryJournal is separate because Contact owns this frozen rule
// table. It shares the same source run but cannot receive Segment receipts.
type SignupTagHistoryJournal struct{ rules *Journal }

var _ contactport.SignupTagHistoryJournal = (*SignupTagHistoryJournal)(nil)

func NewSignupTagHistoryJournal(rules *Journal) (*SignupTagHistoryJournal, error) {
	scope := profileCatalogHistoryScopes[3]
	if !validProfileCatalogHistoryScope(rules, scope) {
		return nil, ErrInvalidScope
	}
	return &SignupTagHistoryJournal{rules: rules}, nil
}

func (journal *SignupTagHistoryJournal) LoadSignupTagHistory(ctx context.Context, source string) (contactport.SignupTagHistoryReceipt, bool, error) {
	if journal == nil || !validProfileCatalogHistoryScope(journal.rules, profileCatalogHistoryScopes[3]) {
		return contactport.SignupTagHistoryReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.rules.LoadTerminal(ctx, source)
	if err != nil || !found {
		return contactport.SignupTagHistoryReceipt{}, found, err
	}
	return signupTagReceiptFromTerminal(source, terminal)
}

func (journal *SignupTagHistoryJournal) RecordSignupTagHistory(ctx context.Context, receipt contactport.SignupTagHistoryReceipt) error {
	if journal == nil || !validProfileCatalogHistoryScope(journal.rules, profileCatalogHistoryScopes[3]) {
		return ErrInvalidScope
	}
	terminal, err := signupTagTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.rules.Record(ctx, terminal)
}

func validProfileCatalogHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(profileCatalogHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range profileCatalogHistoryScopes {
		journal := journals[scope.source]
		if !validProfileCatalogHistoryScope(journal, scope) {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return true
}

func validProfileCatalogSegmentJournals(journals map[string]*Journal) bool {
	if len(journals) != 3 {
		return false
	}
	var run string
	for _, scope := range profileCatalogHistoryScopes[:3] {
		journal := journals[scope.source]
		if !validProfileCatalogHistoryScope(journal, scope) {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return true
}

func validProfileCatalogHistoryScope(journal *Journal, expected struct{ kind, source, domain, target string }) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() && journal.scope.ImportVersion == profileCatalogHistoryImportVersion &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID && journal.scope.TableID == expected.source &&
		journal.scope.TargetDomain == expected.domain && journal.scope.TargetTable == expected.target
}

func profileCatalogReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (segmentport.ProfileCatalogHistoryReceipt, bool, error) {
	if !validProfileCatalogKind(kind) {
		return segmentport.ProfileCatalogHistoryReceipt{}, false, ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(source)
	id, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID || key == ([sha256.Size]byte{}) ||
		terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 {
		return segmentport.ProfileCatalogHistoryReceipt{}, false, ErrConflict
	}
	return segmentport.ProfileCatalogHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}

func profileCatalogTerminalFromReceipt(receipt segmentport.ProfileCatalogHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validProfileCatalogKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) ||
		receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func signupTagReceiptFromTerminal(source string, terminal TerminalReceipt) (contactport.SignupTagHistoryReceipt, bool, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID || key == ([sha256.Size]byte{}) ||
		terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 {
		return contactport.SignupTagHistoryReceipt{}, false, ErrConflict
	}
	return contactport.SignupTagHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, true, nil
}

func signupTagTerminalFromReceipt(receipt contactport.SignupTagHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || key == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validProfileCatalogKind(kind string) bool {
	return kind == v1profilecatalog.ProfileTemplatesKind || kind == v1profilecatalog.ProfileCategoriesKind || kind == v1profilecatalog.ProfileOptionMappingsKind
}
