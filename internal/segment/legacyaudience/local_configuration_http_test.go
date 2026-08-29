package legacyaudience

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type localConfigurationHTTPApplication struct {
	membersPageSize      int
	syncInput            OperationMemberSyncInput
	bindingInput         PutAutomationBindingInput
	sendersInput         ReplaceSendersInput
	deleteInput          DeleteAutomationBindingInput
	configurationInput   PutConfigurationInput
	previewInput         PreviewConfigurationInput
	materializeInput     MaterializeConfigurationInput
	templatePreviewInput PreviewTemplateInput
	templateSaveInput    SaveTemplateConfigurationInput
}

func (*localConfigurationHTTPApplication) GetConfiguration(context.Context, int64) (ConfigurationResponse, error) {
	return ConfigurationResponse{Configuration: &ConfigurationVersion{PackageID: 42, Version: 1, CreatedBy: 7, CreatedAt: time.Unix(1, 0).UTC()}, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) PutConfiguration(_ context.Context, input PutConfigurationInput) (ConfigurationResponse, error) {
	application.configurationInput = input
	return ConfigurationResponse{Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) PreviewConfiguration(_ context.Context, input PreviewConfigurationInput) (ConfigurationEvaluationResponse, error) {
	application.previewInput = input
	return ConfigurationEvaluationResponse{PackageID: input.PackageID, ConfigurationVersion: input.ConfigurationVersion, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) MaterializeConfiguration(_ context.Context, input MaterializeConfigurationInput) (ConfigurationEvaluationResponse, error) {
	application.materializeInput = input
	return ConfigurationEvaluationResponse{PackageID: input.PackageID, ConfigurationVersion: input.ConfigurationVersion, Materialized: true, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) PreviewTemplate(_ context.Context, input PreviewTemplateInput) (TemplateEvaluationResponse, error) {
	application.templatePreviewInput = input
	return TemplateEvaluationResponse{PackageID: input.PackageID, Selection: input.Selection, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) SaveTemplateConfiguration(_ context.Context, input SaveTemplateConfigurationInput) (TemplateEvaluationResponse, error) {
	application.templateSaveInput = input
	return TemplateEvaluationResponse{PackageID: input.PackageID, Selection: input.Selection, Saved: true, Projection: localProjection()}, nil
}

type groupOpsOperationMemberHTTPApplication struct {
	*localConfigurationHTTPApplication
	listedPageSize, refreshedPageSize int
	refreshActor                      int64
	refreshKey                        string
}

func (application *groupOpsOperationMemberHTTPApplication) ListGroupOpsOperationMembers(_ context.Context, pageSize int) (any, error) {
	application.listedPageSize = pageSize
	return map[string]any{"scope": GroupOpsOperationMemberScope, "items": []any{}}, nil
}

func (application *groupOpsOperationMemberHTTPApplication) RefreshGroupOpsOperationMembers(_ context.Context, actor int64, key string, pageSize int) (any, error) {
	application.refreshActor, application.refreshKey, application.refreshedPageSize = actor, key, pageSize
	return map[string]any{"scope": GroupOpsOperationMemberScope, "items": []any{}}, nil
}

func (application *localConfigurationHTTPApplication) ListOperationMembers(_ context.Context, pageSize int) (OperationMemberListResponse, error) {
	application.membersPageSize = pageSize
	return OperationMemberListResponse{Scope: OperationMemberScope, Items: []OperationMember{}, PageSize: pageSize, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) SyncOperationMembers(_ context.Context, input OperationMemberSyncInput) (OperationMemberListResponse, error) {
	application.syncInput = input
	return OperationMemberListResponse{Scope: OperationMemberScope, Items: []OperationMember{}, PageSize: input.PageSize, Projection: localProjection()}, nil
}
func (*localConfigurationHTTPApplication) GetAutomationBinding(context.Context, int64) (AutomationBindingResponse, error) {
	return AutomationBindingResponse{Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) PutAutomationBinding(_ context.Context, input PutAutomationBindingInput) (AutomationBindingResponse, error) {
	application.bindingInput = input
	return AutomationBindingResponse{Binding: &AutomationBinding{PackageID: input.PackageID, AutomationAgentID: input.AutomationAgentID}, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) DeleteAutomationBinding(_ context.Context, input DeleteAutomationBindingInput) (AutomationBindingDeleteResponse, error) {
	application.deleteInput = input
	return AutomationBindingDeleteResponse{PackageID: input.PackageID, Deleted: false, Projection: localProjection()}, nil
}
func (*localConfigurationHTTPApplication) GetSenders(context.Context, int64) (PackageSendersResponse, error) {
	return PackageSendersResponse{PackageID: 1, Items: []PackageSender{}, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) ReplaceSenders(_ context.Context, input ReplaceSendersInput) (PackageSendersResponse, error) {
	application.sendersInput = input
	return PackageSendersResponse{PackageID: input.PackageID, Items: input.Items, Projection: localProjection()}, nil
}

type localConfigurationHTTPSecurity struct {
	requirements []AccessRequirement
	err          error
}

func (security *localConfigurationHTTPSecurity) Authorize(_ *http.Request, requirement AccessRequirement) (Actor, error) {
	security.requirements = append(security.requirements, requirement)
	if security.err != nil {
		return Actor{}, security.err
	}
	return Actor{AdminUserID: 7}, nil
}

func newLocalConfigurationHTTPHandler(t *testing.T, application *localConfigurationHTTPApplication, security *localConfigurationHTTPSecurity) http.Handler {
	t.Helper()
	handler, err := NewLocalConfigurationHandler(application, security)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewLocalConfigurationRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func TestLocalConfigurationHTTPRejectsUnsupportedOperationMemberScope(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, OperationMembersRoute+"?scope=group_ops", nil))
	if response.Code != http.StatusUnprocessableEntity || application.membersPageSize != 0 || len(security.requirements) != 1 || security.requirements[0].Capability != CapabilitySegmentsRead {
		t.Fatalf("scope response=%d page=%d requirements=%+v", response.Code, application.membersPageSize, security.requirements)
	}
}

func TestLocalConfigurationHTTPDelegatesGroupOpsOperationMemberReadAndExplicitSync(t *testing.T) {
	application := &groupOpsOperationMemberHTTPApplication{localConfigurationHTTPApplication: &localConfigurationHTTPApplication{}}
	security := &localConfigurationHTTPSecurity{}
	handler, err := NewLocalConfigurationHandler(application, security)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewLocalConfigurationRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}

	read := httptest.NewRecorder()
	fragment.ServeHTTP(read, httptest.NewRequest(http.MethodGet, OperationMembersRoute+"?scope=group_ops&page_size=50", nil))
	if read.Code != http.StatusOK || application.listedPageSize != 50 {
		t.Fatalf("read status/page=%d/%d body=%s", read.Code, application.listedPageSize, read.Body.String())
	}

	syncResponse := httptest.NewRecorder()
	syncRequest := httptest.NewRequest(http.MethodPost, OperationMembersSyncRoute, strings.NewReader(`{"scope":"group_ops","page_size":25}`))
	syncRequest.Header.Set("Content-Type", "application/json")
	syncRequest.Header.Set("Idempotency-Key", "group-ops-members-sync-01")
	fragment.ServeHTTP(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK || application.refreshedPageSize != 25 || application.refreshActor != 7 || application.refreshKey != "group-ops-members-sync-01" {
		t.Fatalf("sync status/page/actor/key=%d/%d/%d/%q body=%s", syncResponse.Code, application.refreshedPageSize, application.refreshActor, application.refreshKey, syncResponse.Body.String())
	}
	if requirement := security.requirements[len(security.requirements)-1]; requirement.Capability != CapabilityOperationsManage || !requirement.RequireCSRF {
		t.Fatalf("sync security=%+v", requirement)
	}
}

func TestLocalConfigurationHTTPSyncsAudienceOperationMembers(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, OperationMembersSyncRoute, strings.NewReader(`{"scope":"ai_audience","page_size":25}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "audience-members-sync-01")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.syncInput.PageSize != 25 || application.syncInput.Actor.AdminUserID != 7 || application.syncInput.IdempotencyKey != "audience-members-sync-01" {
		t.Fatalf("sync response=%d input=%+v body=%s", response.Code, application.syncInput, response.Body.String())
	}
	if requirement := security.requirements[len(security.requirements)-1]; requirement.Capability != CapabilityOperationsManage || !requirement.RequireCSRF {
		t.Fatalf("sync security=%+v", requirement)
	}
}

func TestLocalConfigurationHTTPEnforcesClosedMutationDTOAndSecurity(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)

	bad := httptest.NewRecorder()
	badRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/automation-binding", strings.NewReader(`{"automation_agent_id":7,"runtime":true}`))
	badRequest.Header.Set("Content-Type", "application/json")
	badRequest.Header.Set("Idempotency-Key", "binding-http-key-0001")
	handler.ServeHTTP(bad, badRequest)
	if bad.Code != http.StatusBadRequest || application.bindingInput.PackageID != 0 {
		t.Fatalf("unknown field response=%d input=%+v", bad.Code, application.bindingInput)
	}

	good := httptest.NewRecorder()
	goodRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/automation-binding", strings.NewReader(`{"automation_agent_id":7,"expected_version":0}`))
	goodRequest.Header.Set("Content-Type", "application/json")
	goodRequest.Header.Set("Idempotency-Key", "binding-http-key-0002")
	handler.ServeHTTP(good, goodRequest)
	if good.Code != http.StatusOK || application.bindingInput.PackageID != 42 || application.bindingInput.AutomationAgentID != 7 || application.bindingInput.Actor.AdminUserID != 7 || application.bindingInput.IdempotencyKey != "binding-http-key-0002" {
		t.Fatalf("binding response=%d input=%+v", good.Code, application.bindingInput)
	}
	if last := security.requirements[len(security.requirements)-1]; last.Capability != CapabilitySegmentsWrite || !last.RequireCSRF {
		t.Fatalf("write security requirement=%+v", last)
	}
	if good.Header().Get("Cache-Control") != "private, no-store" || good.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing response security headers: %#v", good.Header())
	}
}

func TestLocalConfigurationHTTPRequiresOrderedLocalSenderPayloadAndIdempotency(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)

	invalid := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/senders", strings.NewReader(`{"items":[{"sender_userid":"alpha","sort_order":2,"is_enabled":true}]}`))
	invalidRequest.Header.Set("Content-Type", "application/json")
	invalidRequest.Header.Set("Idempotency-Key", "senders-http-key-0001")
	handler.ServeHTTP(invalid, invalidRequest)
	if invalid.Code != http.StatusUnprocessableEntity || len(application.sendersInput.Items) != 0 {
		t.Fatalf("invalid sender response=%d input=%+v", invalid.Code, application.sendersInput)
	}

	valid := httptest.NewRecorder()
	validRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/senders", strings.NewReader(`{"items":[{"sender_userid":"alpha","sort_order":1,"is_enabled":true}]}`))
	validRequest.Header.Set("Content-Type", "application/json")
	validRequest.Header.Set("Idempotency-Key", "senders-http-key-0002")
	handler.ServeHTTP(valid, validRequest)
	if valid.Code != http.StatusOK || application.sendersInput.PackageID != 42 || len(application.sendersInput.Items) != 1 || application.sendersInput.Items[0].SenderUserID != "alpha" {
		t.Fatalf("valid sender response=%d input=%+v", valid.Code, application.sendersInput)
	}

	deleteResponse := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, RoutePrefix+"/packages/42/automation-binding", strings.NewReader(`{"expected_version":0}`))
	deleteRequest.Header.Set("Content-Type", "application/json")
	deleteRequest.Header.Set("Idempotency-Key", "delete-http-key-00001")
	handler.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK || application.deleteInput.PackageID != 42 || application.deleteInput.IdempotencyKey == "" {
		t.Fatalf("delete response=%d input=%+v", deleteResponse.Code, application.deleteInput)
	}

	security.err = errors.New("security backend unavailable")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, RoutePrefix+"/packages/42/senders", nil))
	if denied.Code != http.StatusServiceUnavailable {
		t.Fatalf("security unavailable response=%d", denied.Code)
	}
}

func TestLocalConfigurationHTTPVersionsPreviewsAndMaterializesTypedConfiguration(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)

	put := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/configuration", strings.NewReader(`{"expected_version":0,"expected_package_version":3}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "configuration-http-key-01")
	handler.ServeHTTP(put, request)
	if put.Code != http.StatusOK || application.configurationInput.ExpectedVersion != 0 || application.configurationInput.ExpectedPackageVersion != 3 ||
		application.configurationInput.PackageID != 42 || application.configurationInput.Actor.AdminUserID != 7 {
		t.Fatalf("configuration response=%d input=%+v", put.Code, application.configurationInput)
	}

	preview := httptest.NewRecorder()
	handler.ServeHTTP(preview, httptest.NewRequest(http.MethodGet,
		RoutePrefix+"/packages/42/configuration-preview?configuration_version=1&evaluated_at=2026-08-25T01%3A02%3A03Z", nil))
	if preview.Code != http.StatusOK || application.previewInput.PackageID != 42 || application.previewInput.ConfigurationVersion != 1 ||
		application.previewInput.EvaluatedAt != time.Date(2026, 8, 25, 1, 2, 3, 0, time.UTC) {
		t.Fatalf("preview response=%d input=%+v", preview.Code, application.previewInput)
	}

	materialized := httptest.NewRecorder()
	materializeRequest := httptest.NewRequest(http.MethodPost, RoutePrefix+"/packages/42/configuration-materialize",
		strings.NewReader(`{"configuration_version":1,"expected_package_version":3}`))
	materializeRequest.Header.Set("Content-Type", "application/json")
	materializeRequest.Header.Set("Idempotency-Key", "configuration-materialize-http-key")
	handler.ServeHTTP(materialized, materializeRequest)
	if materialized.Code != http.StatusOK || application.materializeInput.PackageID != 42 || application.materializeInput.ConfigurationVersion != 1 ||
		application.materializeInput.ExpectedPackageVersion != 3 || application.materializeInput.Actor.AdminUserID != 7 {
		t.Fatalf("materialize response=%d input=%+v", materialized.Code, application.materializeInput)
	}
}

func TestLocalConfigurationHTTPPreviewsAndSavesClosedTemplateSelection(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	handler := newLocalConfigurationHTTPHandler(t, application, &localConfigurationHTTPSecurity{})

	preview := httptest.NewRecorder()
	previewRequest := httptest.NewRequest(http.MethodPost, RoutePrefix+"/packages/42/template-preview",
		strings.NewReader(`{"template_key":"tag_any","template_version":1,"parameters":{"tag_ids":[3,2]},"evaluated_at":"2026-08-29T01:02:03Z"}`))
	previewRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(preview, previewRequest)
	if preview.Code != http.StatusOK || application.templatePreviewInput.PackageID != 42 || application.templatePreviewInput.Selection.Key != TemplateTagAny ||
		application.templatePreviewInput.EvaluatedAt != time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC) {
		t.Fatalf("preview response=%d input=%+v body=%s", preview.Code, application.templatePreviewInput, preview.Body.String())
	}

	saved := httptest.NewRecorder()
	saveRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/template-config",
		strings.NewReader(`{"template_key":"owner_any","template_version":1,"parameters":{"owner_staff_ids":[9]},"expected_package_version":3,"expected_configuration_version":1}`))
	saveRequest.Header.Set("Content-Type", "application/json")
	saveRequest.Header.Set("Idempotency-Key", "template-config-http-key-01")
	handler.ServeHTTP(saved, saveRequest)
	if saved.Code != http.StatusOK || application.templateSaveInput.PackageID != 42 || application.templateSaveInput.ExpectedPackageVersion != 3 ||
		application.templateSaveInput.ExpectedConfigurationVersion != 1 || application.templateSaveInput.Actor.AdminUserID != 7 ||
		application.templateSaveInput.IdempotencyKey != "template-config-http-key-01" {
		t.Fatalf("save response=%d input=%+v body=%s", saved.Code, application.templateSaveInput, saved.Body.String())
	}
}
