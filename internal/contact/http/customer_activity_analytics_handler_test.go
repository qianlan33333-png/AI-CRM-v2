package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type activityAnalyticsApplication struct {
	result contactport.CustomerActivityAnalytics
	err    error
	calls  int
	query  contactport.CustomerActivityAnalyticsQuery
}

func (application *activityAnalyticsApplication) ReadCustomerActivityAnalytics(_ context.Context, query contactport.CustomerActivityAnalyticsQuery) (contactport.CustomerActivityAnalytics, error) {
	application.calls++
	application.query = query
	return application.result, application.err
}

func handlerAnalyticsFact() contactport.CustomerActivityAnalytics {
	last := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return contactport.CustomerActivityAnalytics{CustomerID: 41, WindowDays: 30, From: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC), Through: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC), TotalEvents: 2, ActiveDays: 1, UniqueEventTypes: 1, LastOccurredAt: &last, TypeFacets: []contactport.CustomerActivityTypeCount{{EventType: "customer.updated", Count: 2, LastOccurredAt: last}}, DailyCounts: []contactport.CustomerActivityDayCount{{Day: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), Count: 2}}}
}

func TestCustomerActivityAnalyticsHandlerScopesRolesAndReturnsClosedSafeProjection(t *testing.T) {
	owner := int64(71)
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{name: "admin", principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}},
		{name: "ops", principal: authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}},
		{name: "sales owner", principal: authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &owner}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner}, wantOwner: &owner},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &activityAnalyticsApplication{result: handlerAnalyticsFact()}
			handler, _ := NewCustomerActivityAnalyticsHandler(application)
			window := 30
			request := customerEventRequest(t, &testCase.principal, &testCase.authorization)
			response := httptest.NewRecorder()
			handler.GetCustomerActivityAnalytics(response, request, 41, CustomerActivityAnalyticsParams{WindowDays: &window})
			if response.Code != 200 || application.calls != 1 || application.query.CustomerID != 41 || application.query.WindowDays != 30 || !customerEventEqualInt64Pointer(application.query.OwnerStaffID, testCase.wantOwner) {
				t.Fatalf("status=%d body=%s query=%#v", response.Code, response.Body.String(), application.query)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 15 || body["payload_included"] != false || body["actor_included"] != false || body["identity_included"] != false || body["real_external_call_executed"] != false {
				t.Fatalf("body=%#v", body)
			}
			if strings.Contains(response.Body.String(), "payload") && !strings.Contains(response.Body.String(), "payload_included") {
				t.Fatalf("unsafe body=%s", response.Body.String())
			}
		})
	}
}

func TestCustomerActivityAnalyticsHandlerRejectsInvalidAndUnauthorizedWithoutCallingApp(t *testing.T) {
	application := &activityAnalyticsApplication{result: handlerAnalyticsFact()}
	handler, _ := NewCustomerActivityAnalyticsHandler(application)
	invalid := 8
	admin := authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	response := httptest.NewRecorder()
	handler.GetCustomerActivityAnalytics(response, customerEventRequest(t, &admin, &authorization), 41, CustomerActivityAnalyticsParams{WindowDays: &invalid})
	if response.Code != 400 || application.calls != 0 {
		t.Fatalf("invalid status=%d calls=%d", response.Code, application.calls)
	}
	response = httptest.NewRecorder()
	handler.GetCustomerActivityAnalytics(response, httptest.NewRequest("GET", "/", nil), 41, CustomerActivityAnalyticsParams{})
	if response.Code != 403 || application.calls != 0 {
		t.Fatalf("unauthorized status=%d calls=%d", response.Code, application.calls)
	}
}

func TestCustomerActivityAnalyticsResponseRejectsSemanticDriftAndUnsafeText(t *testing.T) {
	mutations := []func(*contactport.CustomerActivityAnalytics){
		func(value *contactport.CustomerActivityAnalytics) { value.From = value.From.Add(time.Hour) },
		func(value *contactport.CustomerActivityAnalytics) {
			value.TypeFacets[0].EventType = "customer\u0000updated"
		},
		func(value *contactport.CustomerActivityAnalytics) { value.TypeFacets[0].Count = 1 },
		func(value *contactport.CustomerActivityAnalytics) {
			value.DailyCounts[0].Day = value.DailyCounts[0].Day.Add(time.Hour)
		},
		func(value *contactport.CustomerActivityAnalytics) { value.DailyCounts[0].Count = 1 },
		func(value *contactport.CustomerActivityAnalytics) {
			value.LastOccurredAt = func() *time.Time { result := value.Through.Add(time.Second); return &result }()
		},
	}
	for index, mutate := range mutations {
		value := handlerAnalyticsFact()
		value.TypeFacets = append([]contactport.CustomerActivityTypeCount(nil), value.TypeFacets...)
		value.DailyCounts = append([]contactport.CustomerActivityDayCount(nil), value.DailyCounts...)
		mutate(&value)
		if _, err := activityAnalyticsResponse(value); err == nil {
			t.Fatalf("mutation %d accepted: %#v", index, value)
		}
	}
}
