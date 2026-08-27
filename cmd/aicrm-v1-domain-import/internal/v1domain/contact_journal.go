package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
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
		!validContactTagScope(bindings, contactBindingsTable, "customer_tags") ||
		!sameContactTagJournalRun(groups, tags) || !sameContactTagJournalRun(groups, bindings) {
		return nil, ErrInvalidScope
	}
	return &ContactTagJournal{groups: groups, tags: tags, bindings: bindings}, nil
}

func validContactTagScope(journal *Journal, sourceTable, targetTable string) bool {
	return journal != nil && journal.scope.valid() && journal.scope.TableID == sourceTable &&
		journal.scope.TargetDomain == "contact" && journal.scope.TargetTable == targetTable &&
		journal.scope.AdapterID == v1archive.DefaultAdapterID
}

func sameContactTagJournalRun(left, right *Journal) bool {
	return left != nil && right != nil && left.scope.ImportVersion == right.scope.ImportVersion &&
		left.scope.ArchiveRunID == right.scope.ArchiveRunID && left.scope.AdapterID == right.scope.AdapterID
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
	lineage, err := contactTagLineageFromTerminal(source, receipt)
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
	if source == contactport.HistoricalCustomerTagSource && lineage.CustomerID < 1 {
		return ErrInvalidScope
	}
	if source != contactport.HistoricalCustomerTagSource && lineage.CustomerID != 0 {
		return ErrInvalidScope
	}
	metadata := map[string]any{"payload_digest": hex.EncodeToString(fact.PayloadDigest[:]), "field_digest": hex.EncodeToString(fact.FieldDigest[:])}
	if source == contactport.HistoricalCustomerTagSource {
		metadata["customer_id"] = strconv.FormatInt(int64(lineage.CustomerID), 10)
	}
	return entry.Record(ctx, TerminalReceipt{
		SourceKeyDigest: fact.SourceKeyDigest, PayloadDigest: fact.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(lineage.TargetID, 10), TargetDigest: lineage.TargetDigest,
		Metadata: metadata,
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

func contactTagLineageFromTerminal(source contactport.HistoricalTagSource, receipt TerminalReceipt) (contactport.HistoricalTagLineage, error) {
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
	lineage := contactport.HistoricalTagLineage{TargetID: targetID, TargetDigest: receipt.TargetDigest, PayloadDigest: payload, FieldDigest: field}
	if source == contactport.HistoricalCustomerTagSource {
		customerID, err := contactTagCustomerIDMetadata(receipt.Metadata)
		if err != nil || customerID < 1 {
			return contactport.HistoricalTagLineage{}, ErrConflict
		}
		lineage.CustomerID = customerID
	} else if source != contactport.HistoricalTagGroupSource && source != contactport.HistoricalTagCatalogTagSource {
		return contactport.HistoricalTagLineage{}, ErrInvalidScope
	} else if _, present := receipt.Metadata["customer_id"]; present {
		return contactport.HistoricalTagLineage{}, ErrConflict
	}
	return lineage, nil
}

func contactTagCustomerIDMetadata(metadata map[string]any) (contactport.CustomerID, error) {
	value, ok := metadata["customer_id"].(string)
	if !ok {
		return 0, errors.New("invalid contact tag customer metadata")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return 0, errors.New("invalid contact tag customer metadata")
	}
	return contactport.CustomerID(parsed), nil
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
