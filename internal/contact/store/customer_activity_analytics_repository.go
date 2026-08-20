package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

type CustomerActivityAnalyticsRepository struct{}

func NewCustomerActivityAnalyticsRepository() *CustomerActivityAnalyticsRepository {
	return &CustomerActivityAnalyticsRepository{}
}

func (*CustomerActivityAnalyticsRepository) ReadCustomerActivityAnalytics(ctx context.Context, query contactapp.CustomerActivityAnalyticsStoreQuery) (contactapp.CustomerActivityAnalyticsStoreResult, error) {
	if query.CustomerID <= 0 || (query.OwnerStaffID != nil && *query.OwnerStaffID <= 0) || query.From.IsZero() || query.Through.IsZero() || !query.From.Before(query.Through) || query.TypeLimit < 1 || query.TypeLimit > contactapp.CustomerActivityTypeFacetLimit+1 {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, contactport.ErrInvalidCustomerActivityAnalytics
	}
	queries, err := customerEventQueriesFromContext(ctx)
	if err != nil {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, err
	}
	shared := contactdb.LoadCustomerEventAnalyticsSummaryParams{CustomerID: int64(query.CustomerID), OwnerStaffID: nullableInt64(query.OwnerStaffID), FromTime: analyticsTimestamp(query.From), ThroughTime: analyticsTimestamp(query.Through)}
	summary, err := queries.LoadCustomerEventAnalyticsSummary(ctx, shared)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, err
	}
	result := contactapp.CustomerActivityAnalyticsStoreResult{CustomerID: contactport.CustomerID(summary.CustomerID), TotalEvents: summary.TotalEvents, ActiveDays: summary.ActiveDays, UniqueEventTypes: summary.UniqueEventTypes}
	last, err := analyticsTime(summary.LastOccurredAt)
	if err != nil {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, err
	}
	if summary.TotalEvents > 0 {
		result.LastOccurredAt = &last
	}
	types, err := queries.ListCustomerEventTypeAnalytics(ctx, contactdb.ListCustomerEventTypeAnalyticsParams{FromTime: shared.FromTime, ThroughTime: shared.ThroughTime, TypeLimit: query.TypeLimit, CustomerID: shared.CustomerID, OwnerStaffID: shared.OwnerStaffID})
	if err != nil {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, err
	}
	result.TypeFacets = make([]contactport.CustomerActivityTypeCount, 0, len(types))
	for _, row := range types {
		occurred, convertErr := analyticsTime(row.LastOccurredAt)
		if convertErr != nil {
			return contactapp.CustomerActivityAnalyticsStoreResult{}, convertErr
		}
		result.TypeFacets = append(result.TypeFacets, contactport.CustomerActivityTypeCount{EventType: row.EventType, Count: row.EventCount, LastOccurredAt: occurred})
	}
	days, err := queries.ListCustomerEventDailyAnalytics(ctx, contactdb.ListCustomerEventDailyAnalyticsParams{FromTime: shared.FromTime, ThroughTime: shared.ThroughTime, CustomerID: shared.CustomerID, OwnerStaffID: shared.OwnerStaffID})
	if err != nil {
		return contactapp.CustomerActivityAnalyticsStoreResult{}, err
	}
	result.DailyCounts = make([]contactport.CustomerActivityDayCount, 0, len(days))
	for _, row := range days {
		day, convertErr := analyticsTime(row.ActivityDay)
		if convertErr != nil {
			return contactapp.CustomerActivityAnalyticsStoreResult{}, convertErr
		}
		result.DailyCounts = append(result.DailyCounts, contactport.CustomerActivityDayCount{Day: day, Count: row.EventCount})
	}
	return result, nil
}

func analyticsTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func analyticsTime(value any) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC(), nil
	case pgtype.Timestamptz:
		if typed.Valid {
			return typed.Time.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid analytics timestamp")
}

var _ contactapp.CustomerActivityAnalyticsStore = (*CustomerActivityAnalyticsRepository)(nil)
