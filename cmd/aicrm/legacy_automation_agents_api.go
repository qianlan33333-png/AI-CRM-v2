package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const automationAgentMaxBodyBytes = 256 << 10

const legacyAutomationAgentListPagePath = "/admin/automation-agents"

// The frozen legacy pages are deliberately compatibility shells, not a new UI.
var automationAgentPageTemplate = template.Must(template.New("automation-agent-compat").Parse("<!doctype html><html lang='zh-CN'><meta charset='utf-8'><title>{{.Title}}</title><main data-automation-agent-id='{{.AgentID}}' data-api-url='{{.APIURL}}'><h1>{{.Title}}</h1></main></html>"))

// AutomationAgentListPage is a carrier only. The SPA owns the intentionally
// narrow local summary projection; this endpoint never reads agent content or
// invokes an automation, provider, or publisher.
func (*Handler) AutomationAgentListPage(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if !legacyAutomationAgentListPageAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape(legacyAutomationAgentListPagePath), http.StatusFound)
}

func legacyAutomationAgentListPageAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin &&
		authorizationOK && authorization.Capability == authport.CapabilityConfigOverviewRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacyAutomationAgentListPageSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "private, no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func writeLegacyAutomationAgentListPageMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
func (handler *Handler) AutomationAgentEditPage(writer http.ResponseWriter, request *http.Request) {
	id, err := automationAgentID(request)
	if err != nil {
		writeAutomationAgentError(writer, err)
		return
	}
	renderAutomationAgentPage(writer, "编辑自动化话术", id, "/api/admin/automation-agents/"+strconv.FormatInt(int64(id), 10))
}
func renderAutomationAgentPage(w http.ResponseWriter, title string, id automationport.AgentID, apiURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	automationAgentHeaders(w)
	_ = automationAgentPageTemplate.Execute(w, struct {
		Title   string
		AgentID automationport.AgentID
		APIURL  string
	}{title, id, apiURL})
}

