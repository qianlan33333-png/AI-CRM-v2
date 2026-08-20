package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const CustomerActivityTypeFacetLimit int32 = 50

type CustomerActivityAnalyticsStoreQuery struct {
	CustomerID   contactport.CustomerID
	OwnerStaffID *int64
	From         time.Time
	Through      time.Time
	TypeLimit    int32
}

type CustomerActivityAnalyticsStoreResult struct {
	CustomerID       contactport.CustomerID
	TotalEvents      int64
	ActiveDays       int32
	UniqueEventTypes int32
	LastOccurredAt   *time.Time
	TypeFacets       []contactport.CustomerActivityTypeCount
	DailyCounts      []contactport.CustomerActivityDayCount
}

type CustomerActivityAnalyticsStore interface {
	ReadCustomerActivityAnalytics(context.Context, CustomerActivityAnalyticsStoreQuery) (CustomerActivityAnalyticsStoreResult, error)
}

type CustomerActivityAnalyticsService struct {
	uow   platformport.UnitOfWork
	store CustomerActivityAnalyticsStore
	now   func() time.Time
}

func NewCustomerActivityAnalyticsService(uow platformport.UnitOfWork, store CustomerActivityAnalyticsStore, now func() time.Time) *CustomerActivityAnalyticsService {
	return &CustomerActivityAnalyticsService{uow: uow, store: store, now: now}
}

func (service *CustomerActivityAnalyticsService) ReadCustomerActivityAnalytics(ctx context.Context, query contactport.CustomerActivityAnalyticsQuery) (contactport.CustomerActivityAnalytics, error) {
	if ctx == nil || query.CustomerID <= 0 || (query.OwnerStaffID != nil && *query.OwnerStaffID <= 0) || !validCustomerActivityWindow(query.WindowDays) {
		return contactport.CustomerActivityAnalytics{}, contactport.ErrInvalidCustomerActivityAnalytics
	}
	if service == nil || nilActivityAnalyticsDependency(service.uow) || nilActivityAnalyticsDependency(service.store) || service.now == nil {
		return contactport.CustomerActivityAnalytics{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	if err := ctx.Err(); err != nil {
		return contactport.CustomerActivityAnalytics{}, errors.Join(contactport.ErrCustomerActivityAnalyticsUnavailable, err)
	}
	through := service.now().UTC()
	if through.IsZero() {
		return contactport.CustomerActivityAnalytics{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	from := through.AddDate(0, 0, -int(query.WindowDays))
	storeQuery := CustomerActivityAnalyticsStoreQuery{CustomerID: query.CustomerID, OwnerStaffID: cloneInt64(query.OwnerStaffID), From: from, Through: through, TypeLimit: CustomerActivityTypeFacetLimit + 1}
	var stored CustomerActivityAnalyticsStoreResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		stored, storeErr = service.store.ReadCustomerActivityAnalytics(txCtx, storeQuery)
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return contactport.CustomerActivityAnalytics{}, ErrCustomerNotFound
		}
		return contactport.CustomerActivityAnalytics{}, errors.Join(contactport.ErrCustomerActivityAnalyticsUnavailable, err)
	}
	if !validCustomerActivityAnalyticsResult(stored, storeQuery) {
		return contactport.CustomerActivityAnalytics{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	truncated := len(stored.TypeFacets) > int(CustomerActivityTypeFacetLimit)
	if truncated {
		stored.TypeFacets = stored.TypeFacets[:CustomerActivityTypeFacetLimit]
	}
	return contactport.CustomerActivityAnalytics{
		CustomerID: stored.CustomerID, WindowDays: query.WindowDays, From: from, Through: through,
		TotalEvents: stored.TotalEvents, ActiveDays: stored.ActiveDays, UniqueEventTypes: stored.UniqueEventTypes,
		LastOccurredAt: cloneActivityAnalyticsTime(stored.LastOccurredAt), TypeFacets: append([]contactport.CustomerActivityTypeCount(nil), stored.TypeFacets...),
		TypeFacetsTruncated: truncated, DailyCounts: append([]contactport.CustomerActivityDayCount(nil), stored.DailyCounts...),
	}, nil
}

func validCustomerActivityWindow(days int32) bool { return days == 7 || days == 30 || days == 90 }

func cloneActivityAnalyticsTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func validCustomerActivityAnalyticsResult(result CustomerActivityAnalyticsStoreResult, query CustomerActivityAnalyticsStoreQuery) bool {
	if result.CustomerID != query.CustomerID || result.TotalEvents < 0 || result.ActiveDays < 0 || result.UniqueEventTypes < 0 || result.ActiveDays > int32(query.Through.Sub(query.From).Hours()/24)+1 || int64(result.UniqueEventTypes) > result.TotalEvents || (result.TotalEvents == 0) != (result.LastOccurredAt == nil) || len(result.TypeFacets) > int(query.TypeLimit) {
		return false
	}
	if result.LastOccurredAt != nil && (result.LastOccurredAt.Before(query.From) || result.LastOccurredAt.After(query.Through)) {
		return false
	}
	var counted int64
	for index, facet := range result.TypeFacets {
		if facet.Count <= 0 || facet.EventType != strings.TrimSpace(facet.EventType) || facet.EventType == "" || utf8.RuneCountInString(facet.EventType) > 200 || !utf8.ValidString(facet.EventType) || facet.LastOccurredAt.Before(query.From) || facet.LastOccurredAt.After(query.Through) {
			return false
		}
		if index > 0 && (result.TypeFacets[index-1].Count < facet.Count || (result.TypeFacets[index-1].Count == facet.Count && result.TypeFacets[index-1].EventType >= facet.EventType)) {
			return false
		}
		counted += facet.Count
	}
	if int32(len(result.TypeFacets)) > result.UniqueEventTypes || (!((int32(len(result.TypeFacets)) == result.UniqueEventTypes) && counted == result.TotalEvents) && len(result.TypeFacets) < int(query.TypeLimit)) {
		return false
	}
	var daily int64
	for index, item := range result.DailyCounts {
		if item.Count <= 0 || item.Day.Location() != time.UTC || !item.Day.Equal(item.Day.Truncate(24*time.Hour)) || item.Day.Before(query.From.Truncate(24*time.Hour)) || item.Day.After(query.Through.Truncate(24*time.Hour)) || (index > 0 && !result.DailyCounts[index-1].Day.Before(item.Day)) {
			return false
		}
		daily += item.Count
	}
	return daily == result.TotalEvents && int32(len(result.DailyCounts)) == result.ActiveDays
}

func nilActivityAnalyticsDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

var _ contactport.CustomerActivityAnalyticsReader = (*CustomerActivityAnalyticsService)(nil)
