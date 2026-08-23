package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

var contactPolicyTestNow = time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)

type contactPolicyUoW struct{ calls int }

func (uow *contactPolicyUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type contactPolicyStoreStub struct {
	policy      *StoredContactPolicy
	receipt     ContactPolicyReceipt
	owned       bool
	lockCalls   int
	insertCalls int
	updateCalls int
	deleteCalls int
	complete    int
}

func (store *contactPolicyStoreStub) ReadActiveCustomerPolicy(_ context.Context, customerID contactport.CustomerID, evaluatedAt time.Time) (ContactPolicy, error) {
	if store.policy == nil {
		return emptyContactPolicy(customerID), nil
	}
	return contactPolicyProjection(*store.policy, evaluatedAt), nil
}

func (store *contactPolicyStoreStub) ReserveContactPolicyReceipt(_ context.Context, reservation ContactPolicyReceiptReservation) (ContactPolicyReceipt, bool, error) {
	if store.receipt.ID == 0 {
		store.receipt = ContactPolicyReceipt{
			ID: 7, Operation: reservation.Operation, ActorScope: reservation.ActorScope,
			KeyDigest: reservation.KeyDigest, PayloadDigest: reservation.PayloadDigest, State: "reserved",
		}
		store.owned = true
	}
	return store.receipt, store.owned, nil
}

func (store *contactPolicyStoreStub) CompleteContactPolicyReceipt(_ context.Context, id int64, snapshot json.RawMessage, _ time.Time) (ContactPolicyReceipt, error) {
	store.complete++
	if id != store.receipt.ID {
		return ContactPolicyReceipt{}, errors.New("wrong receipt")
	}
	store.receipt.State = "completed"
	store.receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
	return store.receipt, nil
}

func (store *contactPolicyStoreStub) LockContactPolicyCustomer(context.Context, contactport.CustomerID) error {
	store.lockCalls++
	return nil
}

func (store *contactPolicyStoreStub) ReadStoredContactPolicy(context.Context, contactport.CustomerID) (StoredContactPolicy, bool, error) {
	if store.policy == nil {
		return StoredContactPolicy{}, false, nil
	}
	return *store.policy, true, nil
}

func (store *contactPolicyStoreStub) InsertContactPolicy(_ context.Context, value StoredContactPolicy) (StoredContactPolicy, error) {
	store.insertCalls++
	value.Version = 1
	store.policy = &value
	return value, nil
}

func (store *contactPolicyStoreStub) UpdateContactPolicy(_ context.Context, value StoredContactPolicy, expected int64) (StoredContactPolicy, error) {
	store.updateCalls++
	if store.policy == nil || store.policy.Version != expected {
		return StoredContactPolicy{}, ErrContactPolicyConflict
	}
	value.Version = expected + 1
	store.policy = &value
	return value, nil
}

func (store *contactPolicyStoreStub) DeleteContactPolicy(_ context.Context, _ contactport.CustomerID, expected int64) (bool, error) {
	store.deleteCalls++
	if store.policy == nil || store.policy.Version != expected {
		return false, nil
	}
	store.policy = nil
	return true, nil
}

type contactPolicyAppender struct {
	events []eventport.Event
	err    error
}

func (appender *contactPolicyAppender) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	appender.events = append(appender.events, event)
	return eventport.EventID(len(appender.events)), appender.err
}

func newContactPolicyTestService(store *contactPolicyStoreStub, events *contactPolicyAppender) *ContactPolicyService {
	service := NewContactPolicyService(&contactPolicyUoW{}, store, events)
	service.now = func() time.Time { return contactPolicyTestNow }
	return service
}

