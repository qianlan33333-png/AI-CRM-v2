package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestLegacyOutboundRoutesCallRealApplicationContracts(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	owner := int64(42)
	task := outboundapp.TaskReadModel{
		TaskID: 71, CustomerID: 81, OwnerStaffID: &owner, BatchID: legacyOutboundInt64(91),
		BatchChunkIndex: legacyOutboundInt32(0), Status: outboundapp.TaskStatusCancelled,
		Job: outboundapp.TaskJob{TaskID: 71, Generation: 1, RiverJobID: 101,
			JobKind: outboundapp.OutboundEnqueueBatchJobKind},
		CreatedAt: now.Add(-time.Hour), StatusUpdatedAt: now,
	}
	query := &legacyOutboundQueryStub{task: task, list: outboundapp.TaskListResult{Items: []outboundapp.TaskReadModel{task}}}
	query.reconciliation = outboundapp.TaskReconciliation{Task: task,
		ControlReceipts: []outboundapp.ControlReceiptReadModel{{
			ID: 121, TaskID: 71, Operation: "cancel", State: outboundapp.ControlReceiptCompleted,
			Job: task.Job, EventID: 122, TaskStatus: outboundapp.TaskStatusCancelled,
			CreatedAt: now.Add(-time.Minute), CompletedAt: now,
		}},
	}
	cancel := &legacyCancelStub{result: outboundapp.CancelledTask{ReceiptID: 121, TaskID: 71,
		CustomerID: 81, Status: outboundapp.TaskStatusCancelled, Job: task.Job, EventID: 122, CancelledAt: now}}
	retry := &legacyRetryStub{result: outboundapp.ManualRetryResult{ReceiptID: 131, TaskID: 71,
		CustomerID: 81, Status: outboundapp.TaskStatusPending,
		Job:     outboundapp.TaskJob{TaskID: 71, Generation: 2, RiverJobID: 102, JobKind: outboundapp.OutboundEnqueueBatchJobKind},
		EventID: 132, RetriedAt: now}}
	router := legacyOutboundRouter(t, &legacyAuthStub{}, query, cancel, retry)

	tests := []struct {
		name, method, target string
		csrf                 bool
		wantStatus           int
		wantKey              string
	}{
		{"batch list", http.MethodGet, "/api/admin/push-center/jobs?business_id=91&status=cancelled&limit=20&offset=0", false, http.StatusOK, "jobs"},
		{"detail", http.MethodGet, "/api/admin/push-center/jobs/71", false, http.StatusOK, "job"},
		{"reconciliation", http.MethodGet, "/api/admin/push-center/jobs/71/reconciliation", false, http.StatusOK, "control_receipts"},
		{"cancel replay", http.MethodPost, "/api/admin/push-center/jobs/71/cancel", true, http.StatusAccepted, "control_receipt"},
		{"manual retry", http.MethodPost, "/api/admin/push-center/jobs/71/retry", true, http.StatusAccepted, "control_receipt"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := legacyRequest(testCase.method, testCase.target, legacyToken(11))
			if testCase.csrf {
				request.Header.Set("X-CSRF-Token", legacyToken(12))
				request.Header.Set("Idempotency-Key", "legacy-outbound-command-0001")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var payload map[string]any
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload["ok"] != true || payload[testCase.wantKey] == nil {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}
	if query.lastList.BatchID == nil || *query.lastList.BatchID != 91 || query.lastList.Status != outboundapp.TaskStatusCancelled {
		t.Fatalf("list query=%+v", query.lastList)
	}
	if cancel.command.TaskID != 71 || cancel.command.IdempotencyScope != "legacy-admin:1" || cancel.command.IdempotencyKey != "legacy-outbound-command-0001" {
		t.Fatalf("cancel command=%+v", cancel.command)
	}
	if retry.command.TaskID != 71 || retry.command.IdempotencyScope != "legacy-admin:1" {
		t.Fatalf("retry command=%+v", retry.command)
	}
}

func TestLegacyOutboundRoutesFailClosedForOwnerCSRFIdempotencyAndConflicts(t *testing.T) {
	staffID := int64(42)
	now := time.Now().UTC()
	task := outboundapp.TaskReadModel{TaskID: 72, CustomerID: 82, OwnerStaffID: &staffID,
		Status:    outboundapp.TaskStatusPending,
		Job:       outboundapp.TaskJob{TaskID: 72, Generation: 1, RiverJobID: 103, JobKind: outboundapp.OutboundEnqueueOneJobKind},
		CreatedAt: now, StatusUpdatedAt: now}

	t.Run("sales read is owner scoped", func(t *testing.T) {
		query := &legacyOutboundQueryStub{task: task, list: outboundapp.TaskListResult{Items: []outboundapp.TaskReadModel{task}}}
		router := legacyOutboundRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}}, query, &legacyCancelStub{}, &legacyRetryStub{})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/push-center/jobs/72", legacyToken(13)))
		if response.Code != http.StatusOK || query.lastGet.OwnerStaffID == nil || *query.lastGet.OwnerStaffID != staffID {
			t.Fatalf("status=%d query=%+v body=%s", response.Code, query.lastGet, response.Body.String())
		}
	})

	t.Run("ops read remains global", func(t *testing.T) {
		requestedOwner := int64(99)
		query := &legacyOutboundQueryStub{list: outboundapp.TaskListResult{}}
		router := legacyOutboundRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}}, query, &legacyCancelStub{}, &legacyRetryStub{})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/push-center/jobs?owner_userid=99", legacyToken(22)))
		if response.Code != http.StatusOK || query.lastList.OwnerStaffID == nil || *query.lastList.OwnerStaffID != requestedOwner {
			t.Fatalf("status=%d query=%+v body=%s", response.Code, query.lastList, response.Body.String())
		}
	})

	t.Run("sales list and reconciliation use the same owner scope", func(t *testing.T) {
		query := &legacyOutboundQueryStub{task: task, reconciliation: outboundapp.TaskReconciliation{Task: task}}
		router := legacyOutboundRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}}, query, &legacyCancelStub{}, &legacyRetryStub{})
		for _, target := range []string{"/api/admin/push-center/jobs", "/api/admin/push-center/jobs/72/reconciliation"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(23)))
			if response.Code != http.StatusOK {
				t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
			}
		}
		if query.lastList.OwnerStaffID == nil || *query.lastList.OwnerStaffID != staffID || query.lastGet.OwnerStaffID == nil || *query.lastGet.OwnerStaffID != staffID {
			t.Fatalf("list=%+v get=%+v", query.lastList, query.lastGet)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/push-center/jobs?owner_userid=99", legacyToken(24)))
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
	})

	t.Run("sales cannot control", func(t *testing.T) {
		router := legacyOutboundRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}}, &legacyOutboundQueryStub{task: task}, &legacyCancelStub{}, &legacyRetryStub{})
		request := legacyRequest(http.MethodPost, "/api/admin/push-center/jobs/72/cancel", legacyToken(14))
		request.Header.Set("X-CSRF-Token", legacyToken(15))
		request.Header.Set("Idempotency-Key", "legacy-outbound-command-0002")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
	})

	tests := []struct {
		name   string
		key    string
		csrf   bool
		err    error
		status int
		code   platformhttp.ErrorCode
	}{
		{"missing csrf", "legacy-outbound-command-0003", false, nil, http.StatusForbidden, platformhttp.CodeUnauthorized},
		{"short idempotency", "short", true, nil, http.StatusBadRequest, platformhttp.CodeMalformedRequest},
		{"transition conflict", "legacy-outbound-command-0004", true, outboundapp.ErrCancelTransitionConflict, http.StatusConflict, platformhttp.CodeConflict},
		{"worker won", "legacy-outbound-command-0005", true, outboundapp.ErrCancelWorkerWon, http.StatusConflict, platformhttp.CodeConflict},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			cancel := &legacyCancelStub{err: testCase.err}
			router := legacyOutboundRouter(t, &legacyAuthStub{}, &legacyOutboundQueryStub{task: task}, cancel, &legacyRetryStub{})
			request := legacyRequest(http.MethodPost, "/api/admin/push-center/jobs/72/cancel", legacyToken(16))
			request.Header.Set("Idempotency-Key", testCase.key)
			if testCase.csrf {
				request.Header.Set("X-CSRF-Token", legacyToken(17))
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assertLegacyError(t, response, testCase.status, testCase.code)
		})
	}

	t.Run("unsupported legacy filter", func(t *testing.T) {
		router := legacyOutboundRouter(t, &legacyAuthStub{}, &legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/push-center/jobs?external_userid=secret", legacyToken(18)))
		assertLegacyError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
	})
}

