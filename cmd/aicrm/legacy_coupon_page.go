package main

import (
	"net/http"
	"net/url"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyCouponPagePath = "/admin/coupons"

// CouponListPage is a carrier only. The browser shell owns the local list,
// filters, and copy interaction; the API owns the separately authorized write.
func (*Handler) CouponListPage(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if !legacyCouponPageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape(legacyCouponPagePath), http.StatusFound)
}

func legacyCouponPageAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 &&
		(principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) &&
		authorizationOK && authorization.Capability == authport.CapabilityCouponsRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacyCouponPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "private, no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func writeLegacyCouponPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
