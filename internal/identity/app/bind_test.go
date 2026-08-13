package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type bindTestStore struct {
	receipt       BindReceipt
	record        BindRecord
	err           error
	reserveCalls  int
	bindCalls     int
	rebindCalls   int
	auditCalls    int
	reviewCalls   int
	completeCalls int
	identity      NormalizedIdentity
	customerID    int64
	completed     identityport.BindResult
	audit         AutoMergeAudit
	reviewIDs     []int64
	candidates    []contactport.CustomerID
	fingerprint   []byte
}

func (store *bindTestStore) ReserveBindReceipt(_ context.Context, _, _ []byte) (BindReceipt, error) {
	store.reserveCalls++
	return store.receipt, store.err
}

func (store *bindTestStore) BindNormalized(_ context.Context, identity NormalizedIdentity, customerID int64) (BindRecord, error) {
	store.bindCalls++
	store.identity, store.customerID = identity, customerID
	return store.record, store.err
}

func (store *bindTestStore) RebindIdentitiesForCustomerMerge(_ context.Context, _, _ contactport.CustomerID) error {
	store.rebindCalls++
	return store.err
}

func (store *bindTestStore) InsertAutoCustomerMergeAudit(_ context.Context, audit AutoMergeAudit) (int64, error) {
	store.auditCalls++
	store.audit = audit
	if store.err != nil {
		return 0, store.err
	}
	return 17, nil
}

func (store *bindTestStore) InsertVerifiedPhoneMergeReview(_ context.Context, identityID int64, candidates []contactport.CustomerID, fingerprint []byte) (int64, error) {
	store.reviewCalls++
	store.reviewIDs = append(store.reviewIDs, identityID)
	store.candidates = append([]contactport.CustomerID(nil), candidates...)
	store.fingerprint = append([]byte(nil), fingerprint...)
	if store.err != nil {
		return 0, store.err
	}
	return 23, nil
}

func (store *bindTestStore) CompleteBindReceipt(_ context.Context, _ BindReceipt, result identityport.BindResult) error {
	store.completeCalls++
	store.completed = result
	return store.err
}

type bindTestEvents struct{ events []eventport.Event }

func (events *bindTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.events = append(events.events, event)
	return 1, nil
}

type bindTestContacts struct {
	merges []contactport.MergeCustomersCommand
	err    error
}

func (contacts *bindTestContacts) CreateForIdentity(context.Context, contactport.CreateForIdentityCommand) (contactport.CustomerID, error) {
	return 0, contacts.err
}

func (contacts *bindTestContacts) MergeCustomers(_ context.Context, command contactport.MergeCustomersCommand) error {
	contacts.merges = append(contacts.merges, command)
	return contacts.err
}

func (contacts *bindTestContacts) AppendExternalEvent(context.Context, contactport.ExternalEventCommand) (contactport.EventID, error) {
	return 0, contacts.err
}

