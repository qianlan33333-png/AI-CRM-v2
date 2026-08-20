package automationhttp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	AudiencePackagesPath         = "/admin/automation-conversion"
	AudiencePackageDetailPattern = "/admin/automation-conversion/packages/{package_id}"
)

var errAudiencePackagePageNotFound = errors.New("audience package page not found")

// AudiencePackagePages carries administrators only to the approved local
// audience-package workspaces. It deliberately owns no package, member,
// sender, automation binding, send record, provider, or outbound fact.
type AudiencePackagePages struct{}

func NewAudiencePackagePages() *AudiencePackagePages {
	return &AudiencePackagePages{}
}

func IsAudiencePackagePagePattern(pattern string) bool {
	return pattern == AudiencePackagesPath || pattern == AudiencePackageDetailPattern
}

func (handler *AudiencePackagePages) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setAudiencePackagePageHeaders(writer)
	if request == nil {
		return
	}

	target, matched := audiencePackagePageTarget(request.URL.Path)
	if !matched {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, errAudiencePackagePageNotFound))
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !audiencePackagePageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?"+legacyAdminPathParameter+"="+url.QueryEscape(target), http.StatusFound)
}

func audiencePackagePageTarget(path string) (string, bool) {
	if path == AudiencePackagesPath {
		return path, true
	}

	const detailPrefix = "/admin/automation-conversion/packages/"
	if !strings.HasPrefix(path, detailPrefix) {
		return "", false
	}
	packageID := strings.TrimPrefix(path, detailPrefix)
	if packageID == "" || strings.ContainsAny(packageID, "/\\\x00\r\n") {
		return "", false
	}
	parsed, err := strconv.ParseInt(packageID, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != packageID {
		return "", false
	}
	return detailPrefix + packageID, true
}

func audiencePackagePageAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityAdminRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func setAudiencePackagePageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func AudiencePackagePageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setAudiencePackagePageHeaders(writer)
		if next != nil {
			next.ServeHTTP(writer, request)
		}
	})
}

func WriteAudiencePackagePageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setAudiencePackagePageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
