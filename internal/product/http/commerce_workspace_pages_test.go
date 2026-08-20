package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestWorkspacePagesCarryElevenApprovedRoutes(t *testing.T) {
	paths := []string{
		AlipayTransactionsPath,
		ServiceProductsPath,
		ServiceProductNewPath,
		ServiceProductsPath + "/service_A-42/edit",
		ServiceProductsPath + "/service_A-42/data",
		WeChatPayProductNewPath,
		WeChatPayProductsPath + "/product_A-42/edit",
		WeChatPayTransactionsPath,
		WeChatPayTransactionsPath + "/order_A-42",
		WeChatShopTransactionsPath,
		WeChatShopTransactionsPath + "/order_A-42",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewWorkspacePages().ServeHTTP(response, authorizedWorkspaceRequest(http.MethodGet, path))
			if response.Code != http.StatusFound || response.Header().Get("Location") == "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertWorkspaceHeaders(t, response)
		})
	}
}

func TestWorkspacePagesFailClosedForIdentityAndScopeDrift(t *testing.T) {
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
	}{
		{name: "missing principal", authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "ops", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "wrong capability", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityProductsRead, Scope: authport.ScopeGlobal}},
		{name: "owner scope", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, ServiceProductsPath, nil)
			ctx := request.Context()
			if test.principal.AdminUserID > 0 {
				ctx = authport.WithAuthenticatedSession(ctx, test.principal, authport.SessionRef("session"))
			}
			if test.authorization.Capability != "" {
				if authorized, err := authport.WithAuthorization(ctx, test.authorization); err == nil {
					ctx = authorized
				}
			}
			response := httptest.NewRecorder()
			NewWorkspacePages().ServeHTTP(response, request.WithContext(ctx))
			if response.Code != http.StatusForbidden || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertWorkspaceError(t, response, "UNAUTHORIZED")
		})
	}
}

func TestWorkspacePagesRejectMethodsBeforeAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		NewWorkspacePages().ServeHTTP(response, httptest.NewRequest(method, WeChatPayTransactionsPath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("method/status/allow=%s/%d/%q", method, response.Code, response.Header().Get("Allow"))
		}
		assertWorkspaceHeaders(t, response)
	}
}

func TestWorkspacePagesRejectUnknownNestedAndUnsafePaths(t *testing.T) {
	for _, path := range []string{
		"/admin/service-period-products/unknown",
		ServiceProductsPath + "/",
		ServiceProductsPath + "/service/nested/edit",
		ServiceProductsPath + "/../edit",
		WeChatPayProductsPath + "/product\\nested/edit",
		WeChatPayTransactionsPath + "/order%0Aheader",
		WeChatPayTransactionsPath + "/order%252Fescape",
		WeChatPayTransactionsPath + "/order%09tab",
		WeChatShopTransactionsPath + "/order/nested",
	} {
		response := httptest.NewRecorder()
		NewWorkspacePages().ServeHTTP(response, authorizedWorkspaceRequest(http.MethodGet, path))
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Fatalf("path/status/location=%q/%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		assertWorkspaceError(t, response, "NOT_FOUND")
	}
}

func TestWorkspacePageTargetDoesNotInventCommerceFacts(t *testing.T) {
	target, matched := workspacePageTarget(WeChatPayTransactionsPath + "/pending-1")
	if target != WeChatPayTransactionsPath+"/pending-1" || !matched {
		t.Fatalf("target/matched=%q/%t", target, matched)
	}
	if !IsWorkspacePagePattern(WeChatPayTransactionPattern) || IsWorkspacePagePattern("/api/admin/refunds") {
		t.Fatal("workspace pattern registry drift")
	}
}

func TestWorkspacePageTargetUsesTheFrozenUnicodeIdentifierLimit(t *testing.T) {
	valid := WeChatPayTransactionsPath + "/" + strings.Repeat("交", maximumCommerceIdentifierLength)
	if target, matched := workspacePageTarget(valid); !matched || target != valid {
		t.Fatalf("valid unicode target/matched=%q/%t", target, matched)
	}
	invalid := WeChatPayTransactionsPath + "/" + strings.Repeat("交", maximumCommerceIdentifierLength+1)
	if target, matched := workspacePageTarget(invalid); matched || target != "" {
		t.Fatalf("long unicode target/matched=%q/%t", target, matched)
	}
}

func authorizedWorkspaceRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func assertWorkspaceHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertWorkspaceError(t *testing.T, response *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("error security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != wantCode {
		t.Fatalf("body/code=%q/%q err=%v", response.Body.String(), body.Code, err)
	}
}
