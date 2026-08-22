package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type legacyAdminOpsTransportStub struct {
	legacyAdminOps
	created      []adminopsapp.CredentialCommand
	credentials  []adminopsport.Credential
	listErr      error
	job          adminopsport.Job
	jobErr       error
	jobKeys      []string
	listJobCalls int
	cancelCalls  int
}

func (stub *legacyAdminOpsTransportStub) CreateCredential(_ context.Context, command adminopsapp.CredentialCommand) (adminopsport.Credential, error) {
	stub.created = append(stub.created, command)
	return adminopsport.Credential{Kind: command.Kind, ClientID: command.ClientID, DisplayName: command.DisplayName, State: "active", SecretRef: "secret://adminops/direct_api_key/direct-default/12345678", SecretMask: "masked:…345678"}, nil
}

func (stub *legacyAdminOpsTransportStub) ListCredentials(context.Context) ([]adminopsport.Credential, error) {
	return stub.credentials, stub.listErr
}

func (stub *legacyAdminOpsTransportStub) GetJob(_ context.Context, key string) (adminopsport.Job, error) {
	stub.jobKeys = append(stub.jobKeys, key)
	return stub.job, stub.jobErr
}

func (stub *legacyAdminOpsTransportStub) ListJobs(context.Context, string, string, int32) ([]adminopsport.Job, error) {
	stub.listJobCalls++
	return nil, nil
}

func (stub *legacyAdminOpsTransportStub) CancelJob(context.Context, string, int64, string, string) (adminopsport.Job, error) {
	stub.cancelCalls++
	return adminopsport.Job{}, nil
}

func TestAdminOpsAPIClientListPageUsesLocalProjectionAndSafeFilters(t *testing.T) {
	updatedAt := time.Date(2026, 8, 19, 2, 3, 4, 0, time.UTC)
	stub := &legacyAdminOpsTransportStub{credentials: []adminopsport.Credential{
		{Kind: adminopsport.CredentialAPIClient, ClientID: "alpha.client", DisplayName: "Alpha <script>alert(1)</script>", State: "active", SecretRef: "secret://must-not-render", SecretMask: "masked:must-not-render", Version: 3, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "beta.client", DisplayName: "Beta", State: "disabled", Version: 2, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "gamma.client", DisplayName: "Gamma", State: "pending_activation", Version: 1, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialDirectAPIKey, ClientID: "direct-key", DisplayName: "Direct", State: "active", Version: 1, UpdatedAt: updatedAt},
	}}
	handler := &Handler{adminOps: stub}

	response := httptest.NewRecorder()
	handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?q=ALPHA&status=enabled", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Cache-Control"), "private") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	for _, want := range []string{"Alpha &lt;script&gt;alert(1)&lt;/script&gt;", "/admin/config/api-clients/alpha.client", "/admin/config/api-clients/new", "共 3 个", "已启用 1 个", "已停用 1 个", "待激活 1 个"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	for _, forbidden := range []string{"beta.client", "gamma.client", "direct-key", "secret://must-not-render", "masked:must-not-render", "<script>alert(1)</script>"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe or unfiltered value %q in %s", forbidden, body)
		}
	}
	for _, testCase := range []struct {
		status, want, forbidden string
	}{
		{status: "enabled", want: "alpha.client", forbidden: "beta.client"},
		{status: "disabled", want: "beta.client", forbidden: "gamma.client"},
		{status: "pending_activation", want: "gamma.client", forbidden: "beta.client"},
	} {
		filtered := httptest.NewRecorder()
		handler.AdminOps(filtered, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?status="+testCase.status, nil))
		if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), testCase.want) || strings.Contains(filtered.Body.String(), testCase.forbidden) {
			t.Fatalf("status filter=%s code=%d body=%s", testCase.status, filtered.Code, filtered.Body.String())
		}
	}
}

func TestAdminOpsAPIClientListPageRejectsBadStatusBeforeReading(t *testing.T) {
	stub := &legacyAdminOpsTransportStub{listErr: errors.New("must not be called")}
	handler := &Handler{adminOps: stub}
	response := httptest.NewRecorder()
	handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?status=unknown", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error":"invalid_status_filter"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminOpsAPIClientListPageFailsClosedForInvalidProjection(t *testing.T) {
	for _, credential := range []adminopsport.Credential{
		{Kind: adminopsport.CredentialAPIClient, ClientID: "broken.client", DisplayName: "Broken", State: "unknown", Version: 1, UpdatedAt: time.Now().UTC()},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "..", DisplayName: "Unsafe path", State: "active", Version: 1, UpdatedAt: time.Now().UTC()},
	} {
		stub := &legacyAdminOpsTransportStub{credentials: []adminopsport.Credential{credential}}
		handler := &Handler{adminOps: stub}
		response := httptest.NewRecorder()
		handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients", nil))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"error":"admin_ops_unavailable"`) || strings.Contains(response.Body.String(), credential.ClientID) {
			t.Fatalf("credential=%q status=%d body=%s", credential.ClientID, response.Code, response.Body.String())
		}
	}
}

