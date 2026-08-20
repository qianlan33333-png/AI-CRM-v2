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
	GroupOpsPlansPath         = "/admin/automation-conversion/group-ops/ui"
	GroupOpsPlanDetailPattern = "/admin/automation-conversion/group-ops/plans/{plan_id}"
)

var errGroupOpsPageNotFound = errors.New("group ops page not found")

// GroupOpsPages is a read-only carrier for the approved group-operations
// workspaces. It does not own plan, member, group, node, webhook, runtime,
// provider, or outbound state and therefore cannot infer any of those facts.
type GroupOpsPages struct{}

func NewGroupOpsPages() *GroupOpsPages {
	return &GroupOpsPages{}
}

func IsGroupOpsPagePattern(pattern string) bool {
	return pattern == GroupOpsPlansPath || pattern == GroupOpsPlanDetailPattern
}

func (handler *GroupOpsPages) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setGroupOpsPageHeaders(writer)
	if request == nil {
		return
	}

	target, matched := groupOpsPageTarget(request.URL.Path)
	if !matched {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, errGroupOpsPageNotFound))
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !groupOpsPageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?"+legacyAdminPathParameter+"="+url.QueryEscape(target), http.StatusFound)
}

func groupOpsPageTarget(path string) (string, bool) {
	switch path {
	case GroupOpsPlansPath:
		return path, true
	}

	const detailPrefix = "/admin/automation-conversion/group-ops/plans/"
	if !strings.HasPrefix(path, detailPrefix) {
		return "", false
	}
	planID := strings.TrimPrefix(path, detailPrefix)
	if planID == "" || strings.ContainsAny(planID, "/\\\x00\r\n") {
		return "", false
	}
	parsed, err := strconv.ParseInt(planID, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != planID {
		return "", false
	}
	return detailPrefix + planID, true
}

func groupOpsPageAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityAdminRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func setGroupOpsPageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func GroupOpsPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setGroupOpsPageHeaders(writer)
		if next != nil {
			next.ServeHTTP(writer, request)
		}
	})
}

func WriteGroupOpsPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setGroupOpsPageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
