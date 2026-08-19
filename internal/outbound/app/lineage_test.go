package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeliveryLineageReaderReturnsOnlySafeProjection(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	store := &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{
		{TaskID: 10, State: TaskStatusSent, AttemptCount: 2, UpdatedAt: now, SameTimestampCount: 2},
		{TaskID: 2, State: TaskStatusPending, AttemptCount: 0, UpdatedAt: now, SameTimestampCount: 2},
	}}

	page, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, store).ListDeliveryLineage(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListDeliveryLineage() error = %v", err)
	}
	if !page.Complete || store.limit != 3 || len(page.Items) != 2 {
		t.Fatalf("page=%+v store.limit=%d", page, store.limit)
	}
	if page.Items[0].LineageID != "outbound-task:10" || page.Items[1].LineageID != "outbound-task:2" {
		t.Fatalf("lineage ids=%+v, want lexical timestamp tie order", page.Items)
	}
	if page.Items[0].InternalState != "sent" || page.Items[0].AttemptCount != 2 || !page.Items[0].UpdatedAt.Equal(now) {
		t.Fatalf("item=%+v", page.Items[0])
	}
}

func TestDeliveryLineageReaderFailsClosedOnIncompleteTimestampGroup(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	store := &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{
		{TaskID: 1, State: TaskStatusSent, AttemptCount: 1, UpdatedAt: now, SameTimestampCount: 2},
	}}

	_, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, store).ListDeliveryLineage(context.Background(), 1)
	if !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("ListDeliveryLineage() error = %v, want unavailable", err)
	}
}

func TestDeliveryLineageReaderFailsClosedOnIncorrectEarlierTimestampGroup(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	store := &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{
		{TaskID: 2, State: TaskStatusSent, AttemptCount: 1, UpdatedAt: now.Add(time.Second), SameTimestampCount: 2},
		{TaskID: 1, State: TaskStatusPending, AttemptCount: 0, UpdatedAt: now, SameTimestampCount: 1},
	}}

	_, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, store).ListDeliveryLineage(context.Background(), 2)
	if !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("ListDeliveryLineage() error = %v, want unavailable", err)
	}
}

func TestDeliveryLineageReaderRejectsInvalidInputAndStoreFailure(t *testing.T) {
	reader := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, &deliveryLineageStoreStub{err: errors.New("store unavailable")})
	if _, err := reader.ListDeliveryLineage(context.Background(), 0); !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("zero limit error = %v", err)
	}
	if _, err := reader.ListDeliveryLineage(context.Background(), 1); !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("store error = %v", err)
	}
}

type inlineDeliveryLineageUOW struct{}

func (inlineDeliveryLineageUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type deliveryLineageStoreStub struct {
	records []DeliveryLineageStoreRecord
	err     error
	limit   int32
}

func (stub *deliveryLineageStoreStub) ListDeliveryLineage(_ context.Context, limit int32) ([]DeliveryLineageStoreRecord, error) {
	stub.limit = limit
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]DeliveryLineageStoreRecord(nil), stub.records...), nil
}
