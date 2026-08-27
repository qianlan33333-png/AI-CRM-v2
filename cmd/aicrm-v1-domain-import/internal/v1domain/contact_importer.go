package v1domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	contactTagGroupsTable = "public/wecom_corp_tag_groups"
	contactTagsTable      = "public/wecom_corp_tags"
	contactBindingsTable  = "public/contact_tags"
)

// VerifiedDM01CustomerTagWriter resolves only an exact DM01 customer root.
// Main-line composition must derive and lock crm_user_identity source-key HMAC
// under the frozen DM01 key, then use the same writer for the final in-UoW
// verification called by HistoricalTagImportService.
type VerifiedDM01CustomerTagWriter interface {
	contactport.HistoricalTagCustomerVerifier
	ResolveVerifiedDM01Customer(context.Context, string) (contactport.CustomerID, error)
}

// ContactTagReceiptJournal is both the Contact-owned lineage journal and the
// V1 terminal receipt writer. Terminal records are scoped to their source
// table; it has no Provider or runtime capability.
type ContactTagReceiptJournal interface {
	contactport.HistoricalTagJournal
	RecordContactTagTerminal(context.Context, contactport.HistoricalTagSource, TerminalReceipt) error
}

type ContactTagImportResult struct {
	ImportedGroups   int
	ImportedTags     int
	ImportedBindings int
	ArchivedRows     int
	QuarantinedRows  int
	ReplayedRows     int
}

type ContactTagImporter struct {
	archive   ArchiveSource
	uow       UnitOfWork
	writer    *contactapp.HistoricalTagImportService
	journal   ContactTagReceiptJournal
	customers VerifiedDM01CustomerTagWriter
}

func NewContactTagImporter(archive ArchiveSource, uow UnitOfWork, store contactport.HistoricalTagStore, journal ContactTagReceiptJournal, customers VerifiedDM01CustomerTagWriter) (*ContactTagImporter, error) {
	if archive == nil || uow == nil || store == nil || journal == nil || customers == nil {
		return nil, ErrInvalidScope
	}
	return &ContactTagImporter{
		archive: archive, uow: uow, writer: contactapp.NewHistoricalTagImportService(uow, store, journal, customers),
		journal: journal, customers: customers,
	}, nil
}

type contactTagGroupJSON struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
}

type contactTagJSON struct {
	TagID      string     `json:"tag_id"`
	TagName    string     `json:"tag_name"`
	GroupID    string     `json:"group_id"`
	OrderIndex int32      `json:"order_index"`
	DeletedAt  *time.Time `json:"deleted_at"`
}

// contactTagBindingJSON intentionally excludes V1 userid: it is never an
// external-contact identifier and therefore cannot influence a V2 customer.
type contactTagBindingJSON struct {
	TagID     string    `json:"tag_id"`
	UnionID   string    `json:"unionid"`
	CreatedAt time.Time `json:"created_at"`
}

type contactTagBindingArchiveRow struct {
	archive v1archive.ArchivedRow
	source  contactTagBindingJSON
}

type contactTagBindingKey struct {
	unionID string
	tagID   string
}

func (importer *ContactTagImporter) Import(ctx context.Context, archiveRunID string) (ContactTagImportResult, error) {
	if importer == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil || importer.customers == nil || archiveRunID == "" {
		return ContactTagImportResult{}, ErrInvalidScope
	}
	result := ContactTagImportResult{}
	groupSourceKeys := map[string][32]byte{}
	if err := importer.archive.EachTableRow(ctx, archiveRunID, contactTagGroupsTable, func(row v1archive.ArchivedRow) error {
		if !validContactArchivedRow(row, contactTagGroupsTable) {
			return ErrInvalidScope
		}
		var source contactTagGroupJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagGroupSource, row, "quarantine", "malformed_tag_group")
		}
		if source.GroupID == "" || source.GroupName == "" {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagGroupSource, row, "quarantine", "invalid_tag_group")
		}
		if prior, exists := groupSourceKeys[source.GroupID]; exists && prior != row.SourceKeyHMAC {
			return ErrConflict
		}
		write, err := importer.writer.ImportGroup(ctx, contactport.HistoricalTagGroupRecord{Fact: contactTagFact(row), Name: source.GroupName})
		if errors.Is(err, contactport.ErrHistoricalTagInput) {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagGroupSource, row, "quarantine", "invalid_tag_group")
		}
		if err != nil {
			return classifyContactWriteError(err)
		}
		groupSourceKeys[source.GroupID] = row.SourceKeyHMAC
		result.ImportedGroups++
		if write.Replayed {
			result.ReplayedRows++
		}
		return nil
	}); err != nil {
		return ContactTagImportResult{}, err
	}
	if err := importer.archive.EachTableRow(ctx, archiveRunID, contactTagsTable, func(row v1archive.ArchivedRow) error {
		if !validContactArchivedRow(row, contactTagsTable) {
			return ErrInvalidScope
		}
		var source contactTagJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "quarantine", "malformed_tag")
		}
		if source.DeletedAt != nil {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "archive", "deleted_source_tag")
		}
		if source.TagID == "" || source.TagName == "" || source.GroupID == "" {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "quarantine", "invalid_tag")
		}
		groupKey, found := groupSourceKeys[source.GroupID]
		if !found {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "quarantine", "tag_group_unresolved")
		}
		write, err := importer.writer.ImportTag(ctx, contactport.HistoricalTagRecord{Fact: contactTagFact(row), GroupSourceKeyDigest: groupKey, ProviderTagID: source.TagID, Name: source.TagName, SortOrder: source.OrderIndex})
		if errors.Is(err, contactport.ErrHistoricalTagBlocked) {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "quarantine", "tag_group_unresolved")
		}
		if errors.Is(err, contactport.ErrHistoricalTagInput) {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalTagCatalogTagSource, row, "quarantine", "invalid_tag")
		}
		if err != nil {
			return classifyContactWriteError(err)
		}
		result.ImportedTags++
		if write.Replayed {
			result.ReplayedRows++
		}
		return nil
	}); err != nil {
		return ContactTagImportResult{}, err
	}
	bindingRows := make([]contactTagBindingArchiveRow, 0)
	if err := importer.archive.EachTableRow(ctx, archiveRunID, contactBindingsTable, func(row v1archive.ArchivedRow) error {
		if !validContactArchivedRow(row, contactBindingsTable) {
			return ErrInvalidScope
		}
		var source contactTagBindingJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalCustomerTagSource, row, "quarantine", "malformed_contact_tag")
		}
		if source.TagID == "" || source.UnionID == "" || source.CreatedAt.IsZero() {
			return importer.recordContactTerminal(ctx, &result, contactport.HistoricalCustomerTagSource, row, "quarantine", "invalid_contact_tag")
		}
		bindingRows = append(bindingRows, contactTagBindingArchiveRow{archive: row, source: source})
		return nil
	}); err != nil {
		return ContactTagImportResult{}, err
	}
	selectedBindings := selectContactTagBindings(bindingRows)
	for index, binding := range bindingRows {
		if selectedBindings[contactTagBindingKey{unionID: binding.source.UnionID, tagID: binding.source.TagID}] != index {
			if err := importer.recordContactTerminal(ctx, &result, contactport.HistoricalCustomerTagSource, binding.archive, "archive", "duplicate_customer_tag_binding"); err != nil {
				return ContactTagImportResult{}, err
			}
			continue
		}
		if err := importer.importContactTagBinding(ctx, &result, binding); err != nil {
			return ContactTagImportResult{}, err
		}
	}
	return result, nil
}

