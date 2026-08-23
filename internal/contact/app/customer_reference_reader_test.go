package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestCustomerReferenceReaderResolvesOneThousandIDsWithOneSortedStoreCall(t *testing.T) {
	ids := make([]contactport.CustomerID, CustomerListMaximumLimit*5)
	for index := range ids {
		ids[index] = contactport.CustomerID(len(ids) - index)
	}
	store := &customerReferenceStoreStub{}
	reader := NewCustomerReferenceReader(store)

	items, err := reader.ReadInTransaction(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || len(store.ids) != len(ids) || len(items) != len(ids) {
		t.Fatalf("store calls/ids/items = %d/%d/%d, want 1/%d/%d", store.calls, len(store.ids), len(items), len(ids), len(ids))
	}
	want := make([]contactport.CustomerID, len(ids))
	for index := range want {
		want[index] = contactport.CustomerID(index + 1)
	}
	if !reflect.DeepEqual(store.ids, want) {
		t.Fatalf("store ids = %v, want one sorted batch", store.ids)
	}
	for index, item := range items {
		if item.ID != want[index] {
			t.Fatalf("item %d = %d, want %d", index, item.ID, want[index])
		}
	}
}

func TestCustomerReferenceReaderFailsClosedWhenBatchCannotProveEveryID(t *testing.T) {
	store := &customerReferenceStoreStub{records: []CustomerReferenceRecord{{ID: 1}}}
	_, err := NewCustomerReferenceReader(store).ReadInTransaction(context.Background(), []contactport.CustomerID{1, 2})
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("ReadInTransaction() error = %v, want missing customer", err)
	}
}

type customerReferenceStoreStub struct {
	calls   int
	ids     []contactport.CustomerID
	records []CustomerReferenceRecord
	err     error
}

func (store *customerReferenceStoreStub) ReadActiveCustomerReferences(_ context.Context, ids []contactport.CustomerID) ([]CustomerReferenceRecord, error) {
	store.calls++
	store.ids = append([]contactport.CustomerID(nil), ids...)
	if store.err != nil {
		return nil, store.err
	}
	if store.records != nil {
		return append([]CustomerReferenceRecord(nil), store.records...), nil
	}
	items := make([]CustomerReferenceRecord, len(ids))
	for index, id := range ids {
		items[index] = CustomerReferenceRecord{ID: id}
	}
	return items, nil
}