func TestAdminOpsDirectKeyRequiresSessionRBACCSRFAndActionToken(t *testing.T) {
	stub := &legacyAdminOpsTransportStub{}
	handler := &Handler{adminOps: stub, auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}}
	tail, err := handler.Authorize(authport.CapabilityConfigSettingsManage, http.HandlerFunc(handler.AdminOps))
	if err != nil {
		t.Fatal(err)
	}
	tail, err = handler.RequireCSRF(tail)
	if err != nil {
		t.Fatal(err)
	}
	route := handler.Authenticate(tail)
	session, csrf := authport.SessionRef(legacyToken(4)), legacyToken(5)
	request := func(token string) *http.Request {
		body := `{"confirm":true,"admin_action_token":"` + token + `"}`
		r := httptest.NewRequest(http.MethodPost, "/api/admin/config/api-key/generate", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: string(session)})
		return r
	}
	response := httptest.NewRecorder()
	route.ServeHTTP(response, request(adminOpsActionToken(session, http.MethodPost, "/api/admin/config/api-key/generate")))
	if response.Code != http.StatusCreated || len(stub.created) != 1 || stub.created[0].Actor != "admin:7" {
		t.Fatalf("status=%d created=%#v", response.Code, stub.created)
	}
	if got := response.Body.String(); strings.Contains(got, "client_secret") || strings.Contains(got, "access_token") || !strings.Contains(got, `"secret_mask":"masked:`) {
		t.Fatalf("unsafe credential response: %s", got)
	}
	response = httptest.NewRecorder()
	route.ServeHTTP(response, request("wrong"))
	if response.Code != http.StatusUnauthorized || len(stub.created) != 1 {
		t.Fatalf("invalid action token status=%d created=%d", response.Code, len(stub.created))
	}
}

func TestDecodeAdminOpsPayloadRejectsTrailingJSONAndSecretMaterial(t *testing.T) {
	for _, body := range []string{`{"confirm":true}{"unexpected":true}`, `{"confirm":true,"webhook_url":"https://secret.example"}`} {
		response := httptest.NewRecorder()
		_, ok := decodeAdminOpsPayload(response, httptest.NewRequest(http.MethodPost, "/api/admin/config/api-key/generate", strings.NewReader(body)))
		if ok || response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d ok=%v", body, response.Code, ok)
		}
	}
}

func TestPublicAdminOpsJobUsesClosedTargetProjection(t *testing.T) {
	for _, test := range []struct {
		kind, target, wantKind string
		wantPresent            bool
	}{
		{"feishu_webhook_validate", "secret://notifications/feishu/sensitive-locator", "notification_secret", true},
		{"message_batch_ack", "message_batch:external-identifier", "message_batch", true},
		{"archive_sync", "", "message_archive", false},
	} {
		failure := `{"provider_token":"must-not-leak"}`
		encoded, err := json.Marshal(publicJob(adminopsport.Job{Key: "admjob_123456789012", Kind: test.kind, State: "failed", TargetRef: test.target, Version: 1, FailureCode: failure}))
		if err != nil {
			t.Fatal(err)
		}
		body := string(encoded)
		if strings.Contains(body, "target_ref") || strings.Contains(body, "failure_code") || strings.Contains(body, "must-not-leak") || test.target != "" && strings.Contains(body, test.target) || !strings.Contains(body, `"target_kind":"`+test.wantKind+`"`) || !strings.Contains(body, `"target_present":`+strconv.FormatBool(test.wantPresent)) || !strings.Contains(body, `"failure_present":true`) || !strings.Contains(body, `"failure_class":"local_failure"`) || !strings.Contains(body, `"local_only":true`) || !strings.Contains(body, `"real_external_call_executed":false`) {
			t.Fatalf("unsafe job projection=%s", body)
		}
		if test.wantPresent && !strings.Contains(body, `"target_mask":"masked"`) {
			t.Fatalf("missing target mask=%s", body)
		}
	}
	queuedWithFailure, err := json.Marshal(publicJob(adminopsport.Job{Key: "admjob_123456789012", Kind: "archive_sync", State: "queued", FailureCode: "provider raw token", Version: 1}))
	if err != nil || strings.Contains(string(queuedWithFailure), "provider raw token") || !strings.Contains(string(queuedWithFailure), `"failure_present":true`) || !strings.Contains(string(queuedWithFailure), `"failure_class":"local_failure"`) {
		t.Fatalf("queued failure projection=%s err=%v", queuedWithFailure, err)
	}
}

