package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

func TestDeliveryLineageReaderHashesEventIdentity(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	store := &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{
		{EventID: 7, Consumer: "automation.tag-trigger.v1", State: eventport.DeliveryCompleted, AttemptCount: 1, UpdatedAt: now, SameTimestampCount: 2},
		{EventID: 8, Consumer: "stats.tag-applied.v1", State: eventport.DeliveryOutcomeUnknown, AttemptCount: 0, UpdatedAt: now, SameTimestampCount: 2},
	}}
	reader, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, store, key)
	if err != nil {
		t.Fatalf("NewDeliveryLineageReader() error = %v", err)
	}
	page, err := reader.ListDeliveryLineage(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListDeliveryLineage() error = %v", err)
	}
	if !page.Complete || store.limit != 3 || len(page.Items) != 2 {
		t.Fatalf("page=%+v store.limit=%d", page, store.limit)
	}
	for _, item := range page.Items {
		if !strings.HasPrefix(item.LineageID, "event-delivery:v1:") || len(item.LineageID) != len("event-delivery:v1:")+64 || strings.Contains(item.LineageID, "automation") || item.LineageID == "event-delivery:7" {
			t.Fatalf("opaque lineage id=%q", item.LineageID)
		}
	}
	if page.Items[0].LineageID > page.Items[1].LineageID {
		t.Fatalf("lineage ids not ordered within timestamp group: %+v", page.Items)
	}
}

func TestDeliveryLineageReaderFailsClosedOnInvalidOrIncompleteRows(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	for _, record := range []DeliveryLineageStoreRecord{
		{EventID: 1, Consumer: "consumer", State: eventport.DeliveryPending, UpdatedAt: now, SameTimestampCount: 2},
		{EventID: 1, Consumer: " consumer", State: eventport.DeliveryPending, UpdatedAt: now, SameTimestampCount: 1},
		{EventID: 1, Consumer: "consumer", State: "sent", UpdatedAt: now, SameTimestampCount: 1},
	} {
		reader, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{record}}, key)
		if err != nil {
			t.Fatalf("NewDeliveryLineageReader() error = %v", err)
		}
		if _, err := reader.ListDeliveryLineage(context.Background(), 1); !errors.Is(err, ErrDeliveryLineageUnavailable) {
			t.Fatalf("record=%+v error=%v, want unavailable", record, err)
		}
	}
}

func TestDeliveryLineageReaderFailsClosedOnIncorrectEarlierTimestampGroup(t *testing.T) {
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	reader, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, &deliveryLineageStoreStub{records: []DeliveryLineageStoreRecord{
		{EventID: 2, Consumer: "newer", State: eventport.DeliveryCompleted, AttemptCount: 1, UpdatedAt: now.Add(time.Second), SameTimestampCount: 2},
		{EventID: 1, Consumer: "older", State: eventport.DeliveryPending, AttemptCount: 0, UpdatedAt: now, SameTimestampCount: 1},
	}}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("NewDeliveryLineageReader() error = %v", err)
	}
	if _, err := reader.ListDeliveryLineage(context.Background(), 2); !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("ListDeliveryLineage() error = %v, want unavailable", err)
	}
}

func TestDeliveryLineageReaderRejectsBadKeyAndStoreFailure(t *testing.T) {
	if _, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, &deliveryLineageStoreStub{}, []byte("short")); !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("bad key error=%v", err)
	}
	reader, err := NewDeliveryLineageReader(inlineDeliveryLineageUOW{}, &deliveryLineageStoreStub{err: errors.New("store unavailable")}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("NewDeliveryLineageReader() error=%v", err)
	}
	if _, err := reader.ListDeliveryLineage(context.Background(), 1); !errors.Is(err, ErrDeliveryLineageUnavailable) {
		t.Fatalf("store error=%v", err)
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
