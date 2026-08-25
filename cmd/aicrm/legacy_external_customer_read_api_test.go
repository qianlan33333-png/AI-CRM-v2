package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestLegacyExternalCustomerReadUsesVerifiedExternalReferenceAndMasksOwnerMismatch(t *testing.T) {
	handler, identity, detail, _, _, _, _ := legacyExternalCustomerReadFixture(t)
	request := legacyExternalHumanRequest(http.MethodGet, "/api/customers/external-secret", "", 7)
	response := httptest.NewRecorder()
	handler.GetCustomer(response, request, "external-secret")
	if response.Code != http.StatusOK || identity.calls != 1 || detail.calls != 1 || detail.input.ID != 44 || detail.input.OwnerStaffID == nil || *detail.input.OwnerStaffID != 7 {
		t.Fatalf("status/calls/input=%d/%d/%d/%+v body=%s", response.Code, identity.calls, detail.calls, detail.input, response.Body.String())
	}
	wantRef := identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp-ci01", Value: "external-secret", Assurance: identityport.AssuranceVerified, Source: "legacy-external-customer-read"}
	if identity.ref != wantRef {
		t.Fatalf("identity ref=%+v want=%+v", identity.ref, wantRef)
	}
	if strings.Contains(response.Body.String(), "external-secret") || strings.Contains(response.Body.String(), "union-secret") || strings.Contains(response.Body.String(), "mobile-secret") {
		t.Fatalf("identity leaked: %s", response.Body.String())
	}

	detail.err = contactapp.ErrCustomerNotFound
	response = httptest.NewRecorder()
	handler.GetCustomer(response, request, "external-secret")
	if response.Code != http.StatusNotFound {
		t.Fatalf("owner-masked status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLegacyExternalCustomerReadProjectsOnlyTimelineAndArchiveMetadata(t *testing.T) {
	handler, _, _, events, chats, _, _ := legacyExternalCustomerReadFixture(t)
	events.result = contactapp.CustomerEventResult{Items: []contactapp.CustomerEventRecord{{
		ID: 9, CustomerID: 44, EventType: "customer.updated", Actor: "staff-secret", Payload: []byte(`{"raw":"payload-secret"}`), OccurredAt: legacyExternalReadTime,
	}}}
	request := legacyExternalHumanRequest(http.MethodGet, "/api/customers/external-secret/timeline?limit=20", "", 7)
	response := httptest.NewRecorder()
	handler.GetCustomerTimeline(response, request, "external-secret")
	if response.Code != http.StatusOK || events.calls != 1 || events.input.CustomerID != 44 || events.input.OwnerStaffID == nil || *events.input.OwnerStaffID != 7 || events.input.Limit != 20 {
		t.Fatalf("status/events=%d/%d/%+v body=%s", response.Code, events.calls, events.input, response.Body.String())
	}
	for _, hidden := range []string{"payload-secret", "staff-secret", "external-secret"} {
		if strings.Contains(response.Body.String(), hidden) {
			t.Fatalf("timeline leaked %q: %s", hidden, response.Body.String())
		}
	}

	chats.page = customer360port.CustomerChatActivityPage{CustomerID: 44, Total: 1, Items: []customer360port.CustomerChatActivityEntry{{
		ChatType: "private", MessageType: "text", SentAt: legacyExternalReadTime,
	}}}
	request = legacyExternalHumanRequest(http.MethodGet, "/api/messages/external-secret/recent?limit=20", "", 7)
	response = httptest.NewRecorder()
	handler.GetRecentMessages(response, request, "external-secret")
	if response.Code != http.StatusOK || chats.calls != 1 || chats.input.CustomerID != 44 || chats.input.OwnerStaffID == nil || *chats.input.OwnerStaffID != 7 || chats.input.Limit != 20 {
		t.Fatalf("status/chats=%d/%d/%+v body=%s", response.Code, chats.calls, chats.input, response.Body.String())
	}
	for _, hidden := range []string{"raw-message-body", "sender-secret", "receiver-secret", "provider-message-secret", "external-secret", "participant_identity_included\":true"} {
		if strings.Contains(strings.ToLower(response.Body.String()), hidden) {
			t.Fatalf("message response leaked %q: %s", hidden, response.Body.String())
		}
	}
}

func TestLegacyExternalCustomerReadListUsesCanonicalKeysetFirstPageOnly(t *testing.T) {
	handler, _, _, _, _, _, list := legacyExternalCustomerReadFixture(t)
	list.result = contactapp.CustomerListResult{Items: []contactapp.CustomerRecord{legacyExternalReadCustomer(44, 7)}, Total: 1, Watermark: legacyExternalReadTime}
	request := legacyExternalHumanRequest(http.MethodGet, "/api/customers?keyword=Ada&limit=20&offset=0", "", 7)
	response := httptest.NewRecorder()
	handler.ListCustomers(response, request)
	if response.Code != http.StatusOK || list.calls != 1 || list.input.Keyword != "Ada" || list.input.OwnerStaffID == nil || *list.input.OwnerStaffID != 7 || list.input.Limit != 20 {
		t.Fatalf("status/list=%d/%d/%+v body=%s", response.Code, list.calls, list.input, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ListCustomers(response, legacyExternalHumanRequest(http.MethodGet, "/api/customers?offset=1", "", 7))
	if response.Code != http.StatusBadRequest || list.calls != 1 {
		t.Fatalf("offset status/calls=%d/%d body=%s", response.Code, list.calls, response.Body.String())
	}
}

func TestLegacyExternalIdentityResolveRequiresInjectedServiceAuthentication(t *testing.T) {
	handler, identity, _, _, _, union, _ := legacyExternalCustomerReadFixture(t)
	service := &legacyExternalServiceAuthStub{principal: operationServicePrincipal{ClientID: "ci-client", PrincipalID: "ci-principal"}}
	handler.service = service
	request := httptest.NewRequest(http.MethodGet, "/api/identity/resolve?external_userid=external-secret", nil)
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: "not-used"})
	response := httptest.NewRecorder()
	handler.ResolveIdentity(response, request)
	if response.Code != http.StatusOK || service.calls != 1 || service.purpose != legacyExternalIdentityResolvePurpose || identity.calls != 1 {
		t.Fatalf("status/auth/identity=%d/%d/%d body=%s", response.Code, service.calls, identity.calls, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "external-secret") || !strings.Contains(response.Body.String(), `"customer_id":44`) {
		t.Fatalf("identity response=%s", response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ResolveIdentity(response, httptest.NewRequest(http.MethodGet, "/api/identity/resolve?external_userid=external-secret&unionid=union-secret", nil))
	if response.Code != http.StatusBadRequest || identity.calls != 1 || union.calls != 0 {
		t.Fatalf("ambiguous identity status/calls=%d/%d/%d body=%s", response.Code, identity.calls, union.calls, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ResolveIdentity(response, httptest.NewRequest(http.MethodGet, "/api/identity/resolve?unionid=union-secret", nil))
	if response.Code != http.StatusOK || union.calls != 1 || union.value != "union-secret" {
		t.Fatalf("union status/calls/value=%d/%d/%q body=%s", response.Code, union.calls, union.value, response.Body.String())
	}

	handler.service = nil
	response = httptest.NewRecorder()
	handler.ResolveIdentity(response, legacyExternalHumanRequest(http.MethodGet, "/api/identity/resolve?external_userid=external-secret", "", 7))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing service auth status=%d body=%s", response.Code, response.Body.String())
	}
}

var legacyExternalReadTime = time.Date(2026, time.August, 25, 8, 0, 0, 0, time.UTC)

func legacyExternalCustomerReadFixture(t *testing.T) (*legacyExternalCustomerReadHandler, *legacyExternalIdentityStub, *legacyExternalDetailStub, *legacyExternalEventsStub, *legacyExternalChatsStub, *legacyExternalUnionStub, *legacyExternalListStub) {
	t.Helper()
	identity := &legacyExternalIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	detail := &legacyExternalDetailStub{result: contactapp.CustomerDetailStoreResult{Customer: legacyExternalReadCustomer(44, 7)}}
	events := &legacyExternalEventsStub{}
	chats := &legacyExternalChatsStub{}
	union := &legacyExternalUnionStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	list := &legacyExternalListStub{}
	handler, err := newLegacyExternalCustomerReadHandler(list, detail, events, chats, identity, union, "corp-ci01", nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler, identity, detail, events, chats, union, list
}

func legacyExternalReadCustomer(id contactport.CustomerID, owner int64) contactapp.CustomerRecord {
	return contactapp.CustomerRecord{ID: id, Name: "Ada", OwnerStaffID: &owner, Extra: []byte(`{}`), CreatedAt: legacyExternalReadTime, UpdatedAt: legacyExternalReadTime}
}

func legacyExternalHumanRequest(method, target, _ string, owner int64) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 1, Role: authport.RoleSales, StaffID: &owner}, "unit-session")
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner})
	return request.WithContext(ctx)
}

type legacyExternalListStub struct {
	input  contactapp.CustomerListInput
	result contactapp.CustomerListResult
	err    error
	calls  int
}

func (stub *legacyExternalListStub) List(_ context.Context, input contactapp.CustomerListInput) (contactapp.CustomerListResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

type legacyExternalDetailStub struct {
	input  contactapp.CustomerDetailInput
	result contactapp.CustomerDetailStoreResult
	err    error
	calls  int
}

func (stub *legacyExternalDetailStub) Get(_ context.Context, input contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

type legacyExternalEventsStub struct {
	input  contactapp.CustomerEventInput
	result contactapp.CustomerEventResult
	err    error
	calls  int
}

func (stub *legacyExternalEventsStub) List(_ context.Context, input contactapp.CustomerEventInput) (contactapp.CustomerEventResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

type legacyExternalChatsStub struct {
	input customer360port.CustomerChatActivityQuery
	page  customer360port.CustomerChatActivityPage
	err   error
	calls int
}

func (stub *legacyExternalChatsStub) ListCustomerChatActivity(_ context.Context, input customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	stub.calls++
	stub.input = input
	return stub.page, stub.err
}

type legacyExternalIdentityStub struct {
	ref    identityport.IDRef
	result identityport.ResolveResult
	err    error
	calls  int
}

func (stub *legacyExternalIdentityStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.calls++
	stub.ref = ref
	return stub.result, stub.err
}

type legacyExternalUnionStub struct {
	value  string
	result identityport.ResolveResult
	err    error
	calls  int
}

func (stub *legacyExternalUnionStub) ResolveUnionID(_ context.Context, value string) (identityport.ResolveResult, error) {
	stub.calls++
	stub.value = value
	return stub.result, stub.err
}

type legacyExternalServiceAuthStub struct {
	principal operationServicePrincipal
	err       error
	purpose   string
	calls     int
}

func (stub *legacyExternalServiceAuthStub) AuthenticateOperation(_ context.Context, _ *http.Request, purpose string) (operationServicePrincipal, error) {
	stub.calls++
	stub.purpose = purpose
	return stub.principal, stub.err
}

var _ customerListApplication = (*legacyExternalListStub)(nil)
var _ customerDetailApplication = (*legacyExternalDetailStub)(nil)
var _ legacyExternalCustomerEventReader = (*legacyExternalEventsStub)(nil)
var _ customer360port.CustomerChatActivityReader = (*legacyExternalChatsStub)(nil)
var _ identityResolveApplication = (*legacyExternalIdentityStub)(nil)
var _ legacyExternalVerifiedUnionIDResolver = (*legacyExternalUnionStub)(nil)
var _ operationServiceAuthenticator = (*legacyExternalServiceAuthStub)(nil)

func TestLegacyExternalCustomerReadSourceContractFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy_external_customer_read_contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version            int      `json:"version"`
		HumanSessionRoutes []string `json:"human_session_routes"`
		ServiceRoute       struct {
			Path    string `json:"path"`
			Purpose string `json:"purpose"`
		} `json:"service_route"`
		MessageForbiddenFields []string `json:"message_forbidden_fields"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 || len(fixture.HumanSessionRoutes) != 7 || fixture.ServiceRoute.Path != "/api/identity/resolve" || fixture.ServiceRoute.Purpose != legacyExternalIdentityResolvePurpose ||
		strings.Join(fixture.MessageForbiddenFields, ",") != "content,sender,receiver,provider_message_id" {
		t.Fatalf("fixture=%+v", fixture)
	}
}
