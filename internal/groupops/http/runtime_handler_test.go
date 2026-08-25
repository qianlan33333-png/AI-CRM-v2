package groupopshttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestRunDueHTTPAcceptsOnlyEERIntent(t *testing.T) {
	called := 0
	runtime := runtimeApplicationStub{runDue: func(_ context.Context, command groupopsport.RunDueCommand) (groupopsport.RunSummary, error) {
		called++
		if command.PlanID != 7 || command.ActorID != 7 || command.IdempotencyKey != "group-ops-run-due-http-01" {
			t.Fatalf("command=%+v", command)
		}
		return groupopsport.RunSummary{Accepted: 1, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}, nil
	}}
	request := groupOpsRequest(http.MethodPost, PlansPath+"/7/run-due", nil, authport.RoleOps, authport.CapabilityOperationsManage)
	request.Header.Set("Idempotency-Key", "group-ops-run-due-http-01")
	response := httptest.NewRecorder()
	NewWithRuntime(applicationStub{}, runtime, nil).RunDue(response, request)
	if response.Code != http.StatusAccepted || called != 1 || !strings.Contains(response.Body.String(), `"provider_accepted":false`) || !strings.Contains(response.Body.String(), `"delivery_proven":false`) {
		t.Fatalf("status/body/called=%d/%s/%d", response.Code, response.Body.String(), called)
	}
}

