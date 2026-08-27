package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var historicalChannelRelationsTestTime = time.Date(2026, 8, 28, 9, 10, 11, 123456789, time.FixedZone("CST", 8*60*60))

func TestHistoricalChannelRelationsWriterImportsHistoricalFacts(t *testing.T) {
	store, journal := newHistoricalChannelRelationsStore(), newHistoricalChannelRelationsJournal()
	writer, err := NewHistoricalChannelRelationsWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	contactReceipt, err := writer.ImportContact(context.Background(), historicalChannelContactDefinition())
	if err != nil || contactReceipt.Replayed || contactReceipt.TargetID != 1 || store.contactCreates != 1 || journal.records != 1 {
		t.Fatalf("contact receipt=%+v creates=%d records=%d err=%v", contactReceipt, store.contactCreates, journal.records, err)
	}
	contact := store.contacts[contactReceipt.TargetID]
	if contact.ID != 1 || contact.ChannelID != 19 || contact.SourceContactID != 91 || contact.CustomerID == nil || *contact.CustomerID != 7 || contact.OwnerReference != "legacy-owner" || !contact.FirstEnteredAt.Equal(historicalChannelRelationsTestTime.UTC().Truncate(time.Microsecond)) || !contact.UpdatedAt.Equal(historicalChannelRelationsTestTime.Add(time.Hour).UTC().Truncate(time.Microsecond)) {
		t.Fatalf("contact=%+v", contact)
	}
	assigneeReceipt, err := writer.ImportAssignee(context.Background(), historicalChannelAssigneeDefinition())
	if err != nil || assigneeReceipt.Replayed || assigneeReceipt.TargetID != 1 || store.assigneeCreates != 1 || journal.records != 2 {
		t.Fatalf("assignee receipt=%+v creates=%d records=%d err=%v", assigneeReceipt, store.assigneeCreates, journal.records, err)
	}
	assignee := store.assignees[assigneeReceipt.TargetID]
	if assignee.ID != 1 || assignee.ChannelID != 19 || assignee.SourceAssigneeID != 31 || assignee.StaffReference != " staff-legacy " || assignee.DisplayNameSnapshot != " 显示名 " || assignee.SourceCreatedAt != "2026-08-28T09:10:11.123456" || assignee.SourceUpdatedAt != "2026-08-28T10:10:11.123456" {
		t.Fatalf("assignee=%+v", assignee)
	}
}

func TestHistoricalChannelRelationsWriterReplaysAndRejectsTargetDrift(t *testing.T) {
	store, journal := newHistoricalChannelRelationsStore(), newHistoricalChannelRelationsJournal()
	writer, _ := NewHistoricalChannelRelationsWriter(store, journal)
	contactDefinition := historicalChannelContactDefinition()
	firstContact, err := writer.ImportContact(context.Background(), contactDefinition)
	if err != nil {
		t.Fatal(err)
	}
	replayedContact, err := writer.ImportContact(context.Background(), contactDefinition)
	if err != nil || !replayedContact.Replayed || replayedContact.TargetID != firstContact.TargetID || store.contactCreates != 1 || store.contactGets != 1 {
		t.Fatalf("first=%+v replay=%+v creates=%d gets=%d err=%v", firstContact, replayedContact, store.contactCreates, store.contactGets, err)
	}
	changedContact := store.contacts[firstContact.TargetID]
	changedContact.OwnerReference = "drift"
	store.contacts[firstContact.TargetID] = changedContact
	if _, err = writer.ImportContact(context.Background(), contactDefinition); !errors.Is(err, contactport.ErrHistoricalChannelConflict) {
		t.Fatalf("contact drift error=%v", err)
	}

	assigneeDefinition := historicalChannelAssigneeDefinition()
	firstAssignee, err := writer.ImportAssignee(context.Background(), assigneeDefinition)
	if err != nil {
		t.Fatal(err)
	}
	replayedAssignee, err := writer.ImportAssignee(context.Background(), assigneeDefinition)
	if err != nil || !replayedAssignee.Replayed || replayedAssignee.TargetID != firstAssignee.TargetID || store.assigneeCreates != 1 || store.assigneeGets != 1 {
		t.Fatalf("first=%+v replay=%+v creates=%d gets=%d err=%v", firstAssignee, replayedAssignee, store.assigneeCreates, store.assigneeGets, err)
	}
	changedAssignee := store.assignees[firstAssignee.TargetID]
	changedAssignee.Status = "drift"
	store.assignees[firstAssignee.TargetID] = changedAssignee
	if _, err = writer.ImportAssignee(context.Background(), assigneeDefinition); !errors.Is(err, contactport.ErrHistoricalChannelConflict) {
		t.Fatalf("assignee drift error=%v", err)
	}
}

