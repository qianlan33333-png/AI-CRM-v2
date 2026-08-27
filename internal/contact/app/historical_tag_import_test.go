package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestHistoricalTagImportIsLocalReplaySafeAndPreservesMappings(t *testing.T) {
	store := newHistoricalTagMemoryStore()
	journal := newHistoricalTagMemoryJournal()
	verifier := &historicalTagCustomerVerifier{targets: map[string]contactport.CustomerID{"union-1": 51}}
	service := NewHistoricalTagImportService(historicalTagMemoryUOW{}, store, journal, verifier)
	group := historicalTagGroupRecord(1, "Lifecycle", 2)
	firstGroup, err := service.ImportGroup(context.Background(), group)
	if err != nil || firstGroup.Replayed || firstGroup.TargetID < 1 || !firstGroup.LocalProjection || firstGroup.ProviderExecutionEligible || firstGroup.RealExternalCallExecuted {
		t.Fatalf("group=%+v err=%v", firstGroup, err)
	}
	tag := historicalTagRecord(2, group.Fact.SourceKeyDigest, "corp-tag-1", "Paid", 3)
	firstTag, err := service.ImportTag(context.Background(), tag)
	if err != nil || firstTag.Replayed || firstTag.TargetID < 1 || store.tags[firstTag.TargetID].GroupID != firstGroup.TargetID || store.tags[firstTag.TargetID].ProviderTagID != "corp-tag-1" {
		t.Fatalf("tag=%+v err=%v tags=%+v", firstTag, err, store.tags)
	}
	stamp := time.Date(2025, 2, 3, 4, 5, 6, 0, time.FixedZone("legacy", 8*60*60))
	customer := historicalCustomerTagRecord(3, "union-1", 51, "corp-tag-1", stamp)
	firstCustomer, err := service.ImportCustomerTag(context.Background(), customer)
	if err != nil || firstCustomer.Replayed || firstCustomer.TargetID != firstTag.TargetID || verifier.calls != 1 {
		t.Fatalf("customer=%+v err=%v calls=%d", firstCustomer, err, verifier.calls)
	}
	if journal.values[historicalTagJournalKey(contactport.HistoricalTagGroupSource, group.Fact.SourceKeyDigest)].CustomerID != 0 ||
		journal.values[historicalTagJournalKey(contactport.HistoricalTagCatalogTagSource, tag.Fact.SourceKeyDigest)].CustomerID != 0 ||
		journal.values[historicalTagJournalKey(contactport.HistoricalCustomerTagSource, customer.Fact.SourceKeyDigest)].CustomerID != 51 {
		t.Fatalf("unexpected lineage addresses: %+v", journal.values)
	}
	binding, found := store.customerTags[historicalCustomerTagKey(51, firstTag.TargetID)]
	if !found || !binding.TaggedAt.Equal(stamp.UTC()) || binding.TaggedBy != historicalContactTagTaggedBy {
		t.Fatalf("binding=%+v found=%t", binding, found)
	}
	writes := store.writes
	for _, replay := range []struct {
		name string
		run  func() (HistoricalTagImportResult, error)
	}{
		{"group", func() (HistoricalTagImportResult, error) { return service.ImportGroup(context.Background(), group) }},
		{"tag", func() (HistoricalTagImportResult, error) { return service.ImportTag(context.Background(), tag) }},
		{"customer", func() (HistoricalTagImportResult, error) {
			return service.ImportCustomerTag(context.Background(), customer)
		}},
	} {
		result, replayErr := replay.run()
		if replayErr != nil || !result.Replayed || store.writes != writes {
			t.Fatalf("%s replay result=%+v err=%v writes=%d", replay.name, result, replayErr, store.writes)
		}
	}
	if verifier.calls != 2 { // exact customer replay still re-verifies the main-provided target.
		t.Fatalf("customer verifier calls=%d", verifier.calls)
	}
}

func TestHistoricalTagImportKeepsTransientCustomerVerifierFailureOutOfQuarantine(t *testing.T) {
	store := newHistoricalTagMemoryStore()
	journal := newHistoricalTagMemoryJournal()
	verifier := &historicalTagCustomerVerifier{targets: map[string]contactport.CustomerID{"union-1": 51}, err: errors.New("database unavailable")}
	service := NewHistoricalTagImportService(historicalTagMemoryUOW{}, store, journal, verifier)
	group := historicalTagGroupRecord(31, "Lifecycle", 0)
	if _, err := service.ImportGroup(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	tag := historicalTagRecord(32, group.Fact.SourceKeyDigest, "corp-tag-1", "Paid", 0)
	if _, err := service.ImportTag(context.Background(), tag); err != nil {
		t.Fatal(err)
	}
	customer := historicalCustomerTagRecord(33, "union-1", 51, "corp-tag-1", time.Now().UTC())
	if _, err := service.ImportCustomerTag(context.Background(), customer); !errors.Is(err, contactport.ErrHistoricalTagUnavailable) || errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		t.Fatalf("transient verifier error=%v", err)
	}
	if len(store.customerTags) != 0 {
		t.Fatalf("transient verifier created binding: %+v", store.customerTags)
	}
}