func TestContactPolicyGetKeepsExpiredPolicyPresentButEligible(t *testing.T) {
	expired := contactPolicyTestNow.Add(-time.Minute)
	store := &contactPolicyStoreStub{policy: &StoredContactPolicy{
		CustomerID: 41, ReasonCode: ContactPolicyReasonManualOptOut,
		SuppressedUntil: &expired, Version: 3,
	}}
	result, err := newContactPolicyTestService(store, &contactPolicyAppender{}).Get(context.Background(), 41)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !result.PolicyPresent || !result.Eligible || result.SuppressionActive || result.Version != 3 || result.ReasonCode == nil || *result.ReasonCode != ContactPolicyReasonManualOptOut {
		t.Fatalf("expired projection = %#v", result)
	}
	if result.ProviderExecutionEligible || result.RealExternalCallExecuted || result.DeliveryProven || !result.LocalOnly {
		t.Fatalf("unsafe effect flags = %#v", result)
	}
}

func TestContactPolicySetCASReceiptEventAndStrictReadback(t *testing.T) {
	store := &contactPolicyStoreStub{}
	events := &contactPolicyAppender{}
	until := contactPolicyTestNow.Add(time.Hour)
	result, err := newContactPolicyTestService(store, events).Set(context.Background(), SetContactPolicyCommand{
		CustomerID: 42, ExpectedVersion: 0, ReasonCode: ContactPolicyReasonCompliance,
		SuppressedUntil: &until, ActorID: 9, IdempotencyKey: "contact-policy-key-0001",
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if result.Version != 1 || result.Eligible || !result.SuppressionActive || store.lockCalls != 1 || store.insertCalls != 1 || store.complete != 1 {
		t.Fatalf("result/store = %#v locks=%d inserts=%d complete=%d", result, store.lockCalls, store.insertCalls, store.complete)
	}
	if len(events.events) != 1 || events.events[0].Type != eventport.EvCustomerContactPolicyChanged || events.events[0].CustomerID != 42 {
		t.Fatalf("events = %#v", events.events)
	}
}

func TestContactPolicyReplayPrecedesCustomerAndMutationDependencies(t *testing.T) {
	snapshot, err := json.Marshal(emptyContactPolicy(43))
	if err != nil {
		t.Fatal(err)
	}
	key := "contact-policy-key-0002"
	payload, err := json.Marshal(struct {
		CustomerID      contactport.CustomerID `json:"customer_id"`
		ExpectedVersion int64                  `json:"expected_version"`
	}{43, 1})
	if err != nil {
		t.Fatal(err)
	}
	store := &contactPolicyStoreStub{owned: false, receipt: ContactPolicyReceipt{
		ID: 8, Operation: contactPolicyClearOperation, ActorScope: contactPolicyActorScope(9),
		KeyDigest: sha256.Sum256([]byte(key)), PayloadDigest: sha256.Sum256(payload),
		State: "completed", ResultSnapshot: snapshot,
	}}
	events := &contactPolicyAppender{}
	result, err := newContactPolicyTestService(store, events).Clear(context.Background(), ClearContactPolicyCommand{
		CustomerID: 43, ExpectedVersion: 1, ActorID: 9, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("Clear replay: %v", err)
	}
	if !result.Eligible || store.lockCalls != 0 || store.deleteCalls != 0 || store.complete != 0 || len(events.events) != 0 {
		t.Fatalf("replay touched dependencies: result=%#v store=%#v events=%d", result, store, len(events.events))
	}
}

func TestContactPolicyEventFailureDoesNotCompleteReceipt(t *testing.T) {
	store := &contactPolicyStoreStub{}
	events := &contactPolicyAppender{err: errors.New("event unavailable")}
	_, err := newContactPolicyTestService(store, events).Set(context.Background(), SetContactPolicyCommand{
		CustomerID: 44, ExpectedVersion: 0, ReasonCode: ContactPolicyReasonOperatorHold,
		ActorID: 9, IdempotencyKey: "contact-policy-key-0003",
	})
	if !errors.Is(err, ErrContactPolicyUnavailable) || store.complete != 0 {
		t.Fatalf("err=%v complete=%d", err, store.complete)
	}
}