func TestHistoricalChannelRelationsWriterRejectsInvalidDefinitionsAndReceiptDrift(t *testing.T) {
	store, journal := newHistoricalChannelRelationsStore(), newHistoricalChannelRelationsJournal()
	writer, _ := NewHistoricalChannelRelationsWriter(store, journal)
	contactDefinition := historicalChannelContactDefinition()
	if _, err := writer.ImportContact(context.Background(), contactDefinition); err != nil {
		t.Fatal(err)
	}
	changed := contactDefinition
	changed.PayloadDigest[0]++
	if _, err := writer.ImportContact(context.Background(), changed); !errors.Is(err, contactport.ErrHistoricalChannelConflict) {
		t.Fatalf("receipt drift error=%v", err)
	}
	zero := int64(0)
	for _, definition := range []contactport.HistoricalChannelContactDefinition{
		func() contactport.HistoricalChannelContactDefinition {
			value := contactDefinition
			value.Contact.ID = 1
			return value
		}(),
		func() contactport.HistoricalChannelContactDefinition {
			value := contactDefinition
			value.Contact.CustomerID = &zero
			return value
		}(),
		func() contactport.HistoricalChannelContactDefinition {
			value := contactDefinition
			value.Contact.LastEnteredAt = value.Contact.FirstEnteredAt.Add(-time.Nanosecond)
			return value
		}(),
	} {
		if _, err := writer.ImportContact(context.Background(), definition); !errors.Is(err, contactport.ErrHistoricalChannelInvalid) {
			t.Fatalf("invalid contact=%+v error=%v", definition, err)
		}
	}
	for _, definition := range []contactport.HistoricalChannelAssigneeDefinition{
		func() contactport.HistoricalChannelAssigneeDefinition {
			value := historicalChannelAssigneeDefinition()
			value.Assignee.ID = 1
			return value
		}(),
		func() contactport.HistoricalChannelAssigneeDefinition {
			value := historicalChannelAssigneeDefinition()
			value.Assignee.SourceCreatedAt += "Z"
			return value
		}(),
		func() contactport.HistoricalChannelAssigneeDefinition {
			value := historicalChannelAssigneeDefinition()
			value.Assignee.SourceUpdatedAt = "2026-08-28T09:10:11.123455"
			return value
		}(),
		func() contactport.HistoricalChannelAssigneeDefinition {
			value := historicalChannelAssigneeDefinition()
			ratio := int32(101)
			value.Assignee.RatioPercent = &ratio
			return value
		}(),
	} {
		if _, err := writer.ImportAssignee(context.Background(), definition); !errors.Is(err, contactport.ErrHistoricalChannelInvalid) {
			t.Fatalf("invalid assignee=%+v error=%v", definition, err)
		}
	}
}

