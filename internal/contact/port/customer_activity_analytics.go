package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCustomerActivityAnalytics     = errors.New("invalid customer activity analytics query")
	ErrCustomerActivityAnalyticsUnavailable = errors.New("customer activity analytics unavailable")
)

type CustomerActivityAnalyticsQuery struct {
	CustomerID   CustomerID
	OwnerStaffID *int64
	WindowDays   int32
}

type CustomerActivityTypeCount struct {
	EventType      string
	Count          int64
	LastOccurredAt time.Time
}

type CustomerActivityDayCount struct {
	Day   time.Time
	Count int64
}

// CustomerActivityAnalytics is a local, payload-free CRM projection. Counts
// include events attached to merged source customers, but expose no lineage,
// actor, event payload, identity value, or provider state.
type CustomerActivityAnalytics struct {
	CustomerID          CustomerID
	WindowDays          int32
	From                time.Time
	Through             time.Time
	TotalEvents         int64
	ActiveDays          int32
	UniqueEventTypes    int32
	LastOccurredAt      *time.Time
	TypeFacets          []CustomerActivityTypeCount
	TypeFacetsTruncated bool
	DailyCounts         []CustomerActivityDayCount
}

type CustomerActivityAnalyticsReader interface {
	ReadCustomerActivityAnalytics(context.Context, CustomerActivityAnalyticsQuery) (CustomerActivityAnalytics, error)
}
