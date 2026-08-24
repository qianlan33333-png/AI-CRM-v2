package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type customerSafeExportTestUoW struct{}

func (customerSafeExportTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type customerSafeExportStoreStub struct {
	receipt CustomerSafeExportReceipt
	owned   bool
	created int
}

func (store *customerSafeExportStoreStub) ReserveCustomerSafeExportReceipt(_ context.Context, _ int64, _ [32]byte, payload [32]byte, _ time.Time) (CustomerSafeExportReceipt, bool, error) {
	if store.receipt.ID == 0 {
		store.receipt = CustomerSafeExportReceipt{ID: 1, PayloadDigest: payload}
		store.owned = true
	}
	return store.receipt, store.owned, nil
}
func (store *customerSafeExportStoreStub) CreateCustomerSafeExport(_ context.Context, _ CustomerSafeExport, _ int64, _ *int64, _ [32]byte, _ CustomerListQuery) ([]CustomerSafeExportRow, error) {
	store.created++
	return []CustomerSafeExportRow{{CustomerID: 10, DisplayName: "=formula"}}, nil
}
func (store *customerSafeExportStoreStub) CompleteCustomerSafeExportReceipt(_ context.Context, _ int64, _ string, snapshot json.RawMessage, _ time.Time) (CustomerSafeExportReceipt, error) {
	store.receipt.Completed = true
	store.receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
	return store.receipt, nil
}
func (store *customerSafeExportStoreStub) ReadCustomerSafeExport(context.Context, string, int64) (CustomerSafeExport, error) {
	return CustomerSafeExport{}, ErrCustomerSafeExportNotFound
}
func (store *customerSafeExportStoreStub) ReadCustomerSafeExportRows(context.Context, string, int64, *int64) ([]CustomerSafeExportRow, error) {
	return nil, nil
}

type customerSafeExportEvents struct {
	events []eventport.Event
	err    error
}

func (events *customerSafeExportEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.events = append(events.events, event)
	return 1, events.err
}

func newCustomerSafeExportTestService(store *customerSafeExportStoreStub, events *customerSafeExportEvents) *CustomerSafeExportService {
	service := NewCustomerSafeExportService(customerSafeExportTestUoW{}, store, events)
	service.now = func() time.Time { return time.Date(2026, time.August, 24, 8, 0, 0, 0, time.UTC) }
	return service
}

func TestCustomerSafeExportCreateReceiptsAndReplays(t *testing.T) {
	store, events := &customerSafeExportStoreStub{}, &customerSafeExportEvents{}
	service := newCustomerSafeExportTestService(store, events)
	command := CustomerSafeExportCreate{ActorID: 7, IdempotencyKey: "customer-safe-export-key-0001", Filter: CustomerListInput{Keyword: "customer"}}
	first, err := service.Create(context.Background(), command)
	if err != nil || first.RecordCount != 1 || store.created != 1 || len(events.events) != 1 || events.events[0].Type != eventport.EvCustomerSafeExportCreated {
		t.Fatalf("first=%+v created=%d events=%+v err=%v", first, store.created, events.events, err)
	}
	store.owned = false
	replay, err := service.Create(context.Background(), command)
	if err != nil || replay.ID != first.ID || store.created != 1 || len(events.events) != 1 {
		t.Fatalf("replay=%+v created=%d events=%d err=%v", replay, store.created, len(events.events), err)
	}
}

func TestCustomerSafeExportReservedReceiptFailsClosed(t *testing.T) {
	store := &customerSafeExportStoreStub{receipt: CustomerSafeExportReceipt{ID: 1}, owned: false}
	_, err := newCustomerSafeExportTestService(store, &customerSafeExportEvents{}).Create(context.Background(), CustomerSafeExportCreate{ActorID: 7, IdempotencyKey: "customer-safe-export-key-0002"})
	if err == nil {
		t.Fatal("reserved receipt was accepted as a successful replay")
	}
}

func TestCustomerSafeExportSalesScopeCannotWiden(t *testing.T) {
	one, two := int64(1), int64(2)
	_, err := newCustomerSafeExportTestService(&customerSafeExportStoreStub{}, &customerSafeExportEvents{}).Create(context.Background(), CustomerSafeExportCreate{ActorID: 7, OwnerScopeStaffID: &one, IdempotencyKey: "customer-safe-export-key-0003", Filter: CustomerListInput{OwnerStaffID: &two}})
	if !errors.Is(err, ErrCustomerSafeExportConflict) {
		t.Fatalf("widen scope error=%v", err)
	}
}