func TestAdminOpsUnownedJobRoutesFailClosedWithoutReadingGenericJobs(t *testing.T) {
	const key = "admjob_1234567890abcdef1234567890abcdef"
	for _, path := range []string{
		"/api/admin/jobs/callbacks",
		"/api/admin/jobs/deferred-jobs",
		"/api/admin/jobs/webhook-deliveries",
		"/api/admin/broadcast-jobs",
		"/api/admin/broadcast-jobs/" + key,
	} {
		stub := &legacyAdminOpsTransportStub{}
		response := httptest.NewRecorder()
		(&Handler{adminOps: stub}).AdminOps(response, httptest.NewRequest(http.MethodGet, path, nil))
		body := response.Body.String()
		if response.Code != http.StatusConflict || stub.listJobCalls != 0 || len(stub.jobKeys) != 0 || !strings.Contains(body, `"local_only":true`) || !strings.Contains(body, `"real_external_call_executed":false`) {
			t.Fatalf("path=%s status=%d keys=%v list_calls=%d body=%s", path, response.Code, stub.jobKeys, stub.listJobCalls, body)
		}
	}
}

func TestAdminOpsBroadcastWritesFailClosedWithoutJobMutation(t *testing.T) {
	const key = "admjob_1234567890abcdef1234567890abcdef"
	for _, test := range []struct {
		action, body, errorCode string
	}{
		{action: "cancel", body: `{"confirm":true,"version":1,"admin_action_token":"%s"}`, errorCode: "broadcast_job_fact_unavailable"},
		{action: "approve", body: `{"confirm":true,"admin_action_token":"%s"}`, errorCode: "broadcast_job_review_state_unavailable"},
	} {
		stub := &legacyAdminOpsTransportStub{}
		handler := &Handler{adminOps: stub}
		session := authport.SessionRef(legacyToken(4))
		pattern := "/api/admin/broadcast-jobs/{job_id}/" + test.action
		token := adminOpsActionToken(session, http.MethodPost, pattern)
		request := httptest.NewRequest(http.MethodPost, "/api/admin/broadcast-jobs/"+key+"/"+test.action, strings.NewReader(fmt.Sprintf(test.body, token)))
		request = request.WithContext(authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, session))
		response := httptest.NewRecorder()
		handler.AdminOps(response, request)
		if response.Code != http.StatusConflict || stub.cancelCalls != 0 || stub.listJobCalls != 0 || len(stub.jobKeys) != 0 || !strings.Contains(response.Body.String(), `"error":"`+test.errorCode+`"`) {
			t.Fatalf("action=%s status=%d cancel=%d list=%d keys=%v body=%s", test.action, response.Code, stub.cancelCalls, stub.listJobCalls, stub.jobKeys, response.Body.String())
		}
	}
}

func TestAdminOpsMessageBatchDetailFailsClosedWithoutOwnerMapping(t *testing.T) {
	stub := &legacyAdminOpsTransportStub{}
	router := chi.NewRouter()
	router.Get("/api/admin/jobs/message-batches/{batch_id}", (&Handler{adminOps: stub}).AdminOps)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/jobs/message-batches/batch-external-locator", nil))
	body := response.Body.String()
	if response.Code != http.StatusConflict || stub.listJobCalls != 0 || !strings.Contains(body, `"error":"message_batch_job_mapping_unavailable"`) || !strings.Contains(body, `"local_only":true`) || !strings.Contains(body, `"real_external_call_executed":false`) {
		t.Fatalf("status=%d list_calls=%d body=%s", response.Code, stub.listJobCalls, body)
	}
}

func TestReleaseChangesFormOnlyAllowsReferenceForSensitiveValues(t *testing.T) {
	build := func(values url.Values) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/admin/config/releases", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		return request
	}
	changes, err := releaseChangesFromForm(build(url.Values{"key__0": {"wecom.webhook_ref"}, "value__0": {"secretref:wecom/alerts"}, "key__1": {"outbound.rate_per_second"}, "value__1": {"30"}}))
	if err != nil || changes["wecom.webhook_ref"] != "secretref:wecom/alerts" || changes["outbound.rate_per_second"] != "30" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
	for _, values := range []url.Values{{"key__0": {"wecom.webhook_ref"}, "value__0": {"https://raw-secret.example"}}, {"key__0": {"same.key"}, "value__0": {"one"}, "key__1": {"same.key"}, "value__1": {"two"}}} {
		if _, err := releaseChangesFromForm(build(values)); err == nil {
			t.Fatalf("unsafe form was accepted: %v", values)
		}
	}
}
