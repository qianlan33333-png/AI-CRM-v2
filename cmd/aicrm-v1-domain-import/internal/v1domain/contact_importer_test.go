package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestContactTagImporterImportsLocalFactsWithVerifiedDM01Customer(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": "Lifecycle"})},
		contactTagsTable:      {contactArchivedRow(contactTagsTable, 2, map[string]any{"tag_id": "tag-1", "tag_name": "Paid", "group_id": "group-1", "order_index": 3})},
		contactBindingsTable:  {contactArchivedRow(contactBindingsTable, 3, map[string]any{"userid": "must-not-map", "tag_id": "tag-1", "unionid": "union-1", "created_at": stamp})},
	}}
	journal := newContactMemoryJournal()
	customers := &contactDM01Verifier{targets: map[string]contactport.CustomerID{"union-1": 19}}
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), journal, customers)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.ImportedGroups != 1 || result.ImportedTags != 1 || result.ImportedBindings != 1 || result.ArchivedRows != 0 || result.QuarantinedRows != 0 || customers.resolveCalls != 1 || customers.verifyCalls != 1 {
		t.Fatalf("result=%+v err=%v resolver=%d verifier=%d", result, err, customers.resolveCalls, customers.verifyCalls)
	}
	if len(journal.lineage) != 3 || len(journal.terminal) != 0 {
		t.Fatalf("lineage=%d terminal=%+v", len(journal.lineage), journal.terminal)
	}
}

func TestContactTagImporterArchivesDeletedTagAndQuarantinesBindingWithoutVerifiedTag(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	deleted := stamp.Add(-time.Hour)
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {},
		contactTagsTable:      {contactArchivedRow(contactTagsTable, 2, map[string]any{"tag_id": "tag-1", "tag_name": "Paid", "group_id": "group-1", "deleted_at": deleted})},
		contactBindingsTable:  {contactArchivedRow(contactBindingsTable, 3, map[string]any{"userid": "must-not-map", "tag_id": "tag-1", "unionid": "union-1", "created_at": stamp})},
	}}
	journal := newContactMemoryJournal()
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), journal, &contactDM01Verifier{targets: map[string]contactport.CustomerID{"union-1": 19}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.ArchivedRows != 1 || result.QuarantinedRows != 1 || result.ImportedTags != 0 || result.ImportedBindings != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(journal.terminal) != 2 || journal.terminal[0].Disposition != "archive" || journal.terminal[0].Reason != "deleted_source_tag" || journal.terminal[1].Reason != "dm01_or_tag_unresolved" {
		t.Fatalf("terminal=%+v", journal.terminal)
	}
}

func TestContactTagImporterQuarantinesWhitespaceOnlyGroup(t *testing.T) {
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": " \t "})},
	}}
	journal := newContactMemoryJournal()
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), journal, &contactDM01Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.QuarantinedRows != 1 || len(journal.terminal) != 1 || journal.terminal[0].Reason != "invalid_tag_group" {
		t.Fatalf("result=%+v err=%v terminal=%+v", result, err, journal.terminal)
	}
}

func TestContactTagImporterStopsForTransientDM01ResolverFailure(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactBindingsTable: {contactArchivedRow(contactBindingsTable, 3, map[string]any{"tag_id": "tag-1", "unionid": "union-1", "created_at": stamp})},
	}}
	journal := newContactMemoryJournal()
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), journal, &contactDM01Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "run"); err == nil || len(journal.terminal) != 0 {
		t.Fatalf("transient resolver became terminal: err=%v terminal=%+v", err, journal.terminal)
	}
}

func TestContactTagImporterRejectsUnauthenticatedArchiveRow(t *testing.T) {
	row := contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": "Lifecycle"})
	row.AdapterID = "untrusted"
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{contactTagGroupsTable: {row}}}
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), newContactMemoryJournal(), &contactDM01Verifier{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("unauthenticated row err=%v", err)
	}
}

