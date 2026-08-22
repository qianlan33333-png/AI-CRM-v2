package operationshttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func TestGetOperationsRequiresQuestionnaireReadAndReturnsLocalProjection(t *testing.T) {
	application := operationsApplicationStub{getFn: func(_ context.Context, id surveyport.ID) (surveyport.OperationsProjection, error) {
		if id != 7 {
			t.Fatalf("questionnaire id=%d", id)
		}
		return operationsProjection(id), nil
	}}
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		request := operationsRequest(t, http.MethodGet, "/api/admin/questionnaires/7/operations", nil, role, authport.CapabilityQuestionnairesRead)
		response := httptest.NewRecorder()
		New(application).GetOperations(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"local_only":true`) || strings.Contains(response.Body.String(), "provider") || strings.Contains(response.Body.String(), "webhook") {
			t.Fatalf("role=%s status/body=%d/%s", role, response.Code, response.Body.String())
		}
	}
	unauthorized := httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/7/operations", nil)
	response := httptest.NewRecorder()
	New(application).GetOperations(response, unauthorized)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"side_effect_executed":false`) {
		t.Fatalf("unauthorized status/body=%d/%s", response.Code, response.Body.String())
	}
	wrongCapability := operationsRequest(t, http.MethodGet, "/api/admin/questionnaires/7/operations", nil, authport.RoleAdmin, authport.CapabilityQuestionnairesWrite)
	response = httptest.NewRecorder()
	New(application).GetOperations(response, wrongCapability)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong capability status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestOperationsMutationStrictInputAndErrorMapping(t *testing.T) {
	called := 0
	application := operationsApplicationStub{saveExternalPushFn: func(_ context.Context, command surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error) {
		called++
		if command.QuestionnaireID != 7 || command.Actor != 41 || command.IdempotencyKey != "survey-operations-http-0001" || !command.ExternalPush.Enabled || command.ExternalPush.ConfigurationReference != "config-7" {
			t.Fatalf("command=%#v", command)
		}
		return operationsProjection(command.QuestionnaireID), nil
	}}
	request := operationsRequest(t, http.MethodPut, "/api/admin/questionnaires/7/operations/external-push", strings.NewReader(`{"enabled":true,"configuration_reference":"config-7"}`), authport.RoleAdmin, authport.CapabilityQuestionnairesWrite)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Idempotency-Key", "survey-operations-http-0001")
	response := httptest.NewRecorder()
	New(application).SaveExternalPush(response, request)
	if response.Code != http.StatusOK || called != 1 || !strings.Contains(response.Body.String(), `"configuration_reference":"config-7"`) {
		t.Fatalf("status/calls/body=%d/%d/%s", response.Code, called, response.Body.String())
	}
	for _, body := range []string{
		`{"enabled":true,"configuration_reference":"https://provider.invalid"}`,
		`{"enabled":true,"enabled":false}`,
		`{"enabled":true,"configuration_reference":"config-7","provider_url":"x"}`,
	} {
		request := operationsRequest(t, http.MethodPut, "/api/admin/questionnaires/7/operations/external-push", strings.NewReader(body), authport.RoleAdmin, authport.CapabilityQuestionnairesWrite)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "survey-operations-http-0002")
		response := httptest.NewRecorder()
		New(application).SaveExternalPush(response, request)
		if response.Code != http.StatusBadRequest || called != 1 || !strings.Contains(response.Body.String(), `"code":"invalid_operations_request"`) {
			t.Fatalf("body=%s status/calls/response=%d/%d/%s", body, response.Code, called, response.Body.String())
		}
	}
	for name, application := range map[string]Application{
		"not found": operationsApplicationStub{saveExternalPushFn: func(context.Context, surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error) {
			return surveyport.OperationsProjection{}, surveyapp.ErrNotFound
		}},
		"conflict": operationsApplicationStub{saveExternalPushFn: func(context.Context, surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error) {
			return surveyport.OperationsProjection{}, surveyapp.ErrConflict
		}},
	} {
		t.Run(name, func(t *testing.T) {
			request := operationsRequest(t, http.MethodPut, "/api/admin/questionnaires/7/operations/external-push", strings.NewReader(`{"enabled":true,"configuration_reference":"config-7"}`), authport.RoleOps, authport.CapabilityQuestionnairesWrite)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "survey-operations-http-0003")
			response := httptest.NewRecorder()
			New(application).SaveExternalPush(response, request)
			want := http.StatusNotFound
			if name == "conflict" {
				want = http.StatusConflict
			}
			if response.Code != want {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestQueueExternalPushTestRejectsUnsafeReadback(t *testing.T) {
	stamp := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	valid := queuedTest(8, 7, stamp)
	application := operationsApplicationStub{queueFn: func(_ context.Context, command surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error) {
		if command.QuestionnaireID != 7 || command.Actor != 41 || command.IdempotencyKey != "survey-operations-queue-http-0001" {
			t.Fatalf("command=%#v", command)
		}
		return valid, nil
	}}
	request := operationsRequest(t, http.MethodPost, "/api/admin/questionnaires/7/operations/external-push/test", nil, authport.RoleOps, authport.CapabilityQuestionnairesWrite)
	request.Header.Set("Idempotency-Key", "survey-operations-queue-http-0001")
	response := httptest.NewRecorder()
	New(application).QueueExternalPushTest(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"status":"queued"`) || !strings.Contains(response.Body.String(), `"auto_retry_allowed":false`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	unsafe := valid
	unsafe.SideEffectExecuted = true
	unsafeApplication := operationsApplicationStub{queueFn: func(context.Context, surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error) {
		return unsafe, nil
	}}
	request = operationsRequest(t, http.MethodPost, "/api/admin/questionnaires/7/operations/external-push/test", nil, authport.RoleAdmin, authport.CapabilityQuestionnairesWrite)
	request.Header.Set("Idempotency-Key", "survey-operations-queue-http-0002")
	response = httptest.NewRecorder()
	New(unsafeApplication).QueueExternalPushTest(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"side_effect_executed":false`) {
		t.Fatalf("unsafe status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestExternalPushLogsAreSafeAndCanonical(t *testing.T) {
	stamp := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	application := operationsApplicationStub{listFn: func(_ context.Context, id *surveyport.ID, limit, offset int32) (surveyport.ExternalPushLogPage, error) {
		if id == nil || *id != 7 || limit != 2 || offset != 1 {
			t.Fatalf("log request id=%v limit=%d offset=%d", id, limit, offset)
		}
		return surveyport.ExternalPushLogPage{Items: []surveyport.ExternalPushTest{queuedTest(8, 7, stamp)}, Total: 2, Limit: limit, Offset: offset, HasMore: false, LocalOnly: true}, nil
	}}
	request := operationsRequest(t, http.MethodGet, "/admin/questionnaires/7/external-push-logs?limit=2&offset=1", nil, authport.RoleAdmin, authport.CapabilityQuestionnairesRead)
	response := httptest.NewRecorder()
	New(application).ListQuestionnaireExternalPushLogs(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "configuration_reference") || strings.Contains(response.Body.String(), "operation_receipt") || strings.Contains(response.Body.String(), "provider_url") {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	for _, target := range []string{
		"/admin/questionnaires/07/external-push-logs",
		"/admin/questionnaires/7/external-push-logs?limit=1&limit=2",
		"/admin/questionnaires/7%2F8/external-push-logs",
	} {
		request := operationsRequest(t, http.MethodGet, target, nil, authport.RoleAdmin, authport.CapabilityQuestionnairesRead)
		response := httptest.NewRecorder()
		New(application).ListQuestionnaireExternalPushLogs(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%s status/body=%d/%s", target, response.Code, response.Body.String())
		}
	}
}

type operationsApplicationStub struct {
	getFn              func(context.Context, surveyport.ID) (surveyport.OperationsProjection, error)
	saveCompletionFn   func(context.Context, surveyport.SaveCompletionOperationsCommand) (surveyport.OperationsProjection, error)
	saveExternalPushFn func(context.Context, surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error)
	queueFn            func(context.Context, surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error)
	listFn             func(context.Context, *surveyport.ID, int32, int32) (surveyport.ExternalPushLogPage, error)
}

func (s operationsApplicationStub) Get(ctx context.Context, id surveyport.ID) (surveyport.OperationsProjection, error) {
	if s.getFn == nil {
		return surveyport.OperationsProjection{}, errors.New("unexpected Get")
	}
	return s.getFn(ctx, id)
}
func (s operationsApplicationStub) SaveCompletion(ctx context.Context, command surveyport.SaveCompletionOperationsCommand) (surveyport.OperationsProjection, error) {
	if s.saveCompletionFn == nil {
		return surveyport.OperationsProjection{}, errors.New("unexpected SaveCompletion")
	}
	return s.saveCompletionFn(ctx, command)
}
func (s operationsApplicationStub) SaveExternalPush(ctx context.Context, command surveyport.SaveExternalPushOperationsCommand) (surveyport.OperationsProjection, error) {
	if s.saveExternalPushFn == nil {
		return surveyport.OperationsProjection{}, errors.New("unexpected SaveExternalPush")
	}
	return s.saveExternalPushFn(ctx, command)
}
func (s operationsApplicationStub) QueueExternalPushTest(ctx context.Context, command surveyport.QueueExternalPushTestCommand) (surveyport.ExternalPushTest, error) {
	if s.queueFn == nil {
		return surveyport.ExternalPushTest{}, errors.New("unexpected QueueExternalPushTest")
	}
	return s.queueFn(ctx, command)
}
func (s operationsApplicationStub) ListExternalPushLogs(ctx context.Context, id *surveyport.ID, limit, offset int32) (surveyport.ExternalPushLogPage, error) {
	if s.listFn == nil {
		return surveyport.ExternalPushLogPage{}, errors.New("unexpected ListExternalPushLogs")
	}
	return s.listFn(ctx, id, limit, offset)
}

func operationsRequest(t *testing.T, method, target string, body *strings.Reader, role authport.Role, capability authport.Capability) *http.Request {
	t.Helper()
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	context := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 41, Role: role}, authport.SessionRef("session"))
	context, err := authport.WithAuthorization(context, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(context)
}

func operationsProjection(id surveyport.ID) surveyport.OperationsProjection {
	return surveyport.OperationsProjection{
		QuestionnaireID: id,
		Completion:      surveyport.CompletionOperations{NavigationTargetID: "target-7", ChannelID: 9},
		ExternalPush:    surveyport.ExternalPushOperations{Enabled: true, ConfigurationReference: "config-7"},
		LocalOnly:       true,
	}
}

func queuedTest(id int64, questionnaireID surveyport.ID, stamp time.Time) surveyport.ExternalPushTest {
	return surveyport.ExternalPushTest{
		TestRunID: id, QuestionnaireID: questionnaireID, Status: surveyapp.ExternalPushTestQueued,
		CreatedAt: stamp, UpdatedAt: stamp,
	}
}
