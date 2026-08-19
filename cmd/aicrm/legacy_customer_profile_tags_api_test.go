package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestLegacyCustomerProfileTagsReadsOnlyLocalTagNames(t *testing.T) {
	detail := &legacyCustomerDetailStub{result: contactapp.CustomerDetailStoreResult{Tags: []contactapp.CustomerTagRecord{
		{ID: 1, Name: " beta "}, {ID: 2, Name: "alpha"}, {ID: 3, Name: "alpha"},
	}}}
	identity := &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	union := &legacyArchiveUnionStub{}
	response := httptest.NewRecorder()
	customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, union).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?external_userid=external-44", legacyToken(1)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"ok\":true,\"tags\":[{\"name\":\"alpha\"},{\"name\":\"beta\"}]}\n" {
		t.Fatalf("body=%s", response.Body.String())
	}
	assertCustomerProfileTagsSecurityHeaders(t, response)
	if detail.calls != 1 || detail.input.ID != 44 || identity.calls != 1 || union.calls != 0 {
		t.Fatalf("detail=%+v identity=%+v union=%+v", detail, identity, union)
	}
	if identity.ref != (identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:corp-0301", Value: "external-44", Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-tags"}) {
		t.Fatalf("identity ref=%+v", identity.ref)
	}
	assertNoCustomerProfileTagsSensitiveFields(t, response.Body.String())
}

func TestLegacyCustomerProfileTagsReturnsAnAuthoritativeEmptyProjection(t *testing.T) {
	detail := &legacyCustomerDetailStub{result: contactapp.CustomerDetailStoreResult{}}
	identity := &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}}
	response := httptest.NewRecorder()
	customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, &legacyArchiveUnionStub{}).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?external_userid=external-44", legacyToken(1)))
	if response.Code != http.StatusOK || response.Body.String() != "{\"ok\":true,\"tags\":[]}\n" || detail.calls != 1 {
		t.Fatalf("status/body/calls=%d/%s/%d", response.Code, response.Body.String(), detail.calls)
	}
	assertCustomerProfileTagsSecurityHeaders(t, response)
}

func TestLegacyCustomerProfileTagsRequiresConsistentFrozenHints(t *testing.T) {
	for _, test := range []struct {
		name              string
		union             identityport.ResolveResult
		external          identityport.ResolveResult
		wantStatus        int
		wantBody          string
		wantReads         int
		wantIdentityCalls int
	}{
		{"same customer", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, http.StatusOK, "", 1, 1},
		{"different customers", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 45}, http.StatusConflict, "identity_hint_conflict", 0, 1},
		{"union conflict", identityport.ResolveResult{Status: identityport.ResolveConflict}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, http.StatusConflict, "identity_hint_conflict", 0, 1},
		{"external conflict", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveConflict}, http.StatusConflict, "identity_hint_conflict", 0, 1},
		{"union not found external found", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, http.StatusConflict, "identity_hint_conflict", 0, 1},
		{"union found external not found", identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 44}, identityport.ResolveResult{Status: identityport.ResolveNotFound}, http.StatusConflict, "identity_hint_conflict", 0, 1},
		{"both not found", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{Status: identityport.ResolveNotFound}, http.StatusConflict, "identity_hint_conflict", 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			detail := &legacyCustomerDetailStub{result: contactapp.CustomerDetailStoreResult{}}
			identity := &legacyIdentityStub{result: test.external}
			union := &legacyArchiveUnionStub{result: test.union}
			response := httptest.NewRecorder()
			customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, union).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?unionid=union-44&external_userid=external-44", legacyToken(2)))
			if response.Code != test.wantStatus || detail.calls != test.wantReads || union.calls != 1 || identity.calls != test.wantIdentityCalls {
				t.Fatalf("status/calls=%d/%d/%d/%d body=%s", response.Code, detail.calls, union.calls, identity.calls, response.Body.String())
			}
			if test.wantBody != "" && !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body=%s want=%q", response.Body.String(), test.wantBody)
			}
			assertCustomerProfileTagsSecurityHeaders(t, response)
		})
	}
}

