package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestContactHistoryWriterCreatesAndReplaysBothHistoricalFacts(t *testing.T) {
	store := &contactHistoryStoreFake{}
	journal := &contactHistoryJournalFake{receipts: map[string]contactport.ContactHistoryReceipt{}}
	writer := NewContactHistoryWriter(store, journal)
	source, payload, sidebar, owner := contactHistoryValues()

	firstSidebar, err := writer.WriteSidebarProfile(context.Background(), source, payload, sidebar)
	if err != nil || firstSidebar.Replayed || firstSidebar.TargetID != 41 || store.sidebarCreates != 1 {
		t.Fatalf("sidebar first=%#v err=%v", firstSidebar, err)
	}
	replaySidebar, err := writer.WriteSidebarProfile(context.Background(), source, payload, sidebar)
	if err != nil || !replaySidebar.Replayed || replaySidebar.TargetDigest != firstSidebar.TargetDigest || store.sidebarCreates != 1 || store.sidebarGets != 1 {
		t.Fatalf("sidebar replay=%#v err=%v", replaySidebar, err)
	}

	sourceOwner := contactHistorySource("owner")
	owner.SourceKeyDigest = sourceOwner
	ownerPayload := sha256.Sum256([]byte("owner-payload"))
	owner.SourcePayloadDigest = ownerPayload
	ownerID := contactHistorySourceIdentifier(sourceOwner)
	firstOwner, err := writer.WriteOwnerMigrationResult(context.Background(), ownerID, ownerPayload, owner)
	if err != nil || firstOwner.Replayed || firstOwner.TargetID != 61 || store.ownerCreates != 1 {
		t.Fatalf("owner first=%#v err=%v", firstOwner, err)
	}
	replayOwner, err := writer.WriteOwnerMigrationResult(context.Background(), ownerID, ownerPayload, owner)
	if err != nil || !replayOwner.Replayed || replayOwner.TargetDigest != firstOwner.TargetDigest || store.ownerCreates != 1 || store.ownerGets != 1 {
		t.Fatalf("owner replay=%#v err=%v", replayOwner, err)
	}

	stored := store.sidebar[firstSidebar.TargetID]
	if stored.UpdatedAt.Location() != time.UTC || stored.UpdatedAt.Nanosecond()%1000 != 0 || stored.CustomerID == nil || *stored.CustomerID != 7 {
		t.Fatalf("sidebar normalization failed: %#v", stored)
	}
}

func TestContactHistoryWriterRejectsTargetAndPayloadDrift(t *testing.T) {
	store := &contactHistoryStoreFake{}
	journal := &contactHistoryJournalFake{receipts: map[string]contactport.ContactHistoryReceipt{}}
	writer := NewContactHistoryWriter(store, journal)
	source, payload, sidebar, owner := contactHistoryValues()
	if _, err := writer.WriteSidebarProfile(context.Background(), source, sha256.Sum256([]byte("other")), sidebar); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
		t.Fatalf("sidebar payload mismatch=%v", err)
	}
	receipt, err := writer.WriteSidebarProfile(context.Background(), source, payload, sidebar)
	if err != nil {
		t.Fatal(err)
	}
	changed := store.sidebar[receipt.TargetID]
	changed.Industry = "changed"
	store.sidebar[receipt.TargetID] = changed
	if _, err = writer.WriteSidebarProfile(context.Background(), source, payload, sidebar); !errors.Is(err, contactport.ErrContactHistoryConflict) {
		t.Fatalf("sidebar target drift=%v", err)
	}

	sourceOwner := contactHistorySource("owner")
	owner.SourceKeyDigest = sourceOwner
	ownerPayload := sha256.Sum256([]byte("owner-payload"))
	owner.SourcePayloadDigest = ownerPayload
	ownerID := contactHistorySourceIdentifier(sourceOwner)
	ownerReceipt, err := writer.WriteOwnerMigrationResult(context.Background(), ownerID, ownerPayload, owner)
	if err != nil {
		t.Fatal(err)
	}
	ownerChanged := store.ownerValues[ownerReceipt.TargetID]
	ownerChanged.WeComSuccess++
	store.ownerValues[ownerReceipt.TargetID] = ownerChanged
	if _, err = writer.WriteOwnerMigrationResult(context.Background(), ownerID, ownerPayload, owner); !errors.Is(err, contactport.ErrContactHistoryConflict) {
		t.Fatalf("owner target drift=%v", err)
	}
}

