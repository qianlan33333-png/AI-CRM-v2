package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type mergeHistoryStoreFake struct {
	rows       []identityport.CustomerMergeHistory
	err        error
	customerID contactport.CustomerID
	afterID    int64
	limit      int32
}

func (fake *mergeHistoryStoreFake) ListCustomerMergeHistory(
	_ context.Context,
	customerID contactport.CustomerID,
	afterID int64,
	limit int32,
) ([]identityport.CustomerMergeHistory, error) {
	fake.customerID, fake.afterID, fake.limit = customerID, afterID, limit
	return append([]identityport.CustomerMergeHistory(nil), fake.rows...), fake.err
}

func mergeHistoryItem(id int64) identityport.CustomerMergeHistory {
	return identityport.CustomerMergeHistory{
		MergeAuditID: id, PrimaryCustomerID: 41, MergedCustomerID: contactport.CustomerID(id + 100),
		Mode: "auto", PolicyVersion: "verified_unionid_unique_wecom_v1",
		MergedAt: time.Date(2026, time.August, 20, 12, int(id), 0, 0, time.UTC),
	}
}

func TestCustomerMergeHistoryServicePaginatesSafeDescendingAuditRecords(t *testing.T) {
	store := &mergeHistoryStoreFake{rows: []identityport.CustomerMergeHistory{mergeHistoryItem(9), mergeHistoryItem(8), mergeHistoryItem(7)}}
	service := NewCustomerMergeHistoryService(&resolveTestUoW{}, store)
	page, err := service.ListCustomerMergeHistory(context.Background(), 41, "", 2)
	if err != nil {
		t.Fatalf("ListCustomerMergeHistory() error = %v", err)
	}
	if page.CustomerID != 41 || len(page.Items) != 2 || page.Items[0].MergeAuditID != 9 || page.Items[1].MergeAuditID != 8 || page.NextCursor == "" {
		t.Fatalf("page = %#v", page)
	}
	if store.customerID != 41 || store.afterID != 0 || store.limit != 3 {
		t.Fatalf("store query = customer:%d after:%d limit:%d", store.customerID, store.afterID, store.limit)
	}

	store.rows = []identityport.CustomerMergeHistory{mergeHistoryItem(7)}
	next, err := service.ListCustomerMergeHistory(context.Background(), 41, page.NextCursor, 2)
	if err != nil || len(next.Items) != 1 || next.NextCursor != "" || store.afterID != 8 {
		t.Fatalf("next=%#v err=%v after=%d", next, err, store.afterID)
	}
	_, err = service.ListCustomerMergeHistory(context.Background(), 42, page.NextCursor, 2)
	if !errors.Is(err, ErrCustomerMergeHistoryInvalid) {
		t.Fatalf("cross-customer cursor error = %v", err)
	}
}

func TestCustomerMergeHistoryServiceFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		rows []identityport.CustomerMergeHistory
	}{
		{name: "ascending", rows: []identityport.CustomerMergeHistory{mergeHistoryItem(8), mergeHistoryItem(9)}},
		{name: "duplicate", rows: []identityport.CustomerMergeHistory{mergeHistoryItem(8), mergeHistoryItem(8)}},
		{name: "same customers", rows: []identityport.CustomerMergeHistory{{MergeAuditID: 1, PrimaryCustomerID: 41, MergedCustomerID: 41, Mode: "auto", PolicyVersion: "v1", MergedAt: time.Now()}}},
		{name: "unsafe policy", rows: []identityport.CustomerMergeHistory{{MergeAuditID: 1, PrimaryCustomerID: 41, MergedCustomerID: 42, Mode: "manual", PolicyVersion: " secret ", MergedAt: time.Now()}}},
		{name: "unknown mode", rows: []identityport.CustomerMergeHistory{{MergeAuditID: 1, PrimaryCustomerID: 41, MergedCustomerID: 42, Mode: "provider", PolicyVersion: "v1", MergedAt: time.Now()}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			page, err := NewCustomerMergeHistoryService(&resolveTestUoW{}, &mergeHistoryStoreFake{rows: testCase.rows}).ListCustomerMergeHistory(context.Background(), 41, "", 50)
			if !errors.Is(err, ErrCustomerMergeHistoryUnavailable) || !reflect.DeepEqual(page, identityport.CustomerMergeHistoryPage{}) {
				t.Fatalf("page=%#v err=%v", page, err)
			}
		})
	}

	store := &mergeHistoryStoreFake{err: errors.New("db")}
	page, err := NewCustomerMergeHistoryService(&resolveTestUoW{}, store).ListCustomerMergeHistory(context.Background(), 41, "", 50)
	if !errors.Is(err, ErrCustomerMergeHistoryUnavailable) || !reflect.DeepEqual(page, identityport.CustomerMergeHistoryPage{}) {
		t.Fatalf("store failure page=%#v err=%v", page, err)
	}
	for _, input := range []struct {
		customerID contactport.CustomerID
		limit      int32
	}{{0, 50}, {41, -1}, {41, 101}} {
		page, err = NewCustomerMergeHistoryService(&resolveTestUoW{}, store).ListCustomerMergeHistory(context.Background(), input.customerID, "", input.limit)
		if !errors.Is(err, ErrCustomerMergeHistoryInvalid) || !reflect.DeepEqual(page, identityport.CustomerMergeHistoryPage{}) {
			t.Fatalf("invalid input %#v page=%#v err=%v", input, page, err)
		}
	}
}
