package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type CustomerActivityAnalyticsParams struct{ WindowDays *int }

type customerActivityAnalyticsApplication interface {
	ReadCustomerActivityAnalytics(context.Context, contactport.CustomerActivityAnalyticsQuery) (contactport.CustomerActivityAnalytics, error)
}

type CustomerActivityAnalyticsHandler struct {
	application customerActivityAnalyticsApplication
}

func NewCustomerActivityAnalyticsHandler(application customerActivityAnalyticsApplication) (*CustomerActivityAnalyticsHandler, error) {
	if nilCustomerActivityAnalyticsApplication(application) {
		return nil, errors.New("customer activity analytics application is required")
	}
	return &CustomerActivityAnalyticsHandler{application: application}, nil
}

func (handler *CustomerActivityAnalyticsHandler) GetCustomerActivityAnalytics(writer http.ResponseWriter, request *http.Request, customerID int64, params CustomerActivityAnalyticsParams) {
	if handler == nil || nilCustomerActivityAnalyticsApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, contactport.ErrCustomerActivityAnalyticsUnavailable))
		return
	}
	owner, err := customerActivityAnalyticsOwner(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	window := int32(30)
	if params.WindowDays != nil {
		window = int32(*params.WindowDays)
	}
	if customerID <= 0 || !validActivityWindow(window) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, contactport.ErrInvalidCustomerActivityAnalytics))
		return
	}
	result, err := handler.application.ReadCustomerActivityAnalytics(request.Context(), contactport.CustomerActivityAnalyticsQuery{CustomerID: contactport.CustomerID(customerID), OwnerStaffID: owner, WindowDays: window})
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, contactapp.ErrCustomerNotFound) {
			code = platformhttp.CodeNotFound
		}
		if errors.Is(err, contactport.ErrInvalidCustomerActivityAnalytics) {
			code = platformhttp.CodeMalformedRequest
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	response, err := activityAnalyticsResponse(result)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, err))
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(response)
}

type customerActivityAnalyticsResponse struct {
	CustomerID               int64                          `json:"customer_id"`
	WindowDays               int32                          `json:"window_days"`
	From                     time.Time                      `json:"from"`
	Through                  time.Time                      `json:"through"`
	TotalEvents              int64                          `json:"total_events"`
	ActiveDays               int32                          `json:"active_days"`
	UniqueEventTypes         int32                          `json:"unique_event_types"`
	LastOccurredAt           *time.Time                     `json:"last_occurred_at"`
	TypeFacets               []customerActivityTypeResponse `json:"type_facets"`
	TypeFacetsTruncated      bool                           `json:"type_facets_truncated"`
	DailyCounts              []customerActivityDayResponse  `json:"daily_counts"`
	PayloadIncluded          bool                           `json:"payload_included"`
	ActorIncluded            bool                           `json:"actor_included"`
	IdentityIncluded         bool                           `json:"identity_included"`
	RealExternalCallExecuted bool                           `json:"real_external_call_executed"`
}

type customerActivityTypeResponse struct {
	EventType      string    `json:"event_type"`
	Count          int64     `json:"count"`
	LastOccurredAt time.Time `json:"last_occurred_at"`
}
type customerActivityDayResponse struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

func activityAnalyticsResponse(result contactport.CustomerActivityAnalytics) (customerActivityAnalyticsResponse, error) {
	if result.CustomerID <= 0 || !validActivityWindow(result.WindowDays) || result.From.IsZero() || result.Through.IsZero() || !result.From.Before(result.Through) || result.TotalEvents < 0 || result.ActiveDays < 0 || result.UniqueEventTypes < 0 || (result.TotalEvents == 0) != (result.LastOccurredAt == nil) {
		return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	if !result.From.Equal(result.Through.AddDate(0, 0, -int(result.WindowDays))) || result.TotalEvents > 1<<53-1 || result.ActiveDays > result.WindowDays+1 || int64(result.UniqueEventTypes) > result.TotalEvents || len(result.TypeFacets) > 50 || (result.TypeFacetsTruncated && len(result.TypeFacets) != 50) || int32(len(result.TypeFacets)) > result.UniqueEventTypes || len(result.DailyCounts) != int(result.ActiveDays) {
		return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	if result.LastOccurredAt != nil && (result.LastOccurredAt.Before(result.From) || result.LastOccurredAt.After(result.Through)) {
		return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	response := customerActivityAnalyticsResponse{CustomerID: int64(result.CustomerID), WindowDays: result.WindowDays, From: result.From.UTC(), Through: result.Through.UTC(), TotalEvents: result.TotalEvents, ActiveDays: result.ActiveDays, UniqueEventTypes: result.UniqueEventTypes, LastOccurredAt: cloneActivityTime(result.LastOccurredAt), TypeFacets: make([]customerActivityTypeResponse, 0, len(result.TypeFacets)), TypeFacetsTruncated: result.TypeFacetsTruncated, DailyCounts: make([]customerActivityDayResponse, 0, len(result.DailyCounts)), PayloadIncluded: false, ActorIncluded: false, IdentityIncluded: false, RealExternalCallExecuted: false}
	var typeCount int64
	for index, facet := range result.TypeFacets {
		if facet.EventType == "" || facet.EventType != strings.TrimSpace(facet.EventType) || !utf8.ValidString(facet.EventType) || utf8.RuneCountInString(facet.EventType) > 200 || strings.IndexFunc(facet.EventType, func(value rune) bool { return value < 0x20 || value == 0x7f }) >= 0 || facet.Count <= 0 || facet.Count > 1<<53-1 || facet.LastOccurredAt.Before(result.From) || facet.LastOccurredAt.After(result.Through) {
			return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
		}
		if index > 0 && (result.TypeFacets[index-1].Count < facet.Count || (result.TypeFacets[index-1].Count == facet.Count && result.TypeFacets[index-1].EventType >= facet.EventType)) {
			return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
		}
		typeCount += facet.Count
		response.TypeFacets = append(response.TypeFacets, customerActivityTypeResponse{EventType: facet.EventType, Count: facet.Count, LastOccurredAt: facet.LastOccurredAt.UTC()})
	}
	if !result.TypeFacetsTruncated && typeCount != result.TotalEvents {
		return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	var dailyCount int64
	for index, day := range result.DailyCounts {
		utcDay := day.Day.UTC()
		if day.Day.IsZero() || !utcDay.Equal(utcDay.Truncate(24*time.Hour)) || utcDay.Before(result.From.UTC().Truncate(24*time.Hour)) || utcDay.After(result.Through.UTC().Truncate(24*time.Hour)) || day.Count <= 0 || day.Count > 1<<53-1 || (index > 0 && !result.DailyCounts[index-1].Day.Before(day.Day)) {
			return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
		}
		dailyCount += day.Count
		response.DailyCounts = append(response.DailyCounts, customerActivityDayResponse{Day: day.Day.UTC().Format("2006-01-02"), Count: day.Count})
	}
	if dailyCount != result.TotalEvents {
		return customerActivityAnalyticsResponse{}, contactport.ErrCustomerActivityAnalyticsUnavailable
	}
	return response, nil
}

func validActivityWindow(value int32) bool { return value == 7 || value == 30 || value == 90 }

func customerActivityAnalyticsOwner(ctx context.Context) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomerEventsRead {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID < 1 {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		owner := authorization.OwnerStaffID
		return &owner, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}
func cloneActivityTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
func nilCustomerActivityAnalyticsApplication(value any) bool {
	if value == nil {
		return true
	}
	typed := reflect.ValueOf(value)
	return typed.Kind() == reflect.Pointer && typed.IsNil()
}