func (handler *Handler) ListAutomationAgents(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	kind := automationport.AutomationType(strings.TrimSpace(r.URL.Query().Get("automation_type")))
	if kind != "" && kind != automationport.AutomationTypeAgent && kind != automationport.AutomationTypeFixedScript {
		writeAutomationAgentError(w, automationapp.ErrInvalidAgent)
		return
	}
	page, err := service.List(r.Context(), kind)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, automationAgentSummary(item))
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "total": page.Total})
}
func (handler *Handler) CreateAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	body, err := decodeAutomationAgentBody(w, r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	if retiredAutomationWebhook(body) {
		writeAutomationAgentFailure(w, http.StatusGone, "webhook_configuration_retired", platformhttp.CodeMalformedRequest)
		return
	}
	if !automationAgentExactBody(body, "agent_name", "agent_code", "automation_type", "status", "role_prompt", "task_prompt", "fixed_content_package", "legacy_configuration") {
		writeAutomationAgentError(w, automationapp.ErrInvalidAgent)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	command, err := automationAgentCreateCommand(body, principal.AdminUserID, key)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Create(r.Context(), command)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) GetAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Get(r.Context(), id)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) PrecheckAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Get(r.Context(), id)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	configurationReady := strings.TrimSpace(item.DraftRolePrompt) != "" || strings.TrimSpace(item.DraftTaskPrompt) != "" || strings.TrimSpace(item.FixedContentPackage.ContentText) != ""
	reasons := []string{"material_unconfigured", "execution_disabled"}
	if !configurationReady {
		reasons = append([]string{"prompt_unconfigured"}, reasons...)
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{
		"ok": true, "agent_id": item.ID, "configuration_ready": configurationReady,
		"materials_configured": false, "execution_enabled": false, "can_activate": false,
		"reasons": reasons, "real_external_call_executed": false,
	})
}
func (handler *Handler) UpdateAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	body, err := decodeAutomationAgentBody(w, r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	if retiredAutomationWebhook(body) {
		writeAutomationAgentFailure(w, http.StatusGone, "webhook_configuration_retired", platformhttp.CodeMalformedRequest)
		return
	}
	if !automationAgentExactBody(body, "agent_name", "automation_type", "status", "role_prompt", "task_prompt", "fixed_content_package", "legacy_configuration") || len(body) == 0 {
		writeAutomationAgentError(w, automationapp.ErrInvalidAgent)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	command, err := automationAgentUpdateCommand(id, body, principal.AdminUserID, key)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Update(r.Context(), command)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) DeleteAutomationAgent(w http.ResponseWriter, r *http.Request) {
	handler.automationAgentStatus(w, r, automationport.AgentStatusArchived, true)
}
func (handler *Handler) ActivateAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	if _, err = service.Get(r.Context(), id); err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentError(w, automationapp.ErrAgentExecutionDisabled)
}
func (handler *Handler) PauseAutomationAgent(w http.ResponseWriter, r *http.Request) {
	handler.automationAgentStatus(w, r, automationport.AgentStatusPaused, false)
}
func (handler *Handler) automationAgentStatus(w http.ResponseWriter, r *http.Request, status automationport.AgentStatus, archived bool) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.SetStatus(r.Context(), automationport.MutationCommand{ID: id, Actor: principal.AdminUserID, IdempotencyKey: key}, status)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	if archived {
		writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": map[string]any{"id": item.ID, "status": item.Status}})
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) CopyAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Copy(r.Context(), automationport.MutationCommand{ID: id, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) PublishAutomationAgent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.Publish(r.Context(), automationport.MutationCommand{ID: id, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}
func (handler *Handler) SaveAutomationAgentFixedContent(w http.ResponseWriter, r *http.Request) {
	service := handler.automationAgentService(w)
	if service == nil {
		return
	}
	id, err := automationAgentID(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	body, err := decodeAutomationAgentBody(w, r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	if !automationAgentExactBody(body, "content_package") {
		writeAutomationAgentError(w, automationapp.ErrInvalidAgent)
		return
	}
	content, _, err := automationAgentContent(body, "content_package")
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	principal, key, err := automationAgentActorAndKey(r)
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	item, err := service.SaveFixedContent(r.Context(), automationport.FixedContentCommand{ID: id, ContentPackage: content, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeAutomationAgentError(w, err)
		return
	}
	writeAutomationAgentJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": automationAgentDetail(item)})
}

func (handler *Handler) automationAgentService(w http.ResponseWriter) automationport.AgentService {
	if handler == nil || handler.automationAgents == nil {
		writeAutomationAgentError(w, automationapp.ErrAgentUnavailable)
		return nil
	}
	return handler.automationAgents
}
func decodeAutomationAgentBody(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, error) {
	if r == nil {
		return nil, automationapp.ErrInvalidAgent
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, automationAgentMaxBodyBytes))
	var body map[string]json.RawMessage
	if err := decoder.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, automationapp.ErrInvalidAgent
	}
	if body == nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, automationapp.ErrInvalidAgent
	}
	return body, nil
}
func retiredAutomationWebhook(body map[string]json.RawMessage) bool {
	_, bound := body["bound_package_key"]
	_, webhook := body["send_webhook_url"]
	return bound || webhook
}
func automationAgentExactBody(body map[string]json.RawMessage, allowed ...string) bool {
	if body == nil {
		return false
	}
	keys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		keys[key] = struct{}{}
	}
	for key := range body {
		if _, ok := keys[key]; !ok {
			return false
		}
	}
	return true
}
func automationAgentCreateCommand(body map[string]json.RawMessage, actor int64, key string) (automationport.CreateCommand, error) {
	name, _, err := automationAgentString(body, "agent_name")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	code, _, err := automationAgentString(body, "agent_code")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	role, _, err := automationAgentString(body, "role_prompt")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	task, _, err := automationAgentString(body, "task_prompt")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	kind := automationport.AutomationTypeAgent
	if value, present, e := automationAgentString(body, "automation_type"); e != nil {
		return automationport.CreateCommand{}, e
	} else if present {
		kind = automationport.AutomationType(value)
	}
	status := automationport.AgentStatusPaused
	if value, present, e := automationAgentString(body, "status"); e != nil {
		return automationport.CreateCommand{}, e
	} else if present {
		status = automationport.AgentStatus(value)
	}
	content, _, err := automationAgentContent(body, "fixed_content_package")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	legacy, _, err := automationAgentObject(body, "legacy_configuration")
	if err != nil {
		return automationport.CreateCommand{}, err
	}
	return automationport.CreateCommand{Agent: automationport.Agent{AgentName: name, AgentCode: code, AutomationType: kind, Status: status, DraftRolePrompt: role, DraftTaskPrompt: task, FixedContentPackage: content, LegacyConfiguration: legacy}, Actor: actor, IdempotencyKey: key}, nil
}
func automationAgentUpdateCommand(id automationport.AgentID, body map[string]json.RawMessage, actor int64, key string) (automationport.UpdateCommand, error) {
	result := automationport.UpdateCommand{ID: id, Actor: actor, IdempotencyKey: key}
	if value, present, err := automationAgentString(body, "agent_name"); err != nil {
		return result, err
	} else if present {
		result.AgentName = &value
	}
	if value, present, err := automationAgentString(body, "automation_type"); err != nil {
		return result, err
	} else if present {
		kind := automationport.AutomationType(value)
		result.AutomationType = &kind
	}
	if value, present, err := automationAgentString(body, "status"); err != nil {
		return result, err
	} else if present {
		status := automationport.AgentStatus(value)
		result.Status = &status
	}
	if value, present, err := automationAgentString(body, "role_prompt"); err != nil {
		return result, err
	} else if present {
		result.RolePrompt = &value
	}
	if value, present, err := automationAgentString(body, "task_prompt"); err != nil {
		return result, err
	} else if present {
		result.TaskPrompt = &value
	}
	if content, present, err := automationAgentContent(body, "fixed_content_package"); err != nil {
		return result, err
	} else if present {
		result.FixedContentPackage = &content
	}
	if legacy, present, err := automationAgentObject(body, "legacy_configuration"); err != nil {
		return result, err
	} else if present {
		result.LegacyConfiguration = &legacy
	}
	return result, nil
}
func automationAgentString(body map[string]json.RawMessage, field string) (string, bool, error) {
	raw, present := body[field]
	if !present {
		return "", false, nil
	}
	if string(raw) == "null" {
		return "", false, automationapp.ErrInvalidAgent
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false, automationapp.ErrInvalidAgent
	}
	return value, true, nil
}
func automationAgentContent(body map[string]json.RawMessage, field string) (automationport.FixedContentPackage, bool, error) {
	raw, present := body[field]
	if !present {
		return automationport.FixedContentPackage{}, false, nil
	}
	if string(raw) == "null" {
		return automationport.FixedContentPackage{}, false, automationapp.ErrInvalidAgent
	}
	var value automationport.FixedContentPackage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return automationport.FixedContentPackage{}, false, automationapp.ErrInvalidAgent
	}
	return value, true, nil
}
func automationAgentObject(body map[string]json.RawMessage, field string) (json.RawMessage, bool, error) {
	raw, present := body[field]
	if !present {
		return nil, false, nil
	}
	if len(raw) < 2 || raw[0] != '{' || !json.Valid(raw) {
		return nil, false, automationapp.ErrInvalidAgent
	}
	return append(json.RawMessage(nil), raw...), true, nil
}
func automationAgentActorAndKey(r *http.Request) (authport.Principal, string, error) {
	if r == nil {
		return authport.Principal{}, "", authport.ErrUnauthorized
	}
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, "", authport.ErrUnauthorized
	}
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || values[0] == "" || values[0] != strings.TrimSpace(values[0]) || len(values[0]) < 16 || len(values[0]) > 128 {
		return authport.Principal{}, "", automationapp.ErrInvalidAgent
	}
	return principal, values[0], nil
}
func automationAgentID(r *http.Request) (automationport.AgentID, error) {
	if r == nil {
		return 0, automationapp.ErrAgentNotFound
	}
	value, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "agent_id")), 10, 64)
	if err != nil || value < 1 {
		return 0, automationapp.ErrAgentNotFound
	}
	return automationport.AgentID(value), nil
}

