package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const adminShellRequiredCapability = "admin_read"

var adminShellTemplate = template.Must(template.New("legacy-admin-shell").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>快捷入口</title></head><body><main><h1>快捷入口</h1><nav><a href="/admin/customers">客户管理</a><a href="/admin/cloud-orchestrator/plans">运营计划</a></nav></main><script id="aicrmAdminActionGrants" type="application/json">{}</script></body></html>`))

// adminShellHandler owns the two frozen compatibility entries. It deliberately
// does not share the generic legacy transport because unauthenticated browser
// navigation must keep its historical login redirect rather than becoming a
// JSON API error.
type adminShellHandler struct {
	auth authport.Service
}

func newAdminShellHandler(auth authport.Service) (*adminShellHandler, error) {
	if nilAuth(auth) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	return &adminShellHandler{auth: auth}, nil
}

// Authenticate establishes the browser session, actor, closed capability, and
// account-budget identity before the route endpoint is invoked.
func (handler *adminShellHandler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if handler == nil || nilAuth(handler.auth) || request == nil || next == nil {
			writeAdminShellDependencyError(writer, request)
			return
		}
		// These browser-only compatibility entries never accept a service or
		// bearer principal. Treat even an empty duplicate Authorization field as
		// an invalid principal boundary rather than selecting an arbitrary value.
		if len(request.Header.Values("Authorization")) != 0 {
			writeAdminShellDenied(writer, "principal_type_forbidden")
			return
		}
		session, err := strictBrowserSession(request)
		if err != nil {
			redirectAdminShellLogin(writer, request)
			return
		}
		principal, err := handler.auth.Authenticate(request.Context(), session)
		if err != nil {
			if errors.Is(err, authport.ErrUnauthenticated) {
				redirectAdminShellLogin(writer, request)
				return
			}
			writeAdminShellDependencyError(writer, request)
			return
		}
		if principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			writeAdminShellDenied(writer, "admin_capability_required")
			return
		}
		authorization, err := handler.auth.Authorize(request.Context(), principal, authport.CapabilityAdminShellRead)
		if err != nil {
			if errors.Is(err, authport.ErrUnauthorized) {
				writeAdminShellDenied(writer, "admin_capability_required")
				return
			}
			writeAdminShellDependencyError(writer, request)
			return
		}
		if authorization.Capability != authport.CapabilityAdminShellRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
			writeAdminShellDenied(writer, "admin_capability_required")
			return
		}
		ctx := authport.WithAuthenticatedSession(request.Context(), principal, session)
		ctx, err = authport.WithAuthorization(ctx, authorization)
		if err != nil {
			writeAdminShellDenied(writer, "admin_capability_required")
			return
		}
		ctx, err = platformhttp.ContextWithAccountID(ctx, "admin:"+strconv.FormatInt(principal.AdminUserID, 10))
		if err != nil {
			writeAdminShellDependencyError(writer, request)
			return
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (handler *adminShellHandler) Page(writer http.ResponseWriter, request *http.Request) {
	noStore(writer)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminShellTemplate.Execute(writer, nil)
}

func (handler *adminShellHandler) LogoutAlias(writer http.ResponseWriter, request *http.Request) {
	// The alias never mutates a session. The existing /logout handler owns the
	// paired session/CSRF validation, invalidation, cookie clearing, and final
	// redirect contract.
	noStore(writer)
	http.Redirect(writer, request, legacyLogoutPath, http.StatusFound)
}
func redirectAdminShellLogin(writer http.ResponseWriter, request *http.Request) {
	path := "/admin"
	if request != nil && request.URL != nil && request.URL.Path == "/admin/logout" {
		path = "/admin/logout"
	}
	login := &url.URL{Path: legacyLoginPath}
	login.RawQuery = url.Values{"next": []string{path}}.Encode()
	noStore(writer)
	http.Redirect(writer, request, login.String(), http.StatusFound)
}

func writeAdminShellDenied(writer http.ResponseWriter, code string) {
	noStore(writer)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"ok":                          false,
		"error":                       code,
		"required_capability":         adminShellRequiredCapability,
		"route_owner":                 "ai_crm_next",
		"real_external_call_executed": false,
	})
}

func writeAdminShellDependencyError(writer http.ResponseWriter, request *http.Request) {
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
}
