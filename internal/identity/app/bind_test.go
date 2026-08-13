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
	completeCalls int
	identity      NormalizedIdentity
	customerID    int64
	completed     identityport.BindResult
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
