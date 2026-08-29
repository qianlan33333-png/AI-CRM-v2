package groupopshttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestCreatePlanRequiresOperationsManageAndStrictJSON(t *testing.T) {
	called := 0
	app := applicationStub{create: func(_ context.Context, command groupopsport.CreatePlanCommand) (groupopsport.Detail, error) {
		called++
		if command.Name != "Onboarding" || command.Actor != 7 || command.IdempotencyKey != "group-ops-http-key-01" {
			t.Fatalf("command=%#v", command)
		}
		return httpDetail(), nil
	}}
	for _, body := range []string{`{"name":"Onboarding","provider_url":"https://example.invalid"}`, `{"name":"Onboarding"}{}`} {
		request := groupOpsRequest(http.MethodPost, PlansPath, strings.NewReader(body), authport.RoleOps, authport.CapabilityOperationsManage)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "group-ops-http-key-01")
		response := httptest.NewRecorder()
		New(app).CreatePlan(response, request)
		if response.Code != http.StatusBadRequest || called != 0 || !strings.Contains(response.Body.String(), `"provider_execution_eligible":false`) {
			t.Fatalf("body=%s status/body=%d/%s", body, response.Code, response.Body.String())
		}
	}
	request := groupOpsRequest(http.MethodPost, PlansPath, strings.NewReader(`{"name":"Onboarding"}`), authport.RoleOps, authport.CapabilityOperationsManage)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Idempotency-Key", "group-ops-http-key-01")
	response := httptest.NewRecorder()
	New(app).CreatePlan(response, request)
	if response.Code != http.StatusCreated || called != 1 || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	request = groupOpsRequest(http.MethodPost, PlansPath, strings.NewReader(`{"name":"Onboarding"}`), authport.RoleOps, authport.CapabilityAdminRead)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "group-ops-http-key-02")
	response = httptest.NewRecorder()
	New(app).CreatePlan(response, request)
	if response.Code != http.StatusForbidden || called != 1 {
		t.Fatalf("capability status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestReadAndMethodErrorsStayMachineReadable(t *testing.T) {
	request := groupOpsRequest(http.MethodGet, PlansPath+"/7", nil, authport.RoleAdmin, authport.CapabilityAdminRead)
	response := httptest.NewRecorder()
	New(applicationStub{detail: func(context.Context, int64) (groupopsport.Detail, error) { return httpDetail(), nil }}).GetPlan(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"provider_execution_eligible":false`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, PlansPath, nil)
	response = httptest.NewRecorder()
	New(nil).ListPlans(response, request)
	if response.Code != http.StatusMethodNotAllowed || !strings.Contains(response.Body.String(), `"code":"method_not_allowed"`) || strings.Count(response.Body.String(), "{") != 2 {
		t.Fatalf("method status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestPreviewUsesOnlyPlanIDAndRejectsPayload(t *testing.T) {
	called := 0
	app := applicationStub{preview: func(_ context.Context, id int64) (groupopsport.ContentValidation, error) {
		called++
		if id != 7 {
			t.Fatal(id)
		}
		return groupopsport.ContentValidation{Valid: true, IssueCodes: []string{}, PreviewLines: []string{}, Safety: groupopsport.LocalSafety()}, nil
	}}
	request := groupOpsRequest(http.MethodPost, PlansPath+"/7/content/preview", strings.NewReader(`{"url":"https://example.invalid"}`), authport.RoleAdmin, authport.CapabilityAdminRead)
	response := httptest.NewRecorder()
	New(app).Preview(response, request)
	if response.Code != http.StatusBadRequest || called != 0 {
		t.Fatalf("payload status/body=%d/%s", response.Code, response.Body.String())
	}
	request = groupOpsRequest(http.MethodPost, PlansPath+"/7/content/preview", nil, authport.RoleAdmin, authport.CapabilityAdminRead)
	response = httptest.NewRecorder()
	New(app).Preview(response, request)
	if response.Code != http.StatusOK || called != 1 || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("preview status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestGetWebhookDescriptorReturnsPublicSigningContractWithoutCredential(t *testing.T) {
	called := 0
	app := applicationStub{webhookDescriptor: func(_ context.Context, id int64) (groupopsport.WebhookDescriptor, error) {
		called++
		if id != 7 {
			t.Fatalf("plan ID=%d", id)
		}
		return groupopsport.WebhookDescriptor{
			Configured: true, Reference: "local-webhook-7",
			Path: "/api/automation/group-ops/webhooks/local-webhook-7", URL: "/api/automation/group-ops/webhooks/local-webhook-7",
			SignatureAlgorithm: groupopsport.WebhookSignatureAlgorithm, SignatureHeader: groupopsport.WebhookSignatureHeader,
			TimestampHeader: groupopsport.WebhookTimestampHeader, NonceHeader: groupopsport.WebhookNonceHeader,
			ClientIDHeader: groupopsport.WebhookClientIDHeader, ClientID: groupopsport.WebhookClientID,
			Description: "same-origin webhook endpoint; signing credentials are withheld",
		}, nil
	}}
	request := groupOpsRequest(http.MethodGet, PlansPath+"/7/webhook-descriptor", nil, authport.RoleAdmin, authport.CapabilityAdminRead)
	response := httptest.NewRecorder()
	New(app).GetWebhookDescriptor(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || called != 1 || !strings.Contains(body, `"configured":true`) ||
		!strings.Contains(body, `"url":"/api/automation/group-ops/webhooks/local-webhook-7"`) ||
		!strings.Contains(body, `"signature_algorithm":"HMAC-SHA256"`) ||
		!strings.Contains(body, `"signature_header":"X-AICRM-Signature"`) ||
		!strings.Contains(body, `"timestamp_header":"X-AICRM-Timestamp"`) ||
		!strings.Contains(body, `"nonce_header":"X-AICRM-Event-Id"`) ||
		!strings.Contains(body, `"client_id":"aicrm-webhook-group-ops"`) ||
		!strings.Contains(body, `"provider_execution_eligible":false`) || !strings.Contains(body, `"real_external_call_executed":false`) ||
		strings.Contains(body, `"secret"`) || strings.Contains(body, `"token"`) || strings.Contains(body, `"receipt"`) {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, called, body)
	}
}

type applicationStub struct {
	create            func(context.Context, groupopsport.CreatePlanCommand) (groupopsport.Detail, error)
	detail            func(context.Context, int64) (groupopsport.Detail, error)
	preview           func(context.Context, int64) (groupopsport.ContentValidation, error)
	webhookDescriptor func(context.Context, int64) (groupopsport.WebhookDescriptor, error)
}

func (s applicationStub) List(context.Context, int32, int32) (groupopsport.PlanPage, error) {
	return groupopsport.PlanPage{}, errors.New("unexpected")
}
func (s applicationStub) Detail(ctx context.Context, id int64) (groupopsport.Detail, error) {
	if s.detail == nil {
		return groupopsport.Detail{}, errors.New("unexpected")
	}
	return s.detail(ctx, id)
}
func (s applicationStub) Create(ctx context.Context, c groupopsport.CreatePlanCommand) (groupopsport.Detail, error) {
	if s.create == nil {
		return groupopsport.Detail{}, errors.New("unexpected")
	}
	return s.create(ctx, c)
}
func (s applicationStub) Update(context.Context, groupopsport.UpdatePlanCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) Activate(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) Pause(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) Archive(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) ListMembers(context.Context, int64, int32, int32) (groupopsport.MemberPage, error) {
	return groupopsport.MemberPage{}, errors.New("unexpected")
}
func (s applicationStub) AddMember(context.Context, groupopsport.MemberCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) RemoveMember(context.Context, groupopsport.MemberCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) ListGroupAssets(context.Context, int64, int32, int32) (groupopsport.GroupAssetPage, error) {
	return groupopsport.GroupAssetPage{}, errors.New("unexpected")
}
func (s applicationStub) AddGroupAsset(context.Context, groupopsport.GroupAssetCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) RemoveGroupAsset(context.Context, groupopsport.GroupAssetCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) ListNodes(context.Context, int64, int32, int32) (groupopsport.NodePage, error) {
	return groupopsport.NodePage{}, errors.New("unexpected")
}
func (s applicationStub) AddNode(context.Context, groupopsport.NodeCreateCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) UpdateNode(context.Context, groupopsport.NodeUpdateCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) RemoveNode(context.Context, groupopsport.NodeDeleteCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) GetWebhookDescriptor(ctx context.Context, id int64) (groupopsport.WebhookDescriptor, error) {
	if s.webhookDescriptor == nil {
		return groupopsport.WebhookDescriptor{}, errors.New("unexpected")
	}
	return s.webhookDescriptor(ctx, id)
}
func (s applicationStub) PutWebhookDescriptor(context.Context, groupopsport.WebhookDescriptorCommand) (groupopsport.Detail, error) {
	return groupopsport.Detail{}, errors.New("unexpected")
}
func (s applicationStub) Preview(ctx context.Context, id int64) (groupopsport.ContentValidation, error) {
	if s.preview == nil {
		return groupopsport.ContentValidation{}, errors.New("unexpected")
	}
	return s.preview(ctx, id)
}

func groupOpsRequest(method, target string, body *strings.Reader, role authport.Role, capability authport.Capability) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: role}, authport.SessionRef("session"))
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	return request.WithContext(ctx)
}
func httpDetail() groupopsport.Detail {
	now := time.Date(2026, time.August, 23, 8, 0, 0, 0, time.UTC)
	return groupopsport.Detail{Plan: groupopsport.Plan{ID: 7, Name: "Onboarding", Status: groupopsport.PlanDraft, Revision: 1, CreatedBy: 7, UpdatedBy: 7, CreatedAt: now, UpdatedAt: now}, Members: []groupopsport.Member{}, GroupAssets: []groupopsport.GroupAsset{}, Nodes: []groupopsport.Node{}, WebhookDescriptor: groupopsport.WebhookDescriptor{Description: "not configured"}, Safety: groupopsport.LocalSafety()}
}
