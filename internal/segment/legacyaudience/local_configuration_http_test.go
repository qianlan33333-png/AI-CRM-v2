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
	membersPageSize    int
	bindingInput       PutAutomationBindingInput
	sendersInput       ReplaceSendersInput
	deleteInput        DeleteAutomationBindingInput
	configurationInput PutConfigurationInput
}

func (*localConfigurationHTTPApplication) GetConfiguration(context.Context, int64) (ConfigurationResponse, error) {
	return ConfigurationResponse{Configuration: &ConfigurationVersion{PackageID: 42, Version: 1, TemplateConfig: []byte(`{"title":"local"}`), FilterConfig: []byte(`{}`), CreatedBy: 7, CreatedAt: time.Unix(1, 0).UTC()}, Projection: localProjection()}, nil
}
func (application *localConfigurationHTTPApplication) PutConfiguration(_ context.Context, input PutConfigurationInput) (ConfigurationResponse, error) {
	application.configurationInput = input
	return ConfigurationResponse{Projection: localProjection()}, nil
}
func (*localConfigurationHTTPApplication) ListSendRecords(context.Context, int64) (SendRecordListResponse, error) {
	return SendRecordListResponse{Projection: localProjection()}, nil
}

func (application *localConfigurationHTTPApplication) ListOperationMembers(_ context.Context, pageSize int) (OperationMemberListResponse, error) {
	application.membersPageSize = pageSize
	return OperationMemberListResponse{Scope: OperationMemberScope, Items: []OperationMember{}, PageSize: pageSize, Projection: localProjection()}, nil
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

func TestLocalConfigurationHTTPVersionsOnlyLocalConfigurationAndReadsRedactedRecords(t *testing.T) {
	application := &localConfigurationHTTPApplication{}
	security := &localConfigurationHTTPSecurity{}
	handler := newLocalConfigurationHTTPHandler(t, application, security)

	put := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/template-config", strings.NewReader(`{"expected_version":0,"template_config":{"title":"local"},"filter_config":{"segment":"active"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "configuration-http-key-01")
	handler.ServeHTTP(put, request)
	if put.Code != http.StatusOK || application.configurationInput.ExpectedVersion != 0 || application.configurationInput.PackageID != 42 || application.configurationInput.Actor.AdminUserID != 7 {
		t.Fatalf("configuration response=%d input=%+v", put.Code, application.configurationInput)
	}

	secret := httptest.NewRecorder()
	secretRequest := httptest.NewRequest(http.MethodPut, RoutePrefix+"/packages/42/template-config", strings.NewReader(`{"expected_version":0,"template_config":{"token":"no"},"filter_config":{}}`))
	secretRequest.Header.Set("Content-Type", "application/json")
	secretRequest.Header.Set("Idempotency-Key", "configuration-http-key-02")
	handler.ServeHTTP(secret, secretRequest)
	if secret.Code != http.StatusUnprocessableEntity {
		t.Fatalf("secret configuration response=%d", secret.Code)
	}

	records := httptest.NewRecorder()
	handler.ServeHTTP(records, httptest.NewRequest(http.MethodGet, RoutePrefix+"/packages/42/send-records", nil))
	if records.Code != http.StatusOK || strings.Contains(records.Body.String(), "provider") || strings.Contains(records.Body.String(), "recipient") || strings.Contains(records.Body.String(), "content") {
		t.Fatalf("send-record response=%d body=%s", records.Code, records.Body.String())
	}
}
