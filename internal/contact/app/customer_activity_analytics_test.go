package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type activityAnalyticsUOW struct{ calls int }

func (uow *activityAnalyticsUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type activityAnalyticsStore struct {
	result CustomerActivityAnalyticsStoreResult
	err    error
	calls  int
	query  CustomerActivityAnalyticsStoreQuery
}

func (store *activityAnalyticsStore) ReadCustomerActivityAnalytics(_ context.Context, query CustomerActivityAnalyticsStoreQuery) (CustomerActivityAnalyticsStoreResult, error) {
	store.calls++
	store.query = query
	return store.result, store.err
}

func activityAnalyticsNow() time.Time {
	return time.Date(2026, time.August, 20, 10, 30, 0, 0, time.UTC)
}
func activityAnalyticsFact() CustomerActivityAnalyticsStoreResult {
	return CustomerActivityAnalyticsStoreResult{CustomerID: 41, TotalEvents: 3, ActiveDays: 2, UniqueEventTypes: 2, LastOccurredAt: activityTime(20, 9), TypeFacets: []contactport.CustomerActivityTypeCount{{EventType: "customer.updated", Count: 2, LastOccurredAt: *activityTime(20, 9)}, {EventType: "customer.created", Count: 1, LastOccurredAt: *activityTime(19, 9)}}, DailyCounts: []contactport.CustomerActivityDayCount{{Day: activityDay(19), Count: 1}, {Day: activityDay(20), Count: 2}}}
}
func activityTime(day, hour int) *time.Time {
	value := time.Date(2026, time.August, day, hour, 0, 0, 0, time.UTC)
	return &value
}
func activityDay(day int) time.Time { return time.Date(2026, time.August, day, 0, 0, 0, 0, time.UTC) }

func TestCustomerActivityAnalyticsReadsScopedPayloadFreeAggregation(t *testing.T) {
	owner := int64(71)
	uow := &activityAnalyticsUOW{}
	store := &activityAnalyticsStore{result: activityAnalyticsFact()}
	result, err := NewCustomerActivityAnalyticsService(uow, store, activityAnalyticsNow).ReadCustomerActivityAnalytics(context.Background(), contactport.CustomerActivityAnalyticsQuery{CustomerID: 41, OwnerStaffID: &owner, WindowDays: 30})
	if err != nil {
		t.Fatalf("ReadCustomerActivityAnalytics() error = %v", err)
	}
	if result.CustomerID != 41 || result.WindowDays != 30 || result.TotalEvents != 3 || result.ActiveDays != 2 || result.UniqueEventTypes != 2 || len(result.TypeFacets) != 2 || len(result.DailyCounts) != 2 || result.TypeFacetsTruncated {
		t.Fatalf("result = %#v", result)
	}
	if uow.calls != 1 || store.calls != 1 || store.query.OwnerStaffID == nil || *store.query.OwnerStaffID != owner || store.query.TypeLimit != 51 || !store.query.Through.Equal(activityAnalyticsNow()) || !store.query.From.Equal(activityAnalyticsNow().AddDate(0, 0, -30)) {
		t.Fatalf("store query = %#v calls=%d/%d", store.query, uow.calls, store.calls)
	}
}

func TestCustomerActivityAnalyticsSupportsEmptyAndTruncatesTypes(t *testing.T) {
	empty := &activityAnalyticsStore{result: CustomerActivityAnalyticsStoreResult{CustomerID: 41, TypeFacets: []contactport.CustomerActivityTypeCount{}, DailyCounts: []contactport.CustomerActivityDayCount{}}}
	result, err := NewCustomerActivityAnalyticsService(&activityAnalyticsUOW{}, empty, activityAnalyticsNow).ReadCustomerActivityAnalytics(context.Background(), contactport.CustomerActivityAnalyticsQuery{CustomerID: 41, WindowDays: 7})
	if err != nil || result.TotalEvents != 0 || result.LastOccurredAt != nil {
		t.Fatalf("empty result=%#v err=%v", result, err)
	}
	facets := make([]contactport.CustomerActivityTypeCount, 51)
	for index := range facets {
		facets[index] = contactport.CustomerActivityTypeCount{EventType: fmt.Sprintf("event.%03d", index), Count: int64(51 - index), LastOccurredAt: *activityTime(20, 9)}
	}
	store := &activityAnalyticsStore{result: CustomerActivityAnalyticsStoreResult{CustomerID: 41, TotalEvents: 1326, ActiveDays: 1, UniqueEventTypes: 51, LastOccurredAt: activityTime(20, 9), TypeFacets: facets, DailyCounts: []contactport.CustomerActivityDayCount{{Day: activityDay(20), Count: 1326}}}}
	if !validCustomerActivityAnalyticsResult(store.result, CustomerActivityAnalyticsStoreQuery{CustomerID: 41, From: activityAnalyticsNow().AddDate(0, 0, -7), Through: activityAnalyticsNow(), TypeLimit: 51}) {
		t.Fatal("truncated fixture must satisfy store contract")
	}
	result, err = NewCustomerActivityAnalyticsService(&activityAnalyticsUOW{}, store, activityAnalyticsNow).ReadCustomerActivityAnalytics(context.Background(), contactport.CustomerActivityAnalyticsQuery{CustomerID: 41, WindowDays: 7})
	if err != nil || !result.TypeFacetsTruncated || len(result.TypeFacets) != 50 {
		t.Fatalf("truncated result=%#v err=%v", result, err)
	}
}

func TestCustomerActivityAnalyticsFailsClosedOnInvalidInputStoreAndDependencies(t *testing.T) {
	for _, query := range []contactport.CustomerActivityAnalyticsQuery{{CustomerID: 0, WindowDays: 30}, {CustomerID: 41, WindowDays: 8}} {
		store := &activityAnalyticsStore{}
		_, err := NewCustomerActivityAnalyticsService(&activityAnalyticsUOW{}, store, activityAnalyticsNow).ReadCustomerActivityAnalytics(context.Background(), query)
		if !errors.Is(err, contactport.ErrInvalidCustomerActivityAnalytics) || store.calls != 0 {
			t.Fatalf("query=%#v err=%v calls=%d", query, err, store.calls)
		}
	}
	if _, err := (*CustomerActivityAnalyticsService)(nil).ReadCustomerActivityAnalytics(context.Background(), contactport.CustomerActivityAnalyticsQuery{CustomerID: 41, WindowDays: 30}); !errors.Is(err, contactport.ErrCustomerActivityAnalyticsUnavailable) {
		t.Fatalf("nil service err=%v", err)
	}
	bad := activityAnalyticsFact()
	bad.DailyCounts[1].Count = 3
	if _, err := NewCustomerActivityAnalyticsService(&activityAnalyticsUOW{}, &activityAnalyticsStore{result: bad}, activityAnalyticsNow).ReadCustomerActivityAnalytics(context.Background(), contactport.CustomerActivityAnalyticsQuery{CustomerID: 41, WindowDays: 30}); !errors.Is(err, contactport.ErrCustomerActivityAnalyticsUnavailable) {
		t.Fatalf("bad store err=%v", err)
	}
}