func TestLegacyOutboundProjectionNeverLeaksProviderIdentifiersOrFailureText(t *testing.T) {
	now := time.Date(2026, 8, 23, 7, 0, 0, 0, time.UTC)
	owner := int64(42)
	attemptID := int64(501)
	sentAt := now.Add(-time.Minute)
	sent := outboundapp.TaskReadModel{
		TaskID: 73, CustomerID: 83, OwnerStaffID: &owner, Status: outboundapp.TaskStatusSent,
		AttemptCount: 1, CurrentAttemptID: &attemptID, ProviderMessageID: "provider-message-id-must-not-leak",
		Job:       outboundapp.TaskJob{TaskID: 73, Generation: 1, RiverJobID: 103, JobKind: outboundapp.OutboundEnqueueOneJobKind},
		CreatedAt: now.Add(-time.Hour), StatusUpdatedAt: now, SentAt: &sentAt,
	}
	failed := outboundapp.TaskReadModel{
		TaskID: 74, CustomerID: 84, OwnerStaffID: &owner, Status: outboundapp.TaskStatusFinalFailed,
		AttemptCount: 1, CurrentAttemptID: &attemptID, LastFailureKind: outboundapp.ProviderFailureInvalidArgument,
		LastError: `{"access_token":"raw-last-error-must-not-leak"}`,
		Job:       outboundapp.TaskJob{TaskID: 74, Generation: 1, RiverJobID: 104, JobKind: outboundapp.OutboundEnqueueOneJobKind},
		CreatedAt: now.Add(-time.Hour), StatusUpdatedAt: now,
	}
	unknown := failed
	unknown.TaskID, unknown.CustomerID, unknown.Status = 75, 85, outboundapp.TaskStatusOutcomeUnknown
	unknown.Job.TaskID, unknown.Job.RiverJobID = 75, 105
	unknown.LastFailureKind = outboundapp.ProviderFailureTimeout
	unknown.LastError = "Bearer raw-outcome-token-must-not-leak"

	for _, testCase := range []struct {
		name               string
		task               outboundapp.TaskReadModel
		wantFailureClass   string
		wantReceiptPresent bool
	}{
		{name: "sent", task: sent, wantFailureClass: "none", wantReceiptPresent: true},
		{name: "failed", task: failed, wantFailureClass: "local_failure"},
		{name: "outcome unknown", task: unknown, wantFailureClass: "outcome_unknown"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(mapLegacyOutboundJob(testCase.task))
			if err != nil {
				t.Fatal(err)
			}
			assertLegacyOutboundSafeProjection(t, encoded, testCase.wantFailureClass, testCase.wantReceiptPresent)
		})
	}

	reconciliation := outboundapp.TaskReconciliation{Task: unknown, Attempts: []outboundapp.AttemptReadModel{
		{ID: 601, HistoryID: 602, TaskID: 75, Generation: 1, RiverJobID: 105, RiverAttempt: 1, RiverMaxAttempts: 2,
			State: outboundapp.SendAttemptSucceeded, ProviderMessageID: "attempt-provider-id-must-not-leak", CompletedAt: &now},
		{ID: 603, HistoryID: 604, TaskID: 75, Generation: 1, RiverJobID: 105, RiverAttempt: 2, RiverMaxAttempts: 2,
			State: outboundapp.SendAttemptOutcomeUnknown, FailureKind: outboundapp.ProviderFailureTimeout,
			ProviderCode: `{"cookie":"attempt-provider-error-must-not-leak"}`, CompletedAt: &now},
	}}
	router := legacyOutboundRouter(t, &legacyAuthStub{}, &legacyOutboundQueryStub{task: sent, list: outboundapp.TaskListResult{Items: []outboundapp.TaskReadModel{sent, failed, unknown}}, reconciliation: reconciliation}, &legacyCancelStub{}, &legacyRetryStub{})
	for _, target := range []string{
		"/api/admin/push-center/jobs",
		"/api/admin/push-center/jobs/73",
		"/api/admin/push-center/jobs/75/reconciliation",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(19)))
		if response.Code != http.StatusOK {
			t.Fatalf("target=%s status=%d body=%s", target, response.Code, response.Body.String())
		}
		body := response.Body.String()
		for _, forbidden := range []string{"provider-message-id-must-not-leak", "raw-last-error-must-not-leak", "raw-outcome-token-must-not-leak", "attempt-provider-id-must-not-leak", "attempt-provider-error-must-not-leak", `"message_id":`, `"provider_receipt":`, `"failure":`, `"code":`} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("target=%s leaked %q: %s", target, forbidden, body)
			}
		}
		for _, required := range []string{`"local_fact_only":true`, `"real_external_call_executed":false`, `"delivery_proven":false`, `"delivery_semantics":"local_state_not_delivery_proof"`} {
			if !strings.Contains(body, required) {
				t.Fatalf("target=%s missing %s: %s", target, required, body)
			}
		}
	}
}

