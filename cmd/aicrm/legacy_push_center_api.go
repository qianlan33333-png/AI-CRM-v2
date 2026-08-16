package main

import (
	"context"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
)

type legacyPushCenterApplication interface {
	Read(context.Context, pushcenterapp.Filter) (pushcenterapp.Summary, error)
}

// PushCenterSections serves LEGACY-API-0421. The authorization check is
// intentionally repeated here so an accidental direct wiring cannot turn an
// owner-scoped Outbound read into this global administrator projection.
func (handler *Handler) PushCenterSections(writer http.ResponseWriter, request *http.Request) {
	filter, ok := legacyPushCenterRequest(writer, request)
	if !ok {
		return
	}
	if handler == nil || handler.pushCenter == nil {
		writePushCenterDegraded(writer, filter, pushcenterapp.ErrReadModelUnavailable, false)
		return
	}
	summary, err := handler.pushCenter.Read(request.Context(), filter)
	if err != nil {
		writePushCenterDegraded(writer, filter, err, false)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":                 true,
		"sections":           legacyPushCenterSections(summary),
		"status_definitions": legacyPushCenterStatusDefinitions(),
		"filters":            summary.AppliedFilter.ResponseFilters(),
		"route_owner":        "ai_crm_next",
	})
}

// PushCenterStats serves LEGACY-API-0422. runtime_queue is intentionally an
// empty object until the read-only lane-summary adapter is available; it never
// calls a provider or changes the projection outcome.
func (handler *Handler) PushCenterStats(writer http.ResponseWriter, request *http.Request) {
	filter, ok := legacyPushCenterRequest(writer, request)
	if !ok {
		return
	}
	if handler == nil || handler.pushCenter == nil {
		writePushCenterDegraded(writer, filter, pushcenterapp.ErrReadModelUnavailable, true)
		return
	}
	summary, err := handler.pushCenter.Read(request.Context(), filter)
	if err != nil {
		writePushCenterDegraded(writer, filter, err, true)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":                          true,
		"counts":                      legacyPushCenterCounts(summary),
		"sections":                    legacyPushCenterSections(summary),
		"status_definitions":          legacyPushCenterStatusDefinitions(),
		"filters":                     summary.AppliedFilter.ResponseFilters(),
		"route_owner":                 "ai_crm_next",
		"real_external_call_executed": false,
		"runtime_queue":               map[string]any{},
		"capability_owner":            pushcenterapp.CapabilityOwner,
	})
}

func legacyPushCenterRequest(writer http.ResponseWriter, request *http.Request) (pushcenterapp.Filter, bool) {
	if request == nil || !legacyPushCenterAuthorized(request.Context()) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return pushcenterapp.Filter{}, false
	}
	values := request.URL.Query()
	return pushcenterapp.Filter{
		Section: values.Get("section"), EffectType: values.Get("effect_type"), Status: values.Get("status"),
		BusinessType: values.Get("business_type"), BusinessID: values.Get("business_id"), TargetType: values.Get("target_type"),
		TargetID: values.Get("target_id"), ExternalUserID: values.Get("external_userid"), OwnerUserID: values.Get("owner_userid"),
		TraceID: values.Get("trace_id"), IdempotencyKey: values.Get("idempotency_key"), SourceModule: values.Get("source_module"),
		SourceRoute: values.Get("source_route"), CreatedFrom: values.Get("created_from"), CreatedTo: values.Get("created_to"),
	}.Normalized(), true
}

func legacyPushCenterAuthorized(ctx context.Context) bool {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	return ok && authorization.Capability == authport.CapabilityOperationsRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacyPushCenterSections(summary pushcenterapp.Summary) []map[string]any {
	definitions := pushcenterapp.SectionDefinitions()
	result := make([]map[string]any, len(definitions))
	for index, definition := range definitions {
		result[index] = map[string]any{
			"key": definition.Key, "label": definition.Label, "effect_types": definition.EffectTypes,
			"capability_key": definition.CapabilityKey, "count": summary.BySection[definition.Key],
		}
	}
	return result
}

func legacyPushCenterStatusDefinitions() []map[string]string {
	definitions := pushcenterapp.StatusDefinitions()
	result := make([]map[string]string, len(definitions))
	for index, definition := range definitions {
		result[index] = map[string]string{"key": definition.Key, "label": definition.Label, "definition": definition.Definition}
	}
	return result
}

func legacyPushCenterCounts(summary pushcenterapp.Summary) map[string]any {
	byStatus := clonePushCenterCounts(summary.ByStatus)
	return map[string]any{
		"total":               summary.Total,
		"by_effective_status": clonePushCenterCounts(summary.ByEffectiveStatus),
		"by_status":           byStatus,
		"by_section":          clonePushCenterCounts(summary.BySection),
		"pending":             byStatus["pending"],
		"running":             byStatus["running"],
		"succeeded":           byStatus["succeeded"],
		"sent":                byStatus["sent"] + byStatus["sent_with_shadow_warning"],
		"failed":              byStatus["failed"],
		"shadow_warning":      byStatus["sent_with_shadow_warning"] + byStatus["shadow_failed_not_business_failed"],
	}
}

func clonePushCenterCounts(counts map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(counts))
	for key, count := range counts {
		result[key] = count
	}
	return result
}

func writePushCenterDegraded(writer http.ResponseWriter, filter pushcenterapp.Filter, readErr error, includeRuntimeQueue bool) {
	payload := map[string]any{
		"ok": true, "degraded": true, "error": "", "error_code": "production_read_unavailable",
		"source_status": "production_unavailable", "read_model_status": "unavailable",
		"capability_owner": pushcenterapp.CapabilityOwner,
		"page_error":       "推送中心读模型暂不可用，请稍后重试。",
		"diagnostics": map[string]any{
			"production_data_ready": false, "fixture_mode": false, "allow_fixture_repo_in_prod": false,
			"error_class": pushcenterapp.ReadUnavailableErrorClass(readErr),
		},
		"route_owner": "ai_crm_next", "fallback_used": false, "real_external_call_executed": false,
		"status_code": http.StatusOK, "items": []any{}, "total": 0,
		"counts": map[string]any{
			"total": 0, "by_effective_status": map[string]int64{}, "by_status": map[string]int64{}, "by_section": map[string]int64{},
			"pending": 0, "running": 0, "sent": 0, "failed": 0,
		},
		"status_definitions": legacyPushCenterStatusDefinitions(), "filters": filter.ResponseFilters(),
		"limit": 50, "offset": 0, "sections": []any{},
	}
	if includeRuntimeQueue {
		payload["runtime_queue"] = map[string]any{}
	}
	writeJSON(writer, http.StatusOK, payload)
}