func TestLegacyCustomerProfileTagsReturnsNotFoundForASingleMissingHint(t *testing.T) {
	for _, test := range []struct {
		name, path string
		identity   identityport.ResolveResult
		union      identityport.ResolveResult
	}{
		{"union", legacyCustomerProfileTagsPath + "?unionid=union-missing", identityport.ResolveResult{}, identityport.ResolveResult{Status: identityport.ResolveNotFound}},
		{"external user", legacyCustomerProfileTagsPath + "?external_userid=external-missing", identityport.ResolveResult{Status: identityport.ResolveNotFound}, identityport.ResolveResult{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, &legacyCustomerDetailStub{}, &legacyIdentityStub{result: test.identity}, &legacyArchiveUnionStub{result: test.union}).ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(2)))
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "customer_not_found") {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
			assertCustomerProfileTagsSecurityHeaders(t, response)
		})
	}
}

func TestLegacyCustomerProfileTagsRejectsUnsafeHintsAndUnauthorizedRequestsBeforeRead(t *testing.T) {
	for _, target := range []struct {
		name, path, code string
	}{
		{"unsafe user id", legacyCustomerProfileTagsPath + "?user_id=4", "unsupported_identity_hint"},
		{"unsafe user id with valid hint", legacyCustomerProfileTagsPath + "?external_userid=external-44&user_id=4", "unsupported_identity_hint"},
		{"missing identity", legacyCustomerProfileTagsPath, "invalid_identity_hint"},
		{"unknown identity", legacyCustomerProfileTagsPath + "?openid=value", "invalid_identity_hint"},
		{"empty identity", legacyCustomerProfileTagsPath + "?unionid=", "invalid_identity_hint"},
		{"duplicate identity", legacyCustomerProfileTagsPath + "?unionid=one&unionid=two", "invalid_identity_hint"},
		{"bad escape", legacyCustomerProfileTagsPath + "?unionid=%ZZ", "invalid_identity_hint"},
	} {
		t.Run(target.name, func(t *testing.T) {
			detail, identity, union := &legacyCustomerDetailStub{}, &legacyIdentityStub{}, &legacyArchiveUnionStub{}
			response := httptest.NewRecorder()
			customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, union).ServeHTTP(response, legacyRequest(http.MethodGet, target.path, legacyToken(3)))
			if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), target.code) || detail.calls != 0 || identity.calls != 0 || union.calls != 0 {
				t.Fatalf("status/calls=%d/%d/%d/%d body=%s", response.Code, detail.calls, identity.calls, union.calls, response.Body.String())
			}
			assertCustomerProfileTagsSecurityHeaders(t, response)
		})
	}

	t.Run("unsafe user id wins before a missing read dependency", func(t *testing.T) {
		response := httptest.NewRecorder()
		customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, nil, nil, nil).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?user_id=4", legacyToken(3)))
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "unsupported_identity_hint") {
			t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
		}
		assertCustomerProfileTagsSecurityHeaders(t, response)
	})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			detail, identity, union := &legacyCustomerDetailStub{}, &legacyIdentityStub{}, &legacyArchiveUnionStub{}
			response := httptest.NewRecorder()
			customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, union).ServeHTTP(response, httptest.NewRequest(method, legacyCustomerProfileTagsPath, nil))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || detail.calls != 0 || identity.calls != 0 || union.calls != 0 {
				t.Fatalf("status/allow/calls=%d/%q/%d/%d/%d", response.Code, response.Header().Get("Allow"), detail.calls, identity.calls, union.calls)
			}
			assertCustomerProfileTagsSecurityHeaders(t, response)
		})
	}

	for _, principal := range []authport.Principal{{AdminUserID: 9, Role: authport.RoleOps}, {AdminUserID: 9, Role: authport.RoleSales}} {
		detail, identity, union := &legacyCustomerDetailStub{}, &legacyIdentityStub{}, &legacyArchiveUnionStub{}
		response := httptest.NewRecorder()
		customerProfileTagsRouter(t, principal, detail, identity, union).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?unionid=union-44", legacyToken(4)))
		if response.Code != http.StatusForbidden || detail.calls != 0 || identity.calls != 0 || union.calls != 0 {
			t.Fatalf("principal=%+v status/calls=%d/%d/%d/%d", principal, response.Code, detail.calls, identity.calls, union.calls)
		}
		assertCustomerProfileTagsSecurityHeaders(t, response)
	}

	detail, identity, union := &legacyCustomerDetailStub{}, &legacyIdentityStub{}, &legacyArchiveUnionStub{}
	response := httptest.NewRecorder()
	customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, detail, identity, union).ServeHTTP(response, httptest.NewRequest(http.MethodGet, legacyCustomerProfileTagsPath+"?unionid=union-44", nil))
	if response.Code != http.StatusUnauthorized || detail.calls != 0 || identity.calls != 0 || union.calls != 0 {
		t.Fatalf("unauthenticated status/calls=%d/%d/%d/%d", response.Code, detail.calls, identity.calls, union.calls)
	}
	assertCustomerProfileTagsSecurityHeaders(t, response)
}

