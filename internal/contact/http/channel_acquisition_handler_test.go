package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

func TestCH01ChannelAcquisitionUpdateAssigneesStrictBodyAndBoundCSRF(t *testing.T) {
	mutation := &channelAcquisitionMutationStub{result: contactapp.Channel{ID: 7, Assignees: []contactapp.ChannelAssignee{
		{WeComUserID: "staff-1", DisplayName: "成员一", Status: "active", Priority: 1},
		{WeComUserID: "staff-2", DisplayName: "成员二", Status: "active", Priority: 2},
	}}}
	csrf := &channelAcquisitionCSRFStub{}
	handler := mustChannelAcquisitionHandler(t, mutation, &channelAcquisitionPreviewStub{}, csrf)
	request := channelAcquisitionRequest(http.MethodPut, `{"assignment_mode":"multi_staff","assignment_strategy":"ratio","overflow_policy":"least_loaded","assignees":[{"staff_id":"staff-1","ratio_percent":40},{"staff_id":"staff-2","ratio_percent":60}]}`, authport.CapabilityChannelsWrite)
	response := httptest.NewRecorder()
	handler.UpdateAssignees(response, request, "7")
	if response.Code != http.StatusOK || mutation.calls != 1 || mutation.command.Actor != 41 || mutation.command.ChannelID != 7 || mutation.command.IdempotencyKey != "channel-acquisition-key-0001" || csrf.calls != 1 {
		t.Fatalf("status/calls/command/csrf=%d/%d/%+v/%d body=%s", response.Code, mutation.calls, mutation.command, csrf.calls, response.Body.String())
	}
	var patch map[string]json.RawMessage
	if json.Unmarshal(mutation.command.Patch, &patch) != nil || len(patch) != 4 {
		t.Fatalf("patch=%s", mutation.command.Patch)
	}
	if !strings.Contains(string(mutation.command.Patch), `"status":"active"`) || !strings.Contains(string(mutation.command.Patch), `"priority":1`) || !strings.Contains(string(mutation.command.Patch), `"priority":2`) {
		t.Fatalf("assignment defaults missing from patch=%s", mutation.command.Patch)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 5 || result["legacy_projection"] != nil || strings.Contains(response.Body.String(), "owner_staff_id") || !strings.Contains(response.Body.String(), `"local_only":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("response=%s", response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v", response.Header())
	}
}

func TestCH01ChannelAcquisitionRejectsMalformedAssignmentBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing assignees", `{"assignment_strategy":"ratio"}`},
		{"unknown top field", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100}],"surprise":true}`},
		{"duplicate top field", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100}],"assignees":[{"staff_id":"staff-2","ratio_percent":100}]}`},
		{"unknown member field", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100,"display_name":"forbidden"}]}`},
		{"duplicate member field", `{"assignees":[{"staff_id":"staff-1","staff_id":"staff-2","ratio_percent":100}]}`},
		{"zero active", `{"assignees":[{"staff_id":"staff-1","status":"inactive"}]}`},
		{"mixed inactive", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100},{"staff_id":"staff-2","status":"inactive"}]}`},
		{"six active", `{"assignees":[{"staff_id":"1","ratio_percent":20},{"staff_id":"2","ratio_percent":20},{"staff_id":"3","ratio_percent":20},{"staff_id":"4","ratio_percent":20},{"staff_id":"5","ratio_percent":10},{"staff_id":"6","ratio_percent":10}]}`},
		{"ratio total", `{"assignees":[{"staff_id":"staff-1","ratio_percent":99}]}`},
		{"ratio carries cap", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100,"max_scans_24h":3}]}`},
		{"cap needs cap", `{"assignment_strategy":"cap_switch","assignees":[{"staff_id":"staff-1"}]}`},
		{"trailing value", `{"assignees":[{"staff_id":"staff-1","ratio_percent":100}]} {}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mutation := &channelAcquisitionMutationStub{}
			handler := mustChannelAcquisitionHandler(t, mutation, &channelAcquisitionPreviewStub{}, &channelAcquisitionCSRFStub{})
			response := httptest.NewRecorder()
			handler.UpdateAssignees(response, channelAcquisitionRequest(http.MethodPut, testCase.body, authport.CapabilityChannelsWrite), "7")
			if response.Code != http.StatusUnprocessableEntity || mutation.calls != 0 || !strings.Contains(response.Body.String(), `"code":"VALIDATION_FAILED"`) {
				t.Fatalf("status/calls/body=%d/%d/%s", response.Code, mutation.calls, response.Body.String())
			}
		})
	}
}

func TestCH01ChannelAcquisitionPreviewUsesReadCapabilityOnly(t *testing.T) {
	preview := &channelAcquisitionPreviewStub{result: contactapp.ChannelAcquisitionPreview{
		ChannelID: 7, ChannelCode: "course", ChannelName: "公开课",
		Assignees: []contactapp.ChannelAssignee{{WeComUserID: "staff-1", DisplayName: "成员一", Status: "active", Priority: 1}},
		Lifecycle: contactapp.ChannelAcquisitionLifecycle{State: "local_prerequisites_ready", EntrantReady: false, ReadinessBlockers: []string{"provider_asset_unverified"}},
		QRCode:    contactapp.ChannelQRCodePreview{Status: "legacy_untracked", SceneValue: "scene-7", URL: "https://cdn.example.test/channel-7.jpg"},
		Share:     contactapp.ChannelSharePreview{URL: "https://go.example.test/channel-7", CopyText: "https://go.example.test/channel-7"},
	}}
	csrf := &channelAcquisitionCSRFStub{err: errors.New("must not be called")}
	handler := mustChannelAcquisitionHandler(t, &channelAcquisitionMutationStub{}, preview, csrf)
	response := httptest.NewRecorder()
	handler.Preview(response, channelAcquisitionRequest(http.MethodGet, "", authport.CapabilityChannelsRead), "7")
	if response.Code != http.StatusOK || preview.calls != 1 || csrf.calls != 0 || !strings.Contains(response.Body.String(), `"entrant_ready":false`) || !strings.Contains(response.Body.String(), `"provider_asset_unverified"`) || !strings.Contains(response.Body.String(), `"local_only":true`) || !strings.Contains(response.Body.String(), `"real_external_call_executed":false`) {
		t.Fatalf("status/calls/body=%d/%d/%d/%s", response.Code, preview.calls, csrf.calls, response.Body.String())
	}
}

func TestCH01ChannelAcquisitionPreviewRejectsEntrantReadyWithoutProviderReceipt(t *testing.T) {
	preview := &channelAcquisitionPreviewStub{result: contactapp.ChannelAcquisitionPreview{
		ChannelID: 7, ChannelCode: "course", ChannelName: "公开课",
		Assignees: []contactapp.ChannelAssignee{{WeComUserID: "staff-1", DisplayName: "成员一", Status: "active", Priority: 1}},
		Lifecycle: contactapp.ChannelAcquisitionLifecycle{State: "ready", EntrantReady: true},
		QRCode:    contactapp.ChannelQRCodePreview{Status: "legacy_untracked", SceneValue: "scene-7", URL: "https://cdn.example.test/channel-7.jpg"},
	}}
	handler := mustChannelAcquisitionHandler(t, &channelAcquisitionMutationStub{}, preview, &channelAcquisitionCSRFStub{})
	response := httptest.NewRecorder()
	handler.Preview(response, channelAcquisitionRequest(http.MethodGet, "", authport.CapabilityChannelsRead), "7")
	if response.Code != http.StatusServiceUnavailable || preview.calls != 1 || strings.Contains(response.Body.String(), `"entrant_ready":true`) {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, preview.calls, response.Body.String())
	}
}

func TestCH01ChannelAcquisitionPreviewReturnsMissingAssigneeBlocker(t *testing.T) {
	preview := &channelAcquisitionPreviewStub{result: contactapp.ChannelAcquisitionPreview{
		ChannelID: 7, ChannelCode: "course", ChannelName: "公开课",
		Assignees: []contactapp.ChannelAssignee{},
		Lifecycle: contactapp.ChannelAcquisitionLifecycle{State: "draft", EntrantReady: false, ReadinessBlockers: []string{"active_assignee_required", "provider_asset_unverified"}},
		QRCode:    contactapp.ChannelQRCodePreview{Status: "not_generated"},
	}}
	handler := mustChannelAcquisitionHandler(t, &channelAcquisitionMutationStub{}, preview, &channelAcquisitionCSRFStub{})
	response := httptest.NewRecorder()
	handler.Preview(response, channelAcquisitionRequest(http.MethodGet, "", authport.CapabilityChannelsRead), "7")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"assignees":[]`) || !strings.Contains(response.Body.String(), `"active_assignee_required"`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestCH01ChannelAcquisitionMapsAuthorizationCSRFAndDomainErrors(t *testing.T) {
	validBody := `{"assignees":[{"staff_id":"staff-1","ratio_percent":100}]}`
	for _, testCase := range []struct {
		name       string
		capability authport.Capability
		csrf       error
		app        error
		wantStatus int
		wantCalls  int
	}{
		{"wrong capability", authport.CapabilityChannelsRead, nil, nil, http.StatusForbidden, 0},
		{"csrf invalid", authport.CapabilityChannelsWrite, authport.ErrCSRFInvalid, nil, http.StatusForbidden, 0},
		{"channel invalid", authport.CapabilityChannelsWrite, nil, contactapp.ErrInvalidChannel, http.StatusUnprocessableEntity, 1},
		{"channel missing", authport.CapabilityChannelsWrite, nil, contactapp.ErrChannelNotFound, http.StatusNotFound, 1},
		{"receipt conflict", authport.CapabilityChannelsWrite, nil, contactapp.ErrChannelConflict, http.StatusConflict, 1},
		{"store unavailable", authport.CapabilityChannelsWrite, nil, contactapp.ErrChannelUnavailable, http.StatusServiceUnavailable, 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mutation := &channelAcquisitionMutationStub{err: testCase.app, result: contactapp.Channel{ID: 7, Assignees: []contactapp.ChannelAssignee{{WeComUserID: "staff-1", DisplayName: "成员一", Status: "active", Priority: 1}}}}
			handler := mustChannelAcquisitionHandler(t, mutation, &channelAcquisitionPreviewStub{}, &channelAcquisitionCSRFStub{err: testCase.csrf})
			response := httptest.NewRecorder()
			handler.UpdateAssignees(response, channelAcquisitionRequest(http.MethodPut, validBody, testCase.capability), "7")
			if response.Code != testCase.wantStatus || mutation.calls != testCase.wantCalls {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, mutation.calls, response.Body.String())
			}
		})
	}
}