func TestHistoricalChannelRelationsWriterRequiresCallerBoundDependenciesAndPropagatesErrors(t *testing.T) {
	if writer, err := NewHistoricalChannelRelationsWriter(nil, newHistoricalChannelRelationsJournal()); writer != nil || !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("missing store writer=%v err=%v", writer, err)
	}
	if writer, err := NewHistoricalChannelRelationsWriter(newHistoricalChannelRelationsStore(), nil); writer != nil || !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("missing journal writer=%v err=%v", writer, err)
	}
	store, journal := newHistoricalChannelRelationsStore(), newHistoricalChannelRelationsJournal()
	writer, _ := NewHistoricalChannelRelationsWriter(store, journal)
	if _, err := writer.ImportContact(nil, historicalChannelContactDefinition()); !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) {
		t.Fatalf("nil context error=%v", err)
	}
	loadErr := errors.New("load failed")
	journal.loadErr = loadErr
	if _, err := writer.ImportContact(context.Background(), historicalChannelContactDefinition()); !errors.Is(err, loadErr) {
		t.Fatalf("load error=%v", err)
	}
	journal.loadErr = nil
	createErr := errors.New("create failed")
	store.contactCreateErr = createErr
	if _, err := writer.ImportContact(context.Background(), historicalChannelContactDefinition()); !errors.Is(err, createErr) {
		t.Fatalf("create error=%v", err)
	}
	store.contactCreateErr = nil
	recordErr := errors.New("record failed")
	journal.recordErr = recordErr
	if _, err := writer.ImportAssignee(context.Background(), historicalChannelAssigneeDefinition()); !errors.Is(err, recordErr) {
		t.Fatalf("record error=%v", err)
	}
	journal.recordErr = nil
	if _, err := writer.ImportAssignee(context.Background(), historicalChannelAssigneeDefinition()); err != nil {
		t.Fatal(err)
	}
	getErr := errors.New("get failed")
	store.assigneeGetErr = getErr
	if _, err := writer.ImportAssignee(context.Background(), historicalChannelAssigneeDefinition()); !errors.Is(err, getErr) {
		t.Fatalf("get error=%v", err)
	}
}

func TestHistoricalChannelRelationsTargetDigestsIncludeEveryStaticField(t *testing.T) {
	contact, err := historicalChannelContactRecord(historicalChannelContactDefinition().Contact)
	if err != nil {
		t.Fatal(err)
	}
	contact.ID = 1
	firstContact, err := HistoricalChannelContactTargetDigest(contact)
	if err != nil {
		t.Fatal(err)
	}
	contact.OwnerReference = "other-owner"
	secondContact, err := HistoricalChannelContactTargetDigest(contact)
	if err != nil || firstContact == secondContact {
		t.Fatalf("contact first=%x second=%x err=%v", firstContact, secondContact, err)
	}

	assignee, err := historicalChannelAssigneeRecord(historicalChannelAssigneeDefinition().Assignee)
	if err != nil {
		t.Fatal(err)
	}
	assignee.ID = 1
	firstAssignee, err := HistoricalChannelAssigneeTargetDigest(assignee)
	if err != nil {
		t.Fatal(err)
	}
	assignee.DisplayNameSnapshot = "other-name"
	secondAssignee, err := HistoricalChannelAssigneeTargetDigest(assignee)
	if err != nil || firstAssignee == secondAssignee {
		t.Fatalf("assignee first=%x second=%x err=%v", firstAssignee, secondAssignee, err)
	}
}

func historicalChannelContactDefinition() contactport.HistoricalChannelContactDefinition {
	customerID := int64(7)
	return contactport.HistoricalChannelContactDefinition{
		SourceIdentifier: "automation_channel_contact:91", PayloadDigest: sha256.Sum256([]byte("channel-contact-91")),
		Contact: contactport.HistoricalChannelContact{ChannelID: 19, SourceContactID: 91, CustomerID: &customerID, OwnerReference: "legacy-owner", FirstEnteredAt: historicalChannelRelationsTestTime, LastEnteredAt: historicalChannelRelationsTestTime.Add(time.Minute), EnterCount: 2, CreatedAt: historicalChannelRelationsTestTime, UpdatedAt: historicalChannelRelationsTestTime.Add(time.Hour)},
	}
}

func historicalChannelAssigneeDefinition() contactport.HistoricalChannelAssigneeDefinition {
	ratio, maximum := int32(50), int32(100)
	return contactport.HistoricalChannelAssigneeDefinition{
		SourceIdentifier: "automation_channel_assignee:31", PayloadDigest: sha256.Sum256([]byte("channel-assignee-31")),
		Assignee: contactport.HistoricalChannelAssignee{ChannelID: 19, SourceAssigneeID: 31, StaffReference: " staff-legacy ", DisplayNameSnapshot: " 显示名 ", Priority: 0, RatioPercent: &ratio, MaxScans24h: &maximum, Status: "disabled", SourceCreatedAt: "2026-08-28T09:10:11.123456", SourceUpdatedAt: "2026-08-28T10:10:11.123456"},
	}
}