func automationAgentSummary(item automationport.Agent) map[string]any {
	return map[string]any{"id": item.ID, "automation_type": item.AutomationType, "agent_code": item.AgentCode, "agent_name": item.AgentName, "bound_package_key": "", "bound_package_id": nil, "bound_package_name": "", "fixed_material_summary": automationContentSummary(item.FixedContentPackage), "status": item.Status, "execution_enabled": false, "materials_configured": false, "updated_at": item.UpdatedAt}
}
func automationAgentDetail(item automationport.Agent) map[string]any {
	result := automationAgentSummary(item)
	// The old edit endpoint retains its compatibility label; the new local list
	// summary deliberately has a closed ten-key projection and derives labels
	// in its own UI from automation_type.
	result["automation_type_label"] = automationTypeLabel(item.AutomationType)
	result["draft_role_prompt"] = item.DraftRolePrompt
	result["draft_task_prompt"] = item.DraftTaskPrompt
	result["published_role_prompt"] = item.PublishedRolePrompt
	result["published_task_prompt"] = item.PublishedTaskPrompt
	result["draft_version"] = item.DraftVersion
	result["published_version"] = item.PublishedVersion
	result["has_unpublished_changes"] = item.DraftVersion != item.PublishedVersion || item.DraftRolePrompt != item.PublishedRolePrompt || item.DraftTaskPrompt != item.PublishedTaskPrompt
	result["fixed_content_package"] = automationContentPayload(item.FixedContentPackage)
	result["fixed_content_package_preview"] = map[string]any{"content_text": item.FixedContentPackage.ContentText, "material_summary": automationContentSummary(item.FixedContentPackage), "materials": []any{}}
	result["legacy_configuration"] = item.LegacyConfiguration
	return result
}
func automationContentPayload(content automationport.FixedContentPackage) map[string]any {
	result := map[string]any{"content_text": content.ContentText, "image_library_ids": content.ImageLibraryIDs, "miniprogram_library_ids": content.MiniprogramLibraryIDs, "attachment_library_ids": content.AttachmentLibraryIDs, "group_invite_library_ids": content.GroupInviteLibraryIDs}
	if len(content.DynamicMiniprogramCard) > 0 {
		var dynamic any
		if json.Unmarshal(content.DynamicMiniprogramCard, &dynamic) == nil {
			result["dynamic_miniprogram_card"] = dynamic
		}
	}
	return result
}
func automationContentSummary(content automationport.FixedContentPackage) map[string]int {
	return map[string]int{"image_count": len(content.ImageLibraryIDs), "miniprogram_count": len(content.MiniprogramLibraryIDs), "attachment_count": len(content.AttachmentLibraryIDs), "group_invite_count": len(content.GroupInviteLibraryIDs)}
}
func automationTypeLabel(kind automationport.AutomationType) string {
	if kind == automationport.AutomationTypeFixedScript {
		return "固定话术"
	}
	return "Agent 机器人"
}
func writeAutomationAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, automationapp.ErrInvalidAgent):
		writeAutomationAgentFailure(w, http.StatusBadRequest, "invalid_agent_payload", platformhttp.CodeMalformedRequest)
	case errors.Is(err, automationapp.ErrAgentNotFound):
		writeAutomationAgentFailure(w, http.StatusNotFound, "agent_not_found", platformhttp.CodeNotFound)
	case errors.Is(err, automationapp.ErrAgentConflict):
		writeAutomationAgentFailure(w, http.StatusConflict, "automation_agent_conflict", platformhttp.CodeConflict)
	case errors.Is(err, automationapp.ErrAgentExecutionDisabled):
		writeAutomationAgentFailure(w, http.StatusConflict, "automation_execution_disabled", platformhttp.CodeConflict)
	case errors.Is(err, authport.ErrUnauthenticated):
		writeAutomationAgentFailure(w, http.StatusUnauthorized, "authentication_required", platformhttp.CodeUnauthenticated)
	case errors.Is(err, authport.ErrUnauthorized):
		writeAutomationAgentFailure(w, http.StatusForbidden, "permission_denied", platformhttp.CodeUnauthorized)
	default:
		writeAutomationAgentFailure(w, http.StatusServiceUnavailable, "automation_agent_unavailable", platformhttp.CodeDependencyUnavailable)
	}
}
func writeAutomationAgentFailure(w http.ResponseWriter, status int, code string, compatibilityCode platformhttp.ErrorCode) {
	platformhttp.MarkCompatibilityError(w, compatibilityCode)
	writeAutomationAgentJSON(w, status, map[string]any{"ok": false, "error": code})
}
func automationAgentHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-AICRM-Route-Owner", "ai_crm_next")
	w.Header().Set("X-AICRM-Fallback-Used", "false")
	w.Header().Set("X-AICRM-Real-External-Call-Executed", "false")
}
func writeAutomationAgentJSON(w http.ResponseWriter, status int, payload map[string]any) {
	automationAgentHeaders(w)
	writeJSON(w, status, payload)
}