func TestHistoricalTagImportFailsClosedForLineageAndCustomerCrosswalk(t *testing.T) {
	store := newHistoricalTagMemoryStore()
	journal := newHistoricalTagMemoryJournal()
	verifier := &historicalTagCustomerVerifier{targets: map[string]contactport.CustomerID{"union-ok": 61}}
	service := NewHistoricalTagImportService(historicalTagMemoryUOW{}, store, journal, verifier)
	group := historicalTagGroupRecord(10, "Original", 0)
	if _, err := service.ImportGroup(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	changed := group
	changed.Name = "Changed"
	changed.Fact.PayloadDigest = historicalTagDigest(42)
	if _, err := service.ImportGroup(context.Background(), changed); !errors.Is(err, contactport.ErrHistoricalTagConflict) || store.writes != 1 {
		t.Fatalf("changed group err=%v writes=%d", err, store.writes)
	}
	missingParent := historicalTagRecord(11, historicalTagDigest(99), "corp-tag-missing", "Missing", 0)
	if _, err := service.ImportTag(context.Background(), missingParent); !errors.Is(err, contactport.ErrHistoricalTagBlocked) || store.writes != 1 {
		t.Fatalf("missing parent err=%v writes=%d", err, store.writes)
	}
	tag := historicalTagRecord(12, group.Fact.SourceKeyDigest, "corp-tag-2", "Tag", 0)
	if _, err := service.ImportTag(context.Background(), tag); err != nil {
		t.Fatal(err)
	}
	badCustomer := historicalCustomerTagRecord(13, "union-ok", 62, "corp-tag-2", time.Now().UTC())
	if _, err := service.ImportCustomerTag(context.Background(), badCustomer); !errors.Is(err, contactport.ErrHistoricalTagBlocked) || len(store.customerTags) != 0 {
		t.Fatalf("unverified customer err=%v bindings=%+v", err, store.customerTags)
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls=%d", verifier.calls)
	}
	withoutJournal := NewHistoricalTagImportService(historicalTagMemoryUOW{}, store, nil, verifier)
	if _, err := withoutJournal.ImportGroup(context.Background(), historicalTagGroupRecord(14, "No journal", 1)); !errors.Is(err, contactport.ErrHistoricalTagUnavailable) || store.writes != 2 {
		t.Fatalf("journal err=%v writes=%d", err, store.writes)
	}
}

func TestHistoricalTagImportReportsFinalAttemptAfterTransactionRetry(t *testing.T) {
	store := newHistoricalTagMemoryStore()
	journal := newHistoricalTagMemoryJournal()
	group := historicalTagGroupRecord(21, "Retry", 0)
	initial := NewHistoricalTagImportService(historicalTagMemoryUOW{}, store, journal, nil)
	prior, err := initial.ImportGroup(context.Background(), group)
	if err != nil || prior.Replayed {
		t.Fatalf("initial=%+v err=%v", prior, err)
	}

	retry := historicalTagRetryUOW{afterFirst: func() {
		delete(journal.values, historicalTagJournalKey(contactport.HistoricalTagGroupSource, group.Fact.SourceKeyDigest))
		delete(store.groups, prior.TargetID)
	}}
	service := NewHistoricalTagImportService(retry, store, journal, nil)
	result, err := service.ImportGroup(context.Background(), group)
	if err != nil || result.Replayed || result.TargetID == prior.TargetID || !result.LocalProjection {
		t.Fatalf("result=%+v prior=%+v err=%v", result, prior, err)
	}
}

type historicalTagMemoryUOW struct{}

func (historicalTagMemoryUOW) Within(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}

// historicalTagRetryUOW models a retryable transaction whose first callback
// view is rolled back before the second callback observes the durable state.
type historicalTagRetryUOW struct{ afterFirst func() }

func (uow historicalTagRetryUOW) Within(ctx context.Context, run func(context.Context) error) error {
	if err := run(ctx); err != nil {
		return err
	}
	uow.afterFirst()
	return run(ctx)
}

type historicalTagMemoryJournal struct {
	values map[string]contactport.HistoricalTagLineage
}

func newHistoricalTagMemoryJournal() *historicalTagMemoryJournal {
	return &historicalTagMemoryJournal{values: map[string]contactport.HistoricalTagLineage{}}
}
func (journal *historicalTagMemoryJournal) FindHistoricalTagLineage(_ context.Context, source contactport.HistoricalTagSource, key [32]byte) (contactport.HistoricalTagLineage, bool, error) {
	value, found := journal.values[historicalTagJournalKey(source, key)]
	return value, found, nil
}
func (journal *historicalTagMemoryJournal) AppendHistoricalTagLineage(_ context.Context, source contactport.HistoricalTagSource, fact contactport.HistoricalTagFact, value contactport.HistoricalTagLineage) error {
	key := historicalTagJournalKey(source, fact.SourceKeyDigest)
	if _, exists := journal.values[key]; exists {
		return contactport.ErrHistoricalTagConflict
	}
	journal.values[key] = value
	return nil
}

type historicalTagCustomerVerifier struct {
	targets map[string]contactport.CustomerID
	calls   int
	err     error
}

func (verifier *historicalTagCustomerVerifier) VerifyHistoricalTagCustomer(_ context.Context, unionID string, target contactport.CustomerID) error {
	verifier.calls++
	if verifier.err != nil {
		return verifier.err
	}
	if verifier.targets[unionID] != target {
		return contactport.ErrHistoricalTagBlocked
	}
	return nil
}

type historicalTagMemoryStore struct {
	groups       map[int64]contactport.HistoricalTagGroup
	tags         map[int64]contactport.HistoricalTag
	customerTags map[string]contactport.HistoricalCustomerTag
	nextID       int64
	writes       int
}

func newHistoricalTagMemoryStore() *historicalTagMemoryStore {
	return &historicalTagMemoryStore{groups: map[int64]contactport.HistoricalTagGroup{}, tags: map[int64]contactport.HistoricalTag{}, customerTags: map[string]contactport.HistoricalCustomerTag{}, nextID: 1}
}
func (store *historicalTagMemoryStore) GetHistoricalTagGroup(_ context.Context, id int64) (contactport.HistoricalTagGroup, error) {
	value, found := store.groups[id]
	if !found {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagBlocked
	}
	return value, nil
}
func (store *historicalTagMemoryStore) CreateHistoricalTagGroup(_ context.Context, value contactport.HistoricalTagGroup) (contactport.HistoricalTagGroup, error) {
	value.ID, store.nextID = store.nextID, store.nextID+1
	store.groups[value.ID], store.writes = value, store.writes+1
	return value, nil
}
func (store *historicalTagMemoryStore) GetHistoricalTag(_ context.Context, id int64) (contactport.HistoricalTag, error) {
	value, found := store.tags[id]
	if !found {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagBlocked
	}
	return value, nil
}
func (store *historicalTagMemoryStore) FindHistoricalTagByProviderID(_ context.Context, providerTagID string) (contactport.HistoricalTag, bool, error) {
	for _, value := range store.tags {
		if value.ProviderTagID == providerTagID {
			return value, true, nil
		}
	}
	return contactport.HistoricalTag{}, false, nil
}
func (store *historicalTagMemoryStore) CreateHistoricalTag(ctx context.Context, value contactport.HistoricalTag) (contactport.HistoricalTag, bool, error) {
	if prior, found, err := store.FindHistoricalTagByProviderID(ctx, value.ProviderTagID); err != nil || found {
		return prior, false, err
	}
	value.ID, store.nextID = store.nextID, store.nextID+1
	store.tags[value.ID], store.writes = value, store.writes+1
	return value, true, nil
}
func (store *historicalTagMemoryStore) GetHistoricalCustomerTag(_ context.Context, customerID contactport.CustomerID, tagID int64) (contactport.HistoricalCustomerTag, bool, error) {
	value, found := store.customerTags[historicalCustomerTagKey(customerID, tagID)]
	return value, found, nil
}
func (store *historicalTagMemoryStore) BindHistoricalCustomerTag(ctx context.Context, value contactport.HistoricalCustomerTag) (contactport.HistoricalCustomerTag, bool, error) {
	if prior, found, err := store.GetHistoricalCustomerTag(ctx, value.CustomerID, value.TagID); err != nil || found {
		return prior, false, err
	}
	store.customerTags[historicalCustomerTagKey(value.CustomerID, value.TagID)], store.writes = value, store.writes+1
	return value, true, nil
}

func historicalTagGroupRecord(index byte, name string, sort int32) contactport.HistoricalTagGroupRecord {
	return contactport.HistoricalTagGroupRecord{Fact: historicalTagFact(index), Name: name, SortOrder: sort}
}
func historicalTagRecord(index byte, groupKey [32]byte, providerID, name string, sort int32) contactport.HistoricalTagRecord {
	return contactport.HistoricalTagRecord{Fact: historicalTagFact(index), GroupSourceKeyDigest: groupKey, ProviderTagID: providerID, Name: name, SortOrder: sort}
}
func historicalCustomerTagRecord(index byte, unionID string, customerID contactport.CustomerID, providerID string, taggedAt time.Time) contactport.HistoricalCustomerTagRecord {
	return contactport.HistoricalCustomerTagRecord{Fact: historicalTagFact(index), UnionID: unionID, VerifiedCustomerID: customerID, ProviderTagID: providerID, TaggedAt: taggedAt}
}
func historicalTagFact(index byte) contactport.HistoricalTagFact {
	return contactport.HistoricalTagFact{SourceKeyDigest: historicalTagDigest(index), PayloadDigest: historicalTagDigest(index + 30), FieldDigest: historicalTagDigest(index + 60)}
}
func historicalTagDigest(index byte) [32]byte {
	var value [32]byte
	value[0] = index
	return value
}
func historicalTagJournalKey(source contactport.HistoricalTagSource, digest [32]byte) string {
	return fmt.Sprintf("%s:%x", source, digest)
}
func historicalCustomerTagKey(customerID contactport.CustomerID, tagID int64) string {
	return fmt.Sprintf("%d:%d", customerID, tagID)
}