func TestContactHistoryWriterRejectsUnsafeInputAndCallerContext(t *testing.T) {
	source, payload, sidebar, owner := contactHistoryValues()
	for _, change := range []func(*contactport.HistoricalSidebarProfile){
		func(value *contactport.HistoricalSidebarProfile) { value.Source = "bad\x00" },
		func(value *contactport.HistoricalSidebarProfile) { value.CustomerID = ptr(int64(0)) },
		func(value *contactport.HistoricalSidebarProfile) { value.UpdatedAt = time.Time{} },
	} {
		value := sidebar
		change(&value)
		if _, err := NewContactHistoryWriter(&contactHistoryStoreFake{}, &contactHistoryJournalFake{}).WriteSidebarProfile(context.Background(), source, payload, value); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
			t.Fatalf("unsafe sidebar accepted: %v", err)
		}
	}
	for _, change := range []func(*contactport.HistoricalOwnerMigrationResult){
		func(value *contactport.HistoricalOwnerMigrationResult) { value.WeComFailed = -1 },
		func(value *contactport.HistoricalOwnerMigrationResult) { value.PreviewRelation = "active" },
		func(value *contactport.HistoricalOwnerMigrationResult) {
			value.TransferWelcomeMessage = string([]byte{0xff})
		},
	} {
		value := owner
		change(&value)
		if _, err := NewContactHistoryWriter(&contactHistoryStoreFake{}, &contactHistoryJournalFake{}).WriteOwnerMigrationResult(context.Background(), source, payload, value); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
			t.Fatalf("unsafe owner accepted: %v", err)
		}
	}

	store := &contactHistoryStoreFake{requireContext: true}
	if _, err := NewContactHistoryWriter(store, &contactHistoryJournalFake{}).WriteSidebarProfile(context.Background(), source, payload, sidebar); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
		t.Fatalf("missing caller context=%v", err)
	}
	ctx := context.WithValue(context.Background(), contactHistoryContextKey{}, "caller")
	if _, err := NewContactHistoryWriter(store, &contactHistoryJournalFake{receipts: map[string]contactport.ContactHistoryReceipt{}}).WriteSidebarProfile(ctx, source, payload, sidebar); err != nil {
		t.Fatalf("caller context not forwarded: %v", err)
	}
	var nilStore *contactHistoryStoreFake
	if _, err := NewContactHistoryWriter(nilStore, &contactHistoryJournalFake{}).WriteSidebarProfile(context.Background(), source, payload, sidebar); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
		t.Fatalf("typed nil store=%v", err)
	}
	var nilJournal *contactHistoryJournalFake
	if _, err := NewContactHistoryWriter(&contactHistoryStoreFake{}, nilJournal).WriteSidebarProfile(context.Background(), source, payload, sidebar); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
		t.Fatalf("typed nil journal=%v", err)
	}
}

type contactHistoryStoreFake struct {
	sidebar                                              map[int64]contactport.HistoricalSidebarProfile
	ownerValues                                          map[int64]contactport.HistoricalOwnerMigrationResult
	sidebarCreates, sidebarGets, ownerCreates, ownerGets int
	requireContext                                       bool
}

func (store *contactHistoryStoreFake) CreateHistoricalSidebarProfile(ctx context.Context, value contactport.HistoricalSidebarProfile) (contactport.HistoricalSidebarProfile, error) {
	if !store.validContext(ctx) {
		return contactport.HistoricalSidebarProfile{}, errors.New("caller transaction required")
	}
	if store.sidebar == nil {
		store.sidebar = map[int64]contactport.HistoricalSidebarProfile{}
	}
	store.sidebarCreates++
	value.ID = 41
	store.sidebar[value.ID] = value
	return value, nil
}

func (store *contactHistoryStoreFake) GetHistoricalSidebarProfile(ctx context.Context, id int64) (contactport.HistoricalSidebarProfile, error) {
	if !store.validContext(ctx) {
		return contactport.HistoricalSidebarProfile{}, errors.New("caller transaction required")
	}
	store.sidebarGets++
	value, found := store.sidebar[id]
	if !found {
		return contactport.HistoricalSidebarProfile{}, contactport.ErrContactHistoryConflict
	}
	return value, nil
}