func TestBroadcastAndWebhookProtocolsFailClosedThenAcceptWithInjectedAuth(t *testing.T) {
	accepted, webhook := 0, 0
	runtime := runtimeApplicationStub{
		acceptPlan: func(_ context.Context, command groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error) {
			accepted++
			if command.PlanID != 9 || command.AcceptedBy != "service:client-1" || command.Trigger != groupopsport.RunTriggerBroadcast {
				t.Fatalf("broadcast=%+v", command)
			}
			return groupopsport.RunSummary{Accepted: 1, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}, nil
		},
		acceptWebhook: func(_ context.Context, reference, key string) (groupopsport.RunSummary, error) {
			webhook++
			if reference != "hook-1" || key != "group-ops-webhook-http-01" {
				t.Fatalf("webhook reference/key=%q/%q", reference, key)
			}
			return groupopsport.RunSummary{Accepted: 1, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}, nil
		},
	}
	broadcast := func(handler *Handler) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, BroadcastPath, strings.NewReader(`{"plan_id":9}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "group-ops-broadcast-http-01")
		response := httptest.NewRecorder()
		handler.Broadcast(response, request)
		return response
	}
	response := broadcast(NewWithRuntime(applicationStub{}, runtime, nil))
	if response.Code != http.StatusServiceUnavailable || accepted != 0 || !strings.Contains(response.Body.String(), `"code":"protocol_auth_unavailable"`) {
		t.Fatalf("closed status/body=%d/%s", response.Code, response.Body.String())
	}
	authenticator := &protocolAuthenticatorStub{principal: ProtocolPrincipal{ID: "client-1"}}
	response = broadcast(NewWithRuntime(applicationStub{}, runtime, authenticator))
	if response.Code != http.StatusAccepted || accepted != 1 || authenticator.purpose != "group_ops_broadcast" {
		t.Fatalf("broadcast status/body=%d/%s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/automation/group-ops/webhooks/hook-1", strings.NewReader(`{"event":"accepted-only"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "group-ops-webhook-http-01")
	response = httptest.NewRecorder()
	NewWithRuntime(applicationStub{}, runtime, authenticator).Webhook(response, request)
	if response.Code != http.StatusAccepted || webhook != 1 || authenticator.purpose != "group_ops_webhook" || authenticator.resource != "hook-1" || string(authenticator.body) != `{"event":"accepted-only"}` {
		t.Fatalf("webhook status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestGroupRefreshReturnsExplicitProviderDisabled(t *testing.T) {
	runtime := runtimeApplicationStub{refreshGroups: func(context.Context, groupopsport.GroupRefreshCommand) (groupopsport.GroupDirectoryPage, error) {
		return groupopsport.GroupDirectoryPage{}, groupopsapp.ErrProviderDisabled
	}}
	request := groupOpsRequest(http.MethodPost, GroupsSyncPath, strings.NewReader(`{"owner_staff_id":7,"limit":50}`), authport.RoleOps, authport.CapabilityOperationsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "group-ops-groups-sync-01")
	response := httptest.NewRecorder()
	NewWithRuntime(applicationStub{}, runtime, nil).SyncGroups(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"provider_disabled"`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}

type protocolAuthenticatorStub struct {
	principal         ProtocolPrincipal
	err               error
	purpose, resource string
	body              []byte
}

func (stub *protocolAuthenticatorStub) Authenticate(_ context.Context, _ *http.Request, purpose, resource string, body []byte) (ProtocolPrincipal, error) {
	stub.purpose, stub.resource, stub.body = purpose, resource, append([]byte{}, body...)
	return stub.principal, stub.err
}

type runtimeApplicationStub struct {
	runDue        func(context.Context, groupopsport.RunDueCommand) (groupopsport.RunSummary, error)
	acceptPlan    func(context.Context, groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error)
	acceptWebhook func(context.Context, string, string) (groupopsport.RunSummary, error)
	refreshGroups func(context.Context, groupopsport.GroupRefreshCommand) (groupopsport.GroupDirectoryPage, error)
}

func (stub runtimeApplicationStub) PreviewRunDue(context.Context, int64) (groupopsport.RunDuePreview, error) {
	return groupopsport.RunDuePreview{}, errors.New("unexpected")
}
func (stub runtimeApplicationStub) RunDue(ctx context.Context, command groupopsport.RunDueCommand) (groupopsport.RunSummary, error) {
	if stub.runDue == nil {
		return groupopsport.RunSummary{}, errors.New("unexpected")
	}
	return stub.runDue(ctx, command)
}
func (stub runtimeApplicationStub) AcceptPlan(ctx context.Context, command groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error) {
	if stub.acceptPlan == nil {
		return groupopsport.RunSummary{}, errors.New("unexpected")
	}
	return stub.acceptPlan(ctx, command)
}
func (stub runtimeApplicationStub) AcceptWebhook(ctx context.Context, reference, key string) (groupopsport.RunSummary, error) {
	if stub.acceptWebhook == nil {
		return groupopsport.RunSummary{}, errors.New("unexpected")
	}
	return stub.acceptWebhook(ctx, reference, key)
}
func (runtimeApplicationStub) ListExecutions(context.Context, int64, int32, int32) (groupopsport.ExecutionPage, error) {
	return groupopsport.ExecutionPage{}, errors.New("unexpected")
}
func (runtimeApplicationStub) ManualReconcile(context.Context, groupopsport.ManualReconcileCommand) (groupopsport.Execution, error) {
	return groupopsport.Execution{}, errors.New("unexpected")
}
func (runtimeApplicationStub) ListOperationMembers(context.Context, int32) (groupopsport.OperationMemberPage, error) {
	return groupopsport.OperationMemberPage{}, errors.New("unexpected")
}
func (runtimeApplicationStub) RefreshOperationMembers(context.Context, groupopsport.OperationMemberRefreshCommand) (groupopsport.OperationMemberPage, error) {
	return groupopsport.OperationMemberPage{}, errors.New("unexpected")
}
func (runtimeApplicationStub) ListGroups(context.Context, int64, int32, int32) (groupopsport.GroupDirectoryPage, error) {
	return groupopsport.GroupDirectoryPage{}, errors.New("unexpected")
}
func (stub runtimeApplicationStub) RefreshGroups(ctx context.Context, command groupopsport.GroupRefreshCommand) (groupopsport.GroupDirectoryPage, error) {
	if stub.refreshGroups == nil {
		return groupopsport.GroupDirectoryPage{}, errors.New("unexpected")
	}
	return stub.refreshGroups(ctx, command)
}