func selectContactTagBindings(rows []contactTagBindingArchiveRow) map[contactTagBindingKey]int {
	selected := make(map[contactTagBindingKey]int, len(rows))
	for index, row := range rows {
		key := contactTagBindingKey{unionID: row.source.UnionID, tagID: row.source.TagID}
		current, found := selected[key]
		if !found || contactTagBindingEarlier(row, rows[current]) {
			selected[key] = index
		}
	}
	return selected
}

func contactTagBindingEarlier(left, right contactTagBindingArchiveRow) bool {
	return left.source.CreatedAt.Before(right.source.CreatedAt) ||
		(left.source.CreatedAt.Equal(right.source.CreatedAt) && left.archive.SourceOrdinal < right.archive.SourceOrdinal)
}

func (importer *ContactTagImporter) importContactTagBinding(ctx context.Context, result *ContactTagImportResult, binding contactTagBindingArchiveRow) error {
	customerID, err := importer.customers.ResolveVerifiedDM01Customer(ctx, binding.source.UnionID)
	if errors.Is(err, contactport.ErrHistoricalTagBlocked) || (err == nil && customerID < 1) {
		return importer.recordContactTerminal(ctx, result, contactport.HistoricalCustomerTagSource, binding.archive, "quarantine", "dm01_customer_unresolved")
	}
	if err != nil {
		return fmt.Errorf("resolve DM01 customer: %w", err)
	}
	write, err := importer.writer.ImportCustomerTag(ctx, contactport.HistoricalCustomerTagRecord{Fact: contactTagFact(binding.archive), UnionID: binding.source.UnionID, VerifiedCustomerID: customerID, ProviderTagID: binding.source.TagID, TaggedAt: binding.source.CreatedAt})
	if errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		return importer.recordContactTerminal(ctx, result, contactport.HistoricalCustomerTagSource, binding.archive, "quarantine", "dm01_or_tag_unresolved")
	}
	if errors.Is(err, contactport.ErrHistoricalTagInput) {
		return importer.recordContactTerminal(ctx, result, contactport.HistoricalCustomerTagSource, binding.archive, "quarantine", "invalid_contact_tag")
	}
	if err != nil {
		return classifyContactWriteError(err)
	}
	result.ImportedBindings++
	if write.Replayed {
		result.ReplayedRows++
	}
	return nil
}

func contactTagFact(row v1archive.ArchivedRow) contactport.HistoricalTagFact {
	return contactport.HistoricalTagFact{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, FieldDigest: row.FieldHMAC}
}

func validContactArchivedRow(row v1archive.ArchivedRow, table string) bool {
	return row.AdapterID == v1archive.DefaultAdapterID && row.TableID == table &&
		row.SourceKeyHMAC != ([32]byte{}) && row.PayloadHMAC != ([32]byte{}) && row.FieldHMAC != ([32]byte{})
}

func (importer *ContactTagImporter) recordContactTerminal(ctx context.Context, result *ContactTagImportResult, source contactport.HistoricalTagSource, row v1archive.ArchivedRow, disposition, reason string) error {
	if disposition == "archive" {
		// Archive is an intentional no-op over a stale Provider fact.
	} else if disposition != "quarantine" {
		return ErrInvalidScope
	}
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		return importer.journal.RecordContactTagTerminal(tx, source, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: disposition, Reason: reason})
	})
	if err == nil && result != nil {
		if disposition == "archive" {
			result.ArchivedRows++
		} else {
			result.QuarantinedRows++
		}
	}
	return err
}

func classifyContactWriteError(err error) error {
	if errors.Is(err, contactport.ErrHistoricalTagConflict) {
		return ErrConflict
	}
	if err == nil {
		return nil
	}
	return fmt.Errorf("write historical contact tag: %w", err)
}