type historicalChannelRelationsStore struct {
	contacts                                               map[int64]contactport.HistoricalChannelContact
	assignees                                              map[int64]contactport.HistoricalChannelAssignee
	nextContact, nextAssignee, contactCreates, contactGets int64
	assigneeCreates, assigneeGets                          int64
	contactCreateErr, contactGetErr                        error
	assigneeCreateErr, assigneeGetErr                      error
}

func newHistoricalChannelRelationsStore() *historicalChannelRelationsStore {
	return &historicalChannelRelationsStore{contacts: map[int64]contactport.HistoricalChannelContact{}, assignees: map[int64]contactport.HistoricalChannelAssignee{}, nextContact: 1, nextAssignee: 1}
}

func (store *historicalChannelRelationsStore) CreateHistoricalChannelContact(_ context.Context, record contactport.HistoricalChannelContact) (contactport.HistoricalChannelContact, error) {
	if store.contactCreateErr != nil {
		return contactport.HistoricalChannelContact{}, store.contactCreateErr
	}
	record.ID, store.nextContact, store.contactCreates = store.nextContact, store.nextContact+1, store.contactCreates+1
	store.contacts[record.ID] = record
	return record, nil
}

func (store *historicalChannelRelationsStore) GetHistoricalChannelContact(_ context.Context, id int64) (contactport.HistoricalChannelContact, error) {
	store.contactGets++
	if store.contactGetErr != nil {
		return contactport.HistoricalChannelContact{}, store.contactGetErr
	}
	record, found := store.contacts[id]
	if !found {
		return contactport.HistoricalChannelContact{}, errors.New("contact not found")
	}
	return record, nil
}

func (store *historicalChannelRelationsStore) CreateHistoricalChannelAssignee(_ context.Context, record contactport.HistoricalChannelAssignee) (contactport.HistoricalChannelAssignee, error) {
	if store.assigneeCreateErr != nil {
		return contactport.HistoricalChannelAssignee{}, store.assigneeCreateErr
	}
	record.ID, store.nextAssignee, store.assigneeCreates = store.nextAssignee, store.nextAssignee+1, store.assigneeCreates+1
	store.assignees[record.ID] = record
	return record, nil
}

func (store *historicalChannelRelationsStore) GetHistoricalChannelAssignee(_ context.Context, id int64) (contactport.HistoricalChannelAssignee, error) {
	store.assigneeGets++
	if store.assigneeGetErr != nil {
		return contactport.HistoricalChannelAssignee{}, store.assigneeGetErr
	}
	record, found := store.assignees[id]
	if !found {
		return contactport.HistoricalChannelAssignee{}, errors.New("assignee not found")
	}
	return record, nil
}

type historicalChannelRelationsJournal struct {
	receipts           map[string]contactport.HistoricalChannelReceipt
	loadErr, recordErr error
	records            int64
}

func newHistoricalChannelRelationsJournal() *historicalChannelRelationsJournal {
	return &historicalChannelRelationsJournal{receipts: map[string]contactport.HistoricalChannelReceipt{}}
}

func (journal *historicalChannelRelationsJournal) LoadHistoricalChannelRelation(_ context.Context, kind, source string) (contactport.HistoricalChannelReceipt, bool, error) {
	if journal.loadErr != nil {
		return contactport.HistoricalChannelReceipt{}, false, journal.loadErr
	}
	receipt, found := journal.receipts[kind+":"+source]
	return receipt, found, nil
}

func (journal *historicalChannelRelationsJournal) RecordHistoricalChannelRelation(_ context.Context, kind string, receipt contactport.HistoricalChannelReceipt) error {
	if journal.recordErr != nil {
		return journal.recordErr
	}
	key := kind + ":" + receipt.SourceIdentifier
	if _, found := journal.receipts[key]; found {
		return contactport.ErrHistoricalChannelConflict
	}
	journal.receipts[key], journal.records = receipt, journal.records+1
	return nil
}
