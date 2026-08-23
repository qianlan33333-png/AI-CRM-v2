package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type contactPolicyApplicationStub struct {
	getResult   contactapp.ContactPolicy
	setResult   contactapp.ContactPolicy
	clearResult contactapp.ContactPolicy
	err         error
	getCalls    int
	sets        []contactapp.SetContactPolicyCommand
	clears      []contactapp.ClearContactPolicyCommand
}

func (stub *contactPolicyApplicationStub) Get(context.Context, contactport.CustomerID) (contactapp.ContactPolicy, error) {
	stub.getCalls++
	return stub.getResult, stub.err
}

func (stub *contactPolicyApplicationStub) Set(_ context.Context, command contactapp.SetContactPolicyCommand) (contactapp.ContactPolicy, error) {
	stub.sets = append(stub.sets, command)
	return stub.setResult, stub.err
}

func (stub *contactPolicyApplicationStub) Clear(_ context.Context, command contactapp.ClearContactPolicyCommand) (contactapp.ContactPolicy, error) {
	stub.clears = append(stub.clears, command)
	return stub.clearResult, stub.err
}

func TestContactPolicyHandlerRequiresGlobalOperationsCapabilities(t *testing.T) {
	application := &contactPolicyApplicationStub{getResult: eligibleContactPolicy(41)}
	handler, err := NewContactPolicyHandler(application)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	handler.Get(response, authorizedContactPolicyRequest(t, http.MethodGet, "/api/v1/customers/41/contact-policy", "", authport.CapabilityOperationsRead), 41)
	if response.Code != http.StatusOK || application.getCalls != 1 {
		t.Fatalf("authorized GET status/calls=%d/%d body=%s", response.Code, application.getCalls, response.Body.String())
	}

	response = httptest.NewRecorder()
	request := authorizedContactPolicyRequest(t, http.MethodGet, "/api/v1/customers/41/contact-policy", "", authport.CapabilityCustomersRead)
	handler.Get(response, request, 41)
	if response.Code != http.StatusForbidden || application.getCalls != 1 {
		t.Fatalf("wrong capability status/calls=%d/%d body=%s", response.Code, application.getCalls, response.Body.String())
	}
}

func TestContactPolicyHandlerDecodesClosedSetAndClearCommands(t *testing.T) {
	until := time.Date(2026, time.August, 24, 9, 0, 0, 0, time.UTC)
	reason := contactapp.ContactPolicyReasonCompliance
	application := &contactPolicyApplicationStub{setResult: contactapp.ContactPolicy{
		CustomerID: 42, Version: 1, PolicyPresent: true, SuppressionActive: true,
		ReasonCode: &reason, SuppressedUntil: &until, LocalOnly: true,
	}, clearResult: eligibleContactPolicy(42)}
	handler, _ := NewContactPolicyHandler(application)

	request := authorizedContactPolicyRequest(t, http.MethodPut, "/api/v1/customers/42/contact-policy", `{"expected_version":0,"reason_code":"compliance_hold","suppressed_until":"2026-08-24T09:00:00Z"}`, authport.CapabilityOperationsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "contact-policy-http-0001")
	response := httptest.NewRecorder()
	handler.Set(response, request, 42, "contact-policy-http-0001")
	if response.Code != http.StatusOK || len(application.sets) != 1 {
		t.Fatalf("set status/calls=%d/%d body=%s", response.Code, len(application.sets), response.Body.String())
	}
	set := application.sets[0]
	if set.CustomerID != 42 || set.ExpectedVersion != 0 || set.ActorID != 91 || set.ReasonCode != reason || set.SuppressedUntil == nil || !set.SuppressedUntil.Equal(until) {
		t.Fatalf("set command=%#v", set)
	}

	request = authorizedContactPolicyRequest(t, http.MethodDelete, "/api/v1/customers/42/contact-policy", `{"expected_version":1}`, authport.CapabilityOperationsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "contact-policy-http-0002")
	response = httptest.NewRecorder()
	handler.Clear(response, request, 42, "contact-policy-http-0002")
	if response.Code != http.StatusOK || len(application.clears) != 1 || application.clears[0].ExpectedVersion != 1 {
		t.Fatalf("clear status/commands=%d/%#v body=%s", response.Code, application.clears, response.Body.String())
	}
}

func TestContactPolicyHandlerRejectsMalformedBeforeApplication(t *testing.T) {
	application := &contactPolicyApplicationStub{}
	handler, _ := NewContactPolicyHandler(application)
	cases := []string{
		`{"expected_version":0,"expected_version":0,"reason_code":"manual_opt_out"}`,
		`{"expected_version":0,"reason_code":"manual_opt_out","phone":"+8613800000000"}`,
		`{"expected_version":0,"reason_code":"unknown"}`,
	}
	for _, body := range cases {
		request := authorizedContactPolicyRequest(t, http.MethodPut, "/api/v1/customers/42/contact-policy", body, authport.CapabilityOperationsManage)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "contact-policy-http-0003")
		response := httptest.NewRecorder()
		handler.Set(response, request, 42, "contact-policy-http-0003")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
	if len(application.sets) != 0 {
		t.Fatalf("malformed requests reached application: %#v", application.sets)
	}
}

func TestContactPolicyHandlerMapsApplicationErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{contactapp.ErrInvalidContactPolicy, http.StatusBadRequest},
		{contactapp.ErrContactPolicyNotFound, http.StatusNotFound},
		{contactapp.ErrContactPolicyConflict, http.StatusConflict},
		{errors.New("private database detail"), http.StatusServiceUnavailable},
	}
	for _, test := range cases {
		application := &contactPolicyApplicationStub{err: test.err}
		handler, _ := NewContactPolicyHandler(application)
		response := httptest.NewRecorder()
		handler.Get(response, authorizedContactPolicyRequest(t, http.MethodGet, "/api/v1/customers/42/contact-policy", "", authport.CapabilityOperationsRead), 42)
		if response.Code != test.want || strings.Contains(response.Body.String(), "private database detail") {
			t.Fatalf("err=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}

func authorizedContactPolicyRequest(t *testing.T, method, path, body string, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 91, Role: authport.RoleOps}, authport.SessionRef("session"))
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

func eligibleContactPolicy(customerID contactport.CustomerID) contactapp.ContactPolicy {
	return contactapp.ContactPolicy{CustomerID: customerID, Eligible: true, LocalOnly: true}
}