func TestLegacyOutboundControlReceiptsStayLocalOnlyAcrossReplay(t *testing.T) {
	now := time.Date(2026, 8, 23, 8, 0, 0, 0, time.UTC)
	job := outboundapp.TaskJob{TaskID: 76, Generation: 1, RiverJobID: 106, JobKind: outboundapp.OutboundEnqueueOneJobKind}
	for _, testCase := range []struct {
		name, path string
		role       authport.Role
		cancel     *legacyCancelStub
		retry      *legacyRetryStub
	}{
		{name: "admin cancel", path: "/api/admin/push-center/jobs/76/cancel", role: authport.RoleAdmin, cancel: &legacyCancelStub{result: outboundapp.CancelledTask{ReceiptID: 701, TaskID: 76, CustomerID: 86, Status: outboundapp.TaskStatusCancelled, Job: job, EventID: 702, CancelledAt: now}}, retry: &legacyRetryStub{}},
		{name: "ops manual retry", path: "/api/admin/push-center/jobs/76/retry", role: authport.RoleOps, cancel: &legacyCancelStub{}, retry: &legacyRetryStub{result: outboundapp.ManualRetryResult{ReceiptID: 703, TaskID: 76, CustomerID: 86, Status: outboundapp.TaskStatusPending, Job: job, EventID: 704, RetriedAt: now}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := legacyOutboundRouter(t, &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: testCase.role}}, &legacyOutboundQueryStub{}, testCase.cancel, testCase.retry)
			var first string
			for call := 0; call < 2; call++ {
				request := legacyRequest(http.MethodPost, testCase.path, legacyToken(20))
				request.Header.Set("X-CSRF-Token", legacyToken(21))
				request.Header.Set("Idempotency-Key", "legacy-outbound-replay-0001")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusAccepted {
					t.Fatalf("call=%d status=%d body=%s", call, response.Code, response.Body.String())
				}
				body := response.Body.String()
				if first != "" && body != first {
					t.Fatalf("replay changed projection\nfirst=%s\nagain=%s", first, body)
				}
				first = body
				for _, required := range []string{`"provider_receipt_present":false`, `"delivery_proven":false`, `"local_fact_only":true`, `"real_external_call_executed":false`, `"delivery_semantics":"local_state_not_delivery_proof"`} {
					if !strings.Contains(body, required) {
						t.Fatalf("missing %s: %s", required, body)
					}
				}
			}
		})
	}
}

