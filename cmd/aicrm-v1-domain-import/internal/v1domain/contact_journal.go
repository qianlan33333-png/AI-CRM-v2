package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// ContactTagJournal routes the three Contact-owned source tables through the
// existing immutable V1 receipt journal. It stores no payload or Provider
// material and introduces no second receipt schema.
type ContactTagJournal struct {
	groups   *Journal
	tags     *Journal
	bindings *Journal
}

var _ ContactTagReceiptJournal = (*ContactTagJournal)(nil)

func NewContactTagJournal(groups, tags, bindings *Journal) (*ContactTagJournal, error) {
	if !validContactTagScope(groups, contactTagGroupsTable, "tag_groups") ||
		!validContactTagScope(tags, contactTagsTable, "tags") ||
		!validContactTagScope(bindings, contactBindingsTable, "customer_tags") {
		return nil, ErrInvalidScope
	}
	return &ContactTagJournal{groups: groups, tags: tags, bindings: bindings}, nil
}

func validContactTagScope(journal *Journal, sourceTable, targetTable string) bool {
	return journal != nil && journal.scope.valid() && journal.scope.TableID == sourceTable &&
		journal.scope.TargetDomain == "contact" && journal.scope.TargetTable == targetTable
}

func (journal *ContactTagJournal) FindHistoricalTagLineage(ctx context.Context, source contactport.HistoricalTagSource, key [sha256.Size]byte) (contactport.HistoricalTagLineage, bool, error) {
	entry, err := journal.forSource(source)
	if err != nil {
		return contactport.HistoricalTagLineage{}, false, err
	}
	receipt, found, err := entry.LoadTerminal(ctx, SourceIdentifier(key))
	if err != nil || !found {
		return contactport.HistoricalTagLineage{}, found, err
	}
	lineage, err := contactTagLineageFromTerminal(receipt)
	if err != nil {
		return contactport.HistoricalTagLineage{}, false, err
	}
	return lineage, true, nil
}

func (journal *ContactTagJournal) AppendHistoricalTagLineage(ctx context.Context, source contactport.HistoricalTagSource, fact contactport.HistoricalTagFact, lineage contactport.HistoricalTagLineage) error {
	entry, err := journal.forSource(source)
	if err != nil {
		return err
	}
	if fact.SourceKeyDigest == ([sha256.Size]byte{}) || fact.PayloadDigest == ([sha256.Size]byte{}) || fact.FieldDigest == ([sha256.Size]byte{}) ||
		lineage.TargetID < 1 || lineage.TargetDigest == ([sha256.Size]byte{}) || lineage.PayloadDigest != fact.PayloadDigest || lineage.FieldDigest != fact.FieldDigest {
		return ErrInvalidScope
	}
	return entry.Record(ctx, TerminalReceipt{
		SourceKeyDigest: fact.SourceKeyDigest, PayloadDigest: fact.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(lineage.TargetID, 10), TargetDigest: lineage.TargetDigest,
		Metadata: map[string]any{"payload_digest": hex.EncodeToString(fact.PayloadDigest[:]), "field_digest": hex.EncodeToString(fact.FieldDigest[:])},
	})
}

func (journal *ContactTagJournal) RecordContactTagTerminal(ctx context.Context, source contactport.HistoricalTagSource, receipt TerminalReceipt) error {
	entry, err := journal.forSource(source)
	if err != nil {
		return err
	}
	return entry.Record(ctx, receipt)
}

func (journal *ContactTagJournal) forSource(source contactport.HistoricalTagSource) (*Journal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch source {
	case contactport.HistoricalTagGroupSource:
		return journal.groups, nil
	case contactport.HistoricalTagCatalogTagSource:
		return journal.tags, nil
	case contactport.HistoricalCustomerTagSource:
		return journal.bindings, nil
	default:
		return nil, ErrInvalidScope
	}
}

func contactTagLineageFromTerminal(receipt TerminalReceipt) (contactport.HistoricalTagLineage, error) {
	if receipt.Disposition != "import" || receipt.Reason != "" || receipt.TargetID == "" || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return contactport.HistoricalTagLineage{}, ErrConflict
	}
	targetID, err := strconv.ParseInt(receipt.TargetID, 10, 64)
	if err != nil || targetID < 1 {
		return contactport.HistoricalTagLineage{}, ErrConflict
	}
	payload, err := contactTagDigestMetadata(receipt.Metadata, "payload_digest")
	if err != nil || payload != receipt.PayloadDigest {
		return contactport.HistoricalTagLineage{}, ErrConflict
	}
	field, err := contactTagDigestMetadata(receipt.Metadata, "field_digest")
	if err != nil {
		return contactport.HistoricalTagLineage{}, ErrConflict
	}
	return contactport.HistoricalTagLineage{TargetID: targetID, TargetDigest: receipt.TargetDigest, PayloadDigest: payload, FieldDigest: field}, nil
}

func contactTagDigestMetadata(metadata map[string]any, field string) ([sha256.Size]byte, error) {
	value, ok := metadata[field].(string)
	if !ok || len(value) != sha256.Size*2 {
		return [sha256.Size]byte{}, errors.New("invalid contact tag receipt metadata")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, errors.New("invalid contact tag receipt metadata")
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return digest, nil
}