func TestContactTagImporterArchivesDuplicateBindingsAndKeepsEarliestPerExactKey(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	late := contactArchivedRow(contactBindingsTable, 10, map[string]any{"tag_id": "tag-1", "unionid": "union-a", "created_at": stamp.Add(time.Hour)})
	early := contactArchivedRow(contactBindingsTable, 11, map[string]any{"tag_id": "tag-1", "unionid": "union-a", "created_at": stamp})
	otherUnion := contactArchivedRow(contactBindingsTable, 12, map[string]any{"tag_id": "tag-1", "unionid": "union-b", "created_at": stamp.Add(2 * time.Hour)})
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": "Lifecycle"})},
		contactTagsTable:      {contactArchivedRow(contactTagsTable, 2, map[string]any{"tag_id": "tag-1", "tag_name": "Paid", "group_id": "group-1"})},
		contactBindingsTable:  {late, early, otherUnion},
	}}
	journal := newContactMemoryJournal()
	store := newContactMemoryStore()
	customers := &contactDM01Verifier{targets: map[string]contactport.CustomerID{"union-a": 19, "union-b": 20}}
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, store, journal, customers)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.ImportedBindings != 2 || result.ArchivedRows != 1 || result.QuarantinedRows != 0 || customers.resolveCalls != 2 || customers.verifyCalls != 2 {
		t.Fatalf("result=%+v err=%v resolver=%d verifier=%d", result, err, customers.resolveCalls, customers.verifyCalls)
	}
	if terminal := journal.terminal[0]; terminal.Disposition != "archive" || terminal.Reason != "duplicate_customer_tag_binding" || terminal.SourceKeyDigest != late.SourceKeyHMAC {
		t.Fatalf("duplicate terminal=%+v", terminal)
	}
	if len(store.customerTags) != 2 || !store.customerTags[contactCustomerTagKey(19, 2)].TaggedAt.Equal(stamp) || !store.customerTags[contactCustomerTagKey(20, 2)].TaggedAt.Equal(stamp.Add(2*time.Hour)) {
		t.Fatalf("customer tags=%+v", store.customerTags)
	}
	if _, found := journal.lineage[contactJournalKey(contactport.HistoricalCustomerTagSource, early.SourceKeyHMAC)]; !found {
		t.Fatal("earliest binding was not imported")
	}
	if _, found := journal.lineage[contactJournalKey(contactport.HistoricalCustomerTagSource, late.SourceKeyHMAC)]; found {
		t.Fatal("later duplicate was imported")
	}
	if result.ImportedBindings+result.ArchivedRows != 3 {
		t.Fatalf("valid binding conservation failed: %+v", result)
	}
}

func TestContactTagImporterBreaksEqualTimeBindingTiesBySourceOrdinal(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	higherOrdinal := contactArchivedRow(contactBindingsTable, 21, map[string]any{"tag_id": "tag-1", "unionid": "union-a", "created_at": stamp})
	lowerOrdinal := contactArchivedRow(contactBindingsTable, 22, map[string]any{"tag_id": "tag-1", "unionid": "union-a", "created_at": stamp})
	lowerOrdinal.SourceOrdinal = 20
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": "Lifecycle"})},
		contactTagsTable:      {contactArchivedRow(contactTagsTable, 2, map[string]any{"tag_id": "tag-1", "tag_name": "Paid", "group_id": "group-1"})},
		contactBindingsTable:  {higherOrdinal, lowerOrdinal},
	}}
	journal := newContactMemoryJournal()
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, newContactMemoryStore(), journal, &contactDM01Verifier{targets: map[string]contactport.CustomerID{"union-a": 19}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "run")
	if err != nil || result.ImportedBindings != 1 || result.ArchivedRows != 1 || len(journal.terminal) != 1 || journal.terminal[0].SourceKeyDigest != higherOrdinal.SourceKeyHMAC {
		t.Fatalf("result=%+v err=%v terminal=%+v", result, err, journal.terminal)
	}
	if _, found := journal.lineage[contactJournalKey(contactport.HistoricalCustomerTagSource, lowerOrdinal.SourceKeyHMAC)]; !found {
		t.Fatal("lower source ordinal was not imported")
	}
}

func TestContactTagImporterDoesNotHideWriterConflictAsDuplicate(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 3, 0, 0, 0, time.UTC)
	archive := &contactArchive{rows: map[string][]v1archive.ArchivedRow{
		contactTagGroupsTable: {contactArchivedRow(contactTagGroupsTable, 1, map[string]any{"group_id": "group-1", "group_name": "Lifecycle"})},
		contactTagsTable:      {contactArchivedRow(contactTagsTable, 2, map[string]any{"tag_id": "tag-1", "tag_name": "Paid", "group_id": "group-1"})},
		contactBindingsTable:  {contactArchivedRow(contactBindingsTable, 3, map[string]any{"tag_id": "tag-1", "unionid": "union-a", "created_at": stamp})},
	}}
	store := newContactMemoryStore()
	store.customerTags[contactCustomerTagKey(19, 2)] = contactport.HistoricalCustomerTag{CustomerID: 19, TagID: 2, TaggedAt: stamp.Add(time.Hour), TaggedBy: "other"}
	journal := newContactMemoryJournal()
	importer, err := NewContactTagImporter(archive, contactMemoryUOW{}, store, journal, &contactDM01Verifier{targets: map[string]contactport.CustomerID{"union-a": 19}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "run"); !errors.Is(err, ErrConflict) || len(journal.terminal) != 0 {
		t.Fatalf("writer conflict was treated as duplicate: err=%v terminal=%+v", err, journal.terminal)
	}
}

type contactArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *contactArchive) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type contactMemoryUOW struct{}