func assertLegacyOutboundSafeProjection(t *testing.T, body []byte, wantFailureClass string, wantReceiptPresent bool) {
	t.Helper()
	value := string(body)
	for _, forbidden := range []string{`"message_id":`, `"provider_receipt":`, `"failure":`, `"code":`, "must-not-leak"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe projection contains %q: %s", forbidden, value)
		}
	}
	for _, required := range []string{
		`"failure_class":"` + wantFailureClass + `"`,
		`"provider_receipt_present":` + strconv.FormatBool(wantReceiptPresent),
		`"delivery_proven":false`, `"local_fact_only":true`, `"real_external_call_executed":false`,
		`"delivery_semantics":"local_state_not_delivery_proof"`,
	} {
		if !strings.Contains(value, required) {
			t.Fatalf("projection missing %s: %s", required, value)
		}
	}
}

func legacyOutboundRouter(t *testing.T, service authport.Service, query legacyOutboundQueryApplication, cancel legacyCancelApplication, retry legacyRetryApplication) http.Handler {
	t.Helper()
	legacy, err := NewHandlerWithOutbound(service, &legacyCustomerStub{result: legacyCustomerResult()}, query, cancel, retry)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type legacyOutboundQueryStub struct {
	task           outboundapp.TaskReadModel
	list           outboundapp.TaskListResult
	reconciliation outboundapp.TaskReconciliation
	err            error
	lastGet        outboundapp.TaskGetQuery
	lastList       outboundapp.TaskListQuery
}

func (stub *legacyOutboundQueryStub) Get(_ context.Context, query outboundapp.TaskGetQuery) (outboundapp.TaskReadModel, error) {
	stub.lastGet = query
	return stub.task, stub.err
}
func (stub *legacyOutboundQueryStub) List(_ context.Context, query outboundapp.TaskListQuery) (outboundapp.TaskListResult, error) {
	stub.lastList = query
	return stub.list, stub.err
}
func (stub *legacyOutboundQueryStub) Reconcile(_ context.Context, query outboundapp.TaskGetQuery) (outboundapp.TaskReconciliation, error) {
	stub.lastGet = query
	return stub.reconciliation, stub.err
}

type legacyCancelStub struct {
	result  outboundapp.CancelledTask
	err     error
	command outboundapp.CancelCommand
}

func (stub *legacyCancelStub) Cancel(_ context.Context, command outboundapp.CancelCommand) (outboundapp.CancelledTask, error) {
	stub.command = command
	return stub.result, stub.err
}

type legacyRetryStub struct {
	result  outboundapp.ManualRetryResult
	err     error
	command outboundapp.ManualRetryCommand
}

func (stub *legacyRetryStub) Retry(_ context.Context, command outboundapp.ManualRetryCommand) (outboundapp.ManualRetryResult, error) {
	stub.command = command
	return stub.result, stub.err
}

func legacyOutboundInt64(value int64) *int64 { return &value }
func legacyOutboundInt32(value int32) *int32 { return &value }
