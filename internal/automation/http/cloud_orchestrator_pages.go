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
	CloudOrchestratorRootPath          = "/admin/cloud-orchestrator"
	CloudOrchestratorPlansPath         = "/admin/cloud-orchestrator/plans"
	CloudOrchestratorPlanDetailPattern = "/admin/cloud-orchestrator/plans/{plan_id}"
	CloudOrchestratorCampaignsPath     = "/admin/cloud-orchestrator/campaigns"
	CloudOrchestratorObservabilityPath = "/admin/cloud-orchestrator/observability"
	legacyAdminPathParameter           = "legacy_admin_path"
)

var (
	errCloudOrchestratorPageNotFound  = errors.New("cloud orchestrator page not found")
	errCloudOrchestratorPageMalformed = errors.New("cloud orchestrator page request is malformed")
)

// CloudOrchestratorPages is a read-only carrier for the approved legacy
// workspaces. It deliberately owns no plan, audience, approval, quality,
// provider, or outbound contract; those contracts cannot be inferred from a
// page route.
type CloudOrchestratorPages struct{}

func NewCloudOrchestratorPages() *CloudOrchestratorPages {
	return &CloudOrchestratorPages{}
}

func (handler *CloudOrchestratorPages) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setCloudOrchestratorPageHeaders(writer)
	if request == nil {
		return
	}

	target, root, matched := cloudOrchestratorPageTarget(request.URL.Path)
	if !matched {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, errCloudOrchestratorPageNotFound))
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !cloudOrchestratorPageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if target == CloudOrchestratorCampaignsPath {
		var valid bool
		target, valid = cloudOrchestratorCampaignsPageTarget(request.URL.RawQuery)
		if !valid {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, errCloudOrchestratorPageMalformed))
			return
		}
	}
	if root {
		http.Redirect(writer, request, CloudOrchestratorPlansPath, http.StatusFound)
		return
	}
	http.Redirect(writer, request, "/?"+legacyAdminPathParameter+"="+url.QueryEscape(target), http.StatusFound)
}

func cloudOrchestratorCampaignsPageTarget(rawQuery string) (string, bool) {
	if rawQuery == "" {
		return CloudOrchestratorCampaignsPath, true
	}
	parameters, err := url.ParseQuery(rawQuery)
	if err != nil || len(parameters) != 2 {
		return "", false
	}
	kinds, ids := parameters["source_kind"], parameters["source_id"]
	if len(kinds) != 1 || len(ids) != 1 {
		return "", false
	}
	kind, sourceID := kinds[0], ids[0]
	switch kind {
	case "customer_selection", "segment_members", "ai_audience_package_members":
	default:
		return "", false
	}
	parsedID, err := strconv.ParseInt(sourceID, 10, 64)
	if err != nil || parsedID < 1 || strconv.FormatInt(parsedID, 10) != sourceID {
		return "", false
	}
	kindFirst := "source_kind=" + kind + "&source_id=" + sourceID
	idFirst := "source_id=" + sourceID + "&source_kind=" + kind
	if rawQuery != kindFirst && rawQuery != idFirst {
		return "", false
	}
	return CloudOrchestratorCampaignsPath + "?" + kindFirst, true
}

func cloudOrchestratorPageTarget(path string) (target string, root bool, matched bool) {
	switch path {
	case CloudOrchestratorRootPath:
		return CloudOrchestratorPlansPath, true, true
	case CloudOrchestratorPlansPath, CloudOrchestratorCampaignsPath, CloudOrchestratorObservabilityPath:
		return path, false, true
	}

	const detailPrefix = CloudOrchestratorPlansPath + "/"
	if !strings.HasPrefix(path, detailPrefix) {
		return "", false, false
	}
	planID := strings.TrimPrefix(path, detailPrefix)
	if planID == "" || planID == "." || planID == ".." || strings.ContainsAny(planID, "/\\\x00\r\n") {
		return "", false, false
	}
	return detailPrefix + planID, false, true
}

func cloudOrchestratorPageAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	allowedRole := principal.Role == authport.RoleAdmin
	requiredCapability := authport.CapabilityAdminRead
	if request.URL.Path == CloudOrchestratorCampaignsPath {
		allowedRole = allowedRole || principal.Role == authport.RoleOps
		requiredCapability = authport.CapabilityOperationsRead
	}
	return principalOK && principal.AdminUserID > 0 && allowedRole &&
		authorizationOK && authorization.Capability == requiredCapability &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func setCloudOrchestratorPageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func CloudOrchestratorPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setCloudOrchestratorPageHeaders(writer)
		if next != nil {
			next.ServeHTTP(writer, request)
		}
	})
}

func WriteCloudOrchestratorPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	setCloudOrchestratorPageHeaders(writer)
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