func (contactMemoryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type contactDM01Verifier struct {
	targets                   map[string]contactport.CustomerID
	resolveCalls, verifyCalls int
}

func (verifier *contactDM01Verifier) ResolveVerifiedDM01Customer(_ context.Context, unionID string) (contactport.CustomerID, error) {
	verifier.resolveCalls++
	value, found := verifier.targets[unionID]
	if !found {
		return 0, errors.New("DM01 source key not found")
	}
	return value, nil
}

func (verifier *contactDM01Verifier) VerifyHistoricalTagCustomer(_ context.Context, unionID string, customerID contactport.CustomerID) error {
	verifier.verifyCalls++
	if verifier.targets[unionID] != customerID {
		return errors.New("DM01 source key mismatch")
	}
	return nil
}

type contactMemoryJournal struct {
	lineage  map[string]contactport.HistoricalTagLineage
	terminal []TerminalReceipt
}

func newContactMemoryJournal() *contactMemoryJournal {
	return &contactMemoryJournal{lineage: map[string]contactport.HistoricalTagLineage{}}
}

func (journal *contactMemoryJournal) FindHistoricalTagLineage(_ context.Context, source contactport.HistoricalTagSource, key [32]byte) (contactport.HistoricalTagLineage, bool, error) {
	value, found := journal.lineage[contactJournalKey(source, key)]
	return value, found, nil
}

func (journal *contactMemoryJournal) AppendHistoricalTagLineage(_ context.Context, source contactport.HistoricalTagSource, fact contactport.HistoricalTagFact, lineage contactport.HistoricalTagLineage) error {
	key := contactJournalKey(source, fact.SourceKeyDigest)
	if _, found := journal.lineage[key]; found {
		return contactport.ErrHistoricalTagConflict
	}
	journal.lineage[key] = lineage
	return nil
}

func (journal *contactMemoryJournal) RecordContactTagTerminal(_ context.Context, _ contactport.HistoricalTagSource, receipt TerminalReceipt) error {
	journal.terminal = append(journal.terminal, receipt)
	return nil
}

type contactMemoryStore struct {
	groups       map[int64]contactport.HistoricalTagGroup
	tags         map[int64]contactport.HistoricalTag
	customerTags map[string]contactport.HistoricalCustomerTag
	nextID       int64
}

func newContactMemoryStore() *contactMemoryStore {
	return &contactMemoryStore{groups: map[int64]contactport.HistoricalTagGroup{}, tags: map[int64]contactport.HistoricalTag{}, customerTags: map[string]contactport.HistoricalCustomerTag{}, nextID: 1}
}

func (store *contactMemoryStore) GetHistoricalTagGroup(_ context.Context, id int64) (contactport.HistoricalTagGroup, error) {
	value, found := store.groups[id]
	if !found {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagBlocked
	}
	return value, nil
}

func (store *contactMemoryStore) CreateHistoricalTagGroup(_ context.Context, value contactport.HistoricalTagGroup) (contactport.HistoricalTagGroup, error) {
	value.ID, store.nextID = store.nextID, store.nextID+1
	store.groups[value.ID] = value
	return value, nil
}

func (store *contactMemoryStore) GetHistoricalTag(_ context.Context, id int64) (contactport.HistoricalTag, error) {
	value, found := store.tags[id]
	if !found {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagBlocked
	}
	return value, nil
}

func (store *contactMemoryStore) FindHistoricalTagByProviderID(_ context.Context, providerTagID string) (contactport.HistoricalTag, bool, error) {
	for _, value := range store.tags {
		if value.ProviderTagID == providerTagID {
			return value, true, nil
		}
	}
	return contactport.HistoricalTag{}, false, nil
}

func (store *contactMemoryStore) CreateHistoricalTag(ctx context.Context, value contactport.HistoricalTag) (contactport.HistoricalTag, bool, error) {
	if prior, found, err := store.FindHistoricalTagByProviderID(ctx, value.ProviderTagID); err != nil || found {
		return prior, false, err
	}
	value.ID, store.nextID = store.nextID, store.nextID+1
	store.tags[value.ID] = value
	return value, true, nil
}

func (store *contactMemoryStore) GetHistoricalCustomerTag(_ context.Context, customerID contactport.CustomerID, tagID int64) (contactport.HistoricalCustomerTag, bool, error) {
	value, found := store.customerTags[contactCustomerTagKey(customerID, tagID)]
	return value, found, nil
}

func (store *contactMemoryStore) BindHistoricalCustomerTag(ctx context.Context, value contactport.HistoricalCustomerTag) (contactport.HistoricalCustomerTag, bool, error) {
	if prior, found, err := store.GetHistoricalCustomerTag(ctx, value.CustomerID, value.TagID); err != nil || found {
		return prior, false, err
	}
	store.customerTags[contactCustomerTagKey(value.CustomerID, value.TagID)] = value
	return value, true, nil
}

func contactArchivedRow(table string, index byte, payload map[string]any) v1archive.ArchivedRow {
	raw, _ := json.Marshal(payload)
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: int64(index), SourceKeyHMAC: contactDigest(index), PayloadHMAC: contactDigest(index + 40), FieldHMAC: contactDigest(index + 80), Payload: raw}
}

func contactDigest(index byte) [32]byte {
	return sha256.Sum256([]byte{index})
}

func contactJournalKey(source contactport.HistoricalTagSource, key [32]byte) string {
	return fmt.Sprintf("%s:%x", source, key)
}

func contactCustomerTagKey(customerID contactport.CustomerID, tagID int64) string {
	return fmt.Sprintf("%d:%d", customerID, tagID)
}