func (store *contactHistoryStoreFake) CreateHistoricalOwnerMigrationResult(ctx context.Context, value contactport.HistoricalOwnerMigrationResult) (contactport.HistoricalOwnerMigrationResult, error) {
	if !store.validContext(ctx) {
		return contactport.HistoricalOwnerMigrationResult{}, errors.New("caller transaction required")
	}
	if store.ownerValues == nil {
		store.ownerValues = map[int64]contactport.HistoricalOwnerMigrationResult{}
	}
	store.ownerCreates++
	value.ID = 61
	store.ownerValues[value.ID] = value
	return value, nil
}

func (store *contactHistoryStoreFake) GetHistoricalOwnerMigrationResult(ctx context.Context, id int64) (contactport.HistoricalOwnerMigrationResult, error) {
	if !store.validContext(ctx) {
		return contactport.HistoricalOwnerMigrationResult{}, errors.New("caller transaction required")
	}
	store.ownerGets++
	value, found := store.ownerValues[id]
	if !found {
		return contactport.HistoricalOwnerMigrationResult{}, contactport.ErrContactHistoryConflict
	}
	return value, nil
}

func (store *contactHistoryStoreFake) validContext(ctx context.Context) bool {
	return !store.requireContext || ctx.Value(contactHistoryContextKey{}) == "caller"
}

type contactHistoryJournalFake struct {
	receipts map[string]contactport.ContactHistoryReceipt
}

func (journal *contactHistoryJournalFake) LoadContactHistory(_ context.Context, kind, source string) (contactport.ContactHistoryReceipt, bool, error) {
	if journal.receipts == nil {
		return contactport.ContactHistoryReceipt{}, false, nil
	}
	value, found := journal.receipts[kind+"/"+source]
	return value, found, nil
}

func (journal *contactHistoryJournalFake) RecordContactHistory(_ context.Context, receipt contactport.ContactHistoryReceipt) error {
	if journal.receipts == nil {
		journal.receipts = map[string]contactport.ContactHistoryReceipt{}
	}
	key := receipt.Kind + "/" + receipt.SourceIdentifier
	if _, found := journal.receipts[key]; found {
		return contactport.ErrContactHistoryConflict
	}
	journal.receipts[key] = receipt
	return nil
}

type contactHistoryContextKey struct{}

func contactHistoryValues() (string, [sha256.Size]byte, contactport.HistoricalSidebarProfile, contactport.HistoricalOwnerMigrationResult) {
	key := contactHistorySource("sidebar")
	payload := sha256.Sum256([]byte("sidebar-payload"))
	customer := int64(7)
	updated := time.Date(2026, 8, 28, 10, 11, 12, 123456789, time.FixedZone("+8", 8*60*60))
	sidebar := contactport.HistoricalSidebarProfile{SourceKeyDigest: key, CustomerID: &customer, Source: " sidebar ", Industry: "education", IndustryDescription: " description ", NeedsBlockersFollowup: " needs ", UpdatedAt: updated, SourcePayloadDigest: payload}
	created := time.Date(2026, 8, 27, 10, 11, 12, 123456789, time.FixedZone("+8", 8*60*60))
	owner := contactport.HistoricalOwnerMigrationResult{SourceKeyDigest: key, ScopeType: "all", FileHash: "file-hash", PreviewHash: "preview-hash", TotalRows: 4, EligibleCount: 3, WeComSuccess: 2, WeComFailed: 1, CRMUpdated: 2, IncludeWeComTransfer: true, TransferWelcomeMessage: " welcome ", SessionRelation: "unresolved", PreviewRelation: "resolved", CreatedAt: created, ExecutedAt: created.Add(time.Second), SourcePayloadDigest: payload}
	return contactHistorySourceIdentifier(key), payload, sidebar, owner
}

func contactHistorySource(seed string) [sha256.Size]byte { return sha256.Sum256([]byte(seed)) }
func contactHistorySourceIdentifier(value [sha256.Size]byte) string {
	return hex.EncodeToString(value[:])
}

func ptr[T any](value T) *T { return &value }