func TestBindServiceBindsFloatingIdentityAndCompletesReceipt(t *testing.T) {
	uow, store, events := &resolveTestUoW{}, &bindTestStore{receipt: BindReceipt{ID: 9}, record: BindRecord{Status: identityport.BindBound, IdentityID: 7}}, &bindTestEvents{}
	service := NewBindService(uow, store, events, []byte("12345678901234567890123456789012"))
	command := validBindCommandForTest()

	result, err := service.Bind(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	want := identityport.BindResult{Status: identityport.BindBound, CustomerID: command.CustomerID}
	if result != want || store.completed != want || uow.calls != 1 || store.reserveCalls != 1 || store.bindCalls != 1 || store.completeCalls != 1 {
		t.Fatalf("result=%+v completed=%+v calls=%d/%d/%d/%d", result, store.completed, uow.calls, store.reserveCalls, store.bindCalls, store.completeCalls)
	}
	wantNormalized := NormalizedIdentity{Kind: identityport.KindPhone, Scope: "phone:e164", NormalizedValue: "+8613800138000", NormalizerVersion: NormalizerVersion}
	if store.customerID != int64(command.CustomerID) || !reflect.DeepEqual(store.identity, wantNormalized) {
		t.Fatalf("store input customer=%d identity=%+v", store.customerID, store.identity)
	}
	if len(events.events) != 1 || events.events[0].Type != "identity.bound" || events.events[0].CustomerID != eventport.CustomerID(command.CustomerID) || events.events[0].IdempotencyKey != "identity.bound:7" {
		t.Fatalf("events=%+v", events.events)
	}
	if string(events.events[0].Payload) == "" || string(events.events[0].Payload) == command.Ref.Value || !json.Valid(events.events[0].Payload) {
		t.Fatalf("bound payload=%s", events.events[0].Payload)
	}
}

func TestBindServiceReplaysCompletedReceiptAndRejectsPayloadReuse(t *testing.T) {
	command := validBindCommandForTest()
	key := []byte("12345678901234567890123456789012")
	service := NewBindService(&resolveTestUoW{}, &bindTestStore{}, &bindTestEvents{}, key)
	normalized, err := Normalize(command.Ref)
	if err != nil {
		t.Fatal(err)
	}
	_, payloadHMAC, err := service.receiptDigests(command, normalized)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("same canonical payload returns original fact", func(t *testing.T) {
		store := &bindTestStore{receipt: BindReceipt{Found: true, PayloadHMAC: payloadHMAC, Result: identityport.BindResult{Status: identityport.BindBound, CustomerID: command.CustomerID}}}
		events := &bindTestEvents{}
		result, err := NewBindService(&resolveTestUoW{}, store, events, key).Bind(context.Background(), command)
		if err != nil || result != store.receipt.Result || store.bindCalls != 0 || store.completeCalls != 0 || len(events.events) != 0 {
			t.Fatalf("result=%+v err=%v bind=%d complete=%d events=%d", result, err, store.bindCalls, store.completeCalls, len(events.events))
		}
	})
	t.Run("same key different canonical payload fails closed", func(t *testing.T) {
		store := &bindTestStore{receipt: BindReceipt{Found: true, PayloadHMAC: []byte("different payload hmac must not match")}}
		_, err := NewBindService(&resolveTestUoW{}, store, &bindTestEvents{}, key).Bind(context.Background(), command)
		if !errors.Is(err, ErrIdentityBindIdempotencyConflict) || store.bindCalls != 0 || store.completeCalls != 0 {
			t.Fatalf("err=%v bind=%d complete=%d", err, store.bindCalls, store.completeCalls)
		}
	})
}

func TestBindServiceDivertsExistingOtherCustomerWithoutMergeOrEvent(t *testing.T) {
	store := &bindTestStore{receipt: BindReceipt{ID: 7}, record: BindRecord{Status: identityport.BindRejected, IdentityID: 11}}
	events := &bindTestEvents{}
	result, err := NewBindService(&resolveTestUoW{}, store, events, []byte("12345678901234567890123456789012")).Bind(context.Background(), validBindCommandForTest())
	if err != nil || result != (identityport.BindResult{Status: identityport.BindRejected}) || store.completeCalls != 1 || len(events.events) != 0 {
		t.Fatalf("result=%+v err=%v complete=%d events=%d", result, err, store.completeCalls, len(events.events))
	}
}

func TestBindServiceAutoMergesOnlyVerifiedUnionIDWithUniqueWeComPrimary(t *testing.T) {
	command := validBindCommandForTest()
	command.CustomerID = 42
	command.Ref = identityport.IDRef{
		Kind:      identityport.KindUnionID,
		Scope:     "wechat-open-platform:account-a",
		Value:     "union-a",
		Assurance: identityport.AssuranceVerified,
		Source:    "wecom.callback",
	}
	store := &bindTestStore{receipt: BindReceipt{ID: 9}, record: BindRecord{
		Status:                           identityport.BindRejected,
		IdentityID:                       11,
		ExistingCustomerID:               84,
		RequestedHasVerifiedWeCom:        true,
		ExistingCustomerHasVerifiedWeCom: false,
	}}
	contacts, events := &bindTestContacts{}, &bindTestEvents{}
	service := NewBindServiceWithMergePort(&resolveTestUoW{}, store, contacts, events, []byte("12345678901234567890123456789012"))

	result, err := service.Bind(context.Background(), command)
	want := identityport.BindResult{Status: identityport.BindMerged, CustomerID: 42, PrimaryCustomerID: 42, MergeAuditID: 17}
	if err != nil || result != want || store.completed != want || store.rebindCalls != 1 || store.auditCalls != 1 {
		t.Fatalf("result=%+v err=%v completed=%+v rebind=%d audit=%d", result, err, store.completed, store.rebindCalls, store.auditCalls)
	}
	if len(contacts.merges) != 1 || contacts.merges[0].PrimaryID != 42 || contacts.merges[0].MergedID != 84 || contacts.merges[0].Reason != identityport.MergePolicyVerifiedUnionIDUniqueWeCom {
		t.Fatalf("merge calls=%+v", contacts.merges)
	}
	if len(events.events) != 1 || events.events[0].Type != eventport.EvCustomerMerged || events.events[0].CustomerID != 42 || events.events[0].IdempotencyKey != "customer.merged:17" || string(events.events[0].Payload) == command.Ref.Value {
		t.Fatalf("merge events=%+v", events.events)
	}
	if len(store.audit.ReviewFingerprint) != 16 || string(store.audit.Detail) == "" || string(store.audit.Detail) == command.Ref.Value {
		t.Fatalf("audit=%+v", store.audit)
	}
}

func TestBindServiceCreatesVerifiedPhoneManualReviewWithoutAutomaticMerge(t *testing.T) {
	command := validBindCommandForTest()
	command.CustomerID = 84
	command.Ref = identityport.IDRef{
		Kind:      identityport.KindPhone,
		Scope:     "phone:e164",
		Value:     "+86 138 0013 8000",
		Assurance: identityport.AssuranceVerified,
		Source:    "wecom.callback",
	}
	store := &bindTestStore{receipt: BindReceipt{ID: 9}, record: BindRecord{
		Status: identityport.BindRejected, IdentityID: 11, ExistingCustomerID: 42,
	}}
	events := &bindTestEvents{}
	result, err := NewBindService(&resolveTestUoW{}, store, events, []byte("12345678901234567890123456789012")).Bind(context.Background(), command)
	want := identityport.BindResult{Status: identityport.BindManualReview, ReviewID: 23}
	if err != nil || result != want || store.completed != want || store.reviewCalls != 1 || store.auditCalls != 0 || store.rebindCalls != 0 {
		t.Fatalf("result=%+v completed=%+v err=%v review=%d audit=%d rebind=%d", result, store.completed, err, store.reviewCalls, store.auditCalls, store.rebindCalls)
	}
	if got := store.candidates; !reflect.DeepEqual(got, []contactport.CustomerID{42, 84}) || len(store.fingerprint) != 16 {
		t.Fatalf("candidates=%v fingerprint=%x", got, store.fingerprint)
	}
	if len(events.events) != 1 || events.events[0].Type != "identity.merge_review.created" || events.events[0].CustomerID != eventport.CustomerID(command.CustomerID) || events.events[0].IdempotencyKey != "identity.merge_review.created:23" || string(events.events[0].Payload) == command.Ref.Value {
		t.Fatalf("events=%+v", events.events)
	}
}

func TestBindServiceRejectsInvalidCommandBeforeTransaction(t *testing.T) {
	uow, store := &resolveTestUoW{}, &bindTestStore{}
	command := validBindCommandForTest()
	command.Actor = contactport.Actor("")
	_, err := NewBindService(uow, store, &bindTestEvents{}, []byte("12345678901234567890123456789012")).Bind(context.Background(), command)
	if !errors.Is(err, ErrIdentityBindFailed) || uow.calls != 0 || store.reserveCalls != 0 {
		t.Fatalf("err=%v uow=%d reserve=%d", err, uow.calls, store.reserveCalls)
	}
}

func validBindCommandForTest() identityport.BindCommand {
	return identityport.BindCommand{
		CustomerID:     42,
		Ref:            identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 ", Assurance: identityport.AssuranceDeclared, Source: "admin"},
		Actor:          "admin:42",
		IdempotencyKey: "bind-key-42",
	}
}
