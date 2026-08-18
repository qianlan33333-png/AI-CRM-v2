package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/readiness"
)

const (
	legacyDataHealthChecksPath  = "/api/admin/data-health/checks"
	legacyDataHealthCheckPath   = "/api/admin/data-health/checks/{check_id}"
	legacyDataHealthSummaryPath = "/api/admin/data-health/summary"
)

type legacyDataHealthObservationSource interface {
	Observe(context.Context) readiness.Input
}

type legacyDataHealthHandler struct {
	source legacyDataHealthObservationSource
	now    func() time.Time
}

func newLegacyDataHealthHandler(source legacyDataHealthObservationSource) *legacyDataHealthHandler {
	return &legacyDataHealthHandler{source: source, now: time.Now}
}

func (handler *legacyDataHealthHandler) List(writer http.ResponseWriter, request *http.Request) {
	aggregate, ok := handler.observe(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": aggregate.OK, "checks": aggregate.Checks, "registry_id": aggregate.RegistryID, "registry_sha256": aggregate.RegistrySHA256, "registry_matches_manifest": aggregate.RegistryMatchesManifest, "excluded_legacy_check_ids": aggregate.ExcludedLegacyCheckIDs, "observed_at": aggregate.ObservedAt, "real_external_call_executed": false})
}

func (handler *legacyDataHealthHandler) Detail(writer http.ResponseWriter, request *http.Request) {
	if !legacyDataHealthAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	id := chi.URLParam(request, "check_id")
	if !legacyDataHealthCheckIDKnown(id) {
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeNotFound)
		writeJSON(writer, http.StatusNotFound, map[string]any{"ok": false, "status_code": http.StatusNotFound, "error_code": "data_health_check_not_found", "check_id": id, "real_external_call_executed": false})
		return
	}
	aggregate, ok := handler.observeAuthorized(writer, request)
	if !ok {
		return
	}
	for _, check := range aggregate.Checks {
		if check.CheckID == id {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": check.GateDecision != "block", "check": check, "registry_id": aggregate.RegistryID, "registry_sha256": aggregate.RegistrySHA256, "registry_matches_manifest": aggregate.RegistryMatchesManifest, "observed_at": aggregate.ObservedAt, "real_external_call_executed": false})
			return
		}
	}
}

func (handler *legacyDataHealthHandler) Summary(writer http.ResponseWriter, request *http.Request) {
	aggregate, ok := handler.observe(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": aggregate.OK, "overall_status": aggregate.OverallStatus, "counts": aggregate.Counts, "checks": aggregate.Checks, "gate_counts": aggregate.GateCounts, "registry_id": aggregate.RegistryID, "registry_sha256": aggregate.RegistrySHA256, "registry_matches_manifest": aggregate.RegistryMatchesManifest, "excluded_legacy_check_ids": aggregate.ExcludedLegacyCheckIDs, "observed_at": aggregate.ObservedAt, "real_external_call_executed": false})
}

func (handler *legacyDataHealthHandler) observe(writer http.ResponseWriter, request *http.Request) (readiness.DataHealthAggregate, bool) {
	if !legacyDataHealthAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return readiness.DataHealthAggregate{}, false
	}
	return handler.observeAuthorized(writer, request)
}

func (handler *legacyDataHealthHandler) observeAuthorized(writer http.ResponseWriter, request *http.Request) (readiness.DataHealthAggregate, bool) {
	if handler == nil || handler.source == nil {
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "status_code": http.StatusServiceUnavailable, "error_code": "data_health_observation_unavailable", "real_external_call_executed": false})
		return readiness.DataHealthAggregate{}, false
	}
	now := time.Now
	if handler.now != nil {
		now = handler.now
	}
	return readiness.EvaluateDataHealth(handler.source.Observe(request.Context()), now().UTC()), true
}

func legacyDataHealthCheckIDKnown(id string) bool {
	for _, known := range []string{"database_readiness", "migration_compatibility", "outbound_outcome_unknown_backlog", "release_sha_complete"} {
		if id == known {
			return true
		}
	}
	return false
}

func legacyDataHealthAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK && authorization.Capability == authport.CapabilityAdminRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}
