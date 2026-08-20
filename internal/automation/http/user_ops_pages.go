package automationhttp

import (
	"errors"
	"net/http"
	"net/url"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	UserOpsPath   = "/admin/user-ops"
	UserOpsUIPath = "/admin/user-ops/ui"
)

var errUserOpsPageNotFound = errors.New("user ops page not found")

// UserOpsPages carries administrators to the local review workspace only. It
// owns no customer, identity, do-not-disturb, export, task, provider, outbound,
// or delivery fact and therefore cannot mutate or infer any of those facts.
type UserOpsPages struct{}

func NewUserOpsPages() *UserOpsPages {
	return &UserOpsPages{}
}

func IsUserOpsPagePattern(pattern string) bool {
	return pattern == UserOpsPath || pattern == UserOpsUIPath
}

func (handler *UserOpsPages) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setUserOpsPageHeaders(writer)
	if request == nil {
		return
	}
	if !IsUserOpsPagePattern(request.URL.Path) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, errUserOpsPageNotFound))
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !userOpsPageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?"+legacyAdminPathParameter+"="+url.QueryEscape(request.URL.Path), http.StatusFound)
}

func userOpsPageAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityAdminRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func setUserOpsPageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func UserOpsPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setUserOpsPageHeaders(writer)
		if next != nil {
			next.ServeHTTP(writer, request)
		}
	})
}

func WriteUserOpsPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setUserOpsPageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