type channelAcquisitionMutationStub struct {
	command contactapp.UpdateChannelCommand
	result  contactapp.Channel
	err     error
	calls   int
}

func (stub *channelAcquisitionMutationStub) UpdateChannel(_ context.Context, command contactapp.UpdateChannelCommand) (contactapp.Channel, error) {
	stub.calls++
	stub.command = command
	return stub.result, stub.err
}

type channelAcquisitionPreviewStub struct {
	result contactapp.ChannelAcquisitionPreview
	err    error
	calls  int
}

func (stub *channelAcquisitionPreviewStub) Preview(_ context.Context, _ int64) (contactapp.ChannelAcquisitionPreview, error) {
	stub.calls++
	return stub.result, stub.err
}

type channelAcquisitionCSRFStub struct {
	err     error
	calls   int
	session authport.SessionRef
	token   authport.CSRFToken
}

func (stub *channelAcquisitionCSRFStub) ValidateCSRF(_ context.Context, session authport.SessionRef, token authport.CSRFToken) error {
	stub.calls++
	stub.session, stub.token = session, token
	return stub.err
}

func channelAcquisitionRequest(method, body string, capability authport.Capability) *http.Request {
	request := httptest.NewRequest(method, "/api/admin/channels/7/acquisition", strings.NewReader(body))
	request.Header.Set("Idempotency-Key", "channel-acquisition-key-0001")
	request.Header.Set("X-CSRF-Token", "channel-acquisition-csrf")
	contextWithSession := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}, "channel-acquisition-session")
	contextWithAuthorization, _ := authport.WithAuthorization(contextWithSession, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	return request.WithContext(contextWithAuthorization)
}

func mustChannelAcquisitionHandler(t *testing.T, mutation channelAcquisitionMutationApplication, preview channelAcquisitionPreviewApplication, csrf channelAcquisitionCSRFValidator) *ChannelAcquisitionHandler {
	t.Helper()
	handler, err := NewChannelAcquisitionHandler(mutation, preview, csrf)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