func TestLegacyCustomerProfileTagsFailsClosedForProjectionOrResolverFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		detail   *legacyCustomerDetailStub
		identity *legacyIdentityStub
		union    *legacyArchiveUnionStub
		path     string
	}{
		{"union resolver failure", &legacyCustomerDetailStub{}, &legacyIdentityStub{}, &legacyArchiveUnionStub{err: context.DeadlineExceeded}, legacyCustomerProfileTagsPath + "?unionid=union-44"},
		{"external resolver failure", &legacyCustomerDetailStub{}, &legacyIdentityStub{err: errors.New("resolver failed")}, &legacyArchiveUnionStub{}, legacyCustomerProfileTagsPath + "?external_userid=external-44"},
		{"invalid found result", &legacyCustomerDetailStub{}, &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound}}, &legacyArchiveUnionStub{}, legacyCustomerProfileTagsPath + "?external_userid=external-44"},
		{"projection unavailable", &legacyCustomerDetailStub{err: contactapp.ErrCustomerDetailUnavailable}, &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(44)}}, &legacyArchiveUnionStub{}, legacyCustomerProfileTagsPath + "?external_userid=external-44"},
		{"invalid projected tag", &legacyCustomerDetailStub{result: contactapp.CustomerDetailStoreResult{Tags: []contactapp.CustomerTagRecord{{ID: 1, Name: "   "}}}}, &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(44)}}, &legacyArchiveUnionStub{}, legacyCustomerProfileTagsPath + "?external_userid=external-44"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			customerProfileTagsRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, test.detail, test.identity, test.union).ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(5)))
			if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"ok\":false,\"status_code\":503,\"error_code\":\"customer_profile_tags_unavailable\"}\n" {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
			assertCustomerProfileTagsSecurityHeaders(t, response)
		})
	}
}

func customerProfileTagsRouter(t *testing.T, principal authport.Principal, detail *legacyCustomerDetailStub, identity *legacyIdentityStub, union *legacyArchiveUnionStub) http.Handler {
	t.Helper()
	service := &legacyAuthStub{principal: principal}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.customerDetail = detail
	legacy.identityResolve = identity
	legacy.messageArchiveUnionID = union
	legacy.weComCorpID = "corp-0301"
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func assertNoCustomerProfileTagsSensitiveFields(t *testing.T, body string) {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"identity", "source", "provider", "external", "customer_id", "tag_id", "group"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("forbidden field/value fragment %q in %s", forbidden, body)
		}
	}
	if value == nil {
		t.Fatal("nil response")
	}
}

func assertCustomerProfileTagsSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers cache-control=%q x-content-type-options=%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}
