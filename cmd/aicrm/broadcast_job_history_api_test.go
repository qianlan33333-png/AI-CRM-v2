package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type broadcastJobHistoryAPIReader struct {
	err                           error
	empty, inconsistent           bool
	invalid, wrongTarget, missing bool
	calls                         int
	query                         outboundport.BroadcastJobHistoryQuery
}

func (reader *broadcastJobHistoryAPIReader) ListHistoricalBroadcastJobs(_ context.Context, query outboundport.BroadcastJobHistoryQuery) ([]outboundport.HistoricalBroadcastJob, int64, error) {
	reader.calls++
	reader.query = query
	if reader.missing {
		return nil, 0, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	if reader.empty {
		return nil, 0, reader.err
	}
	item := broadcastJobHistoryAPIValue(11)
	if reader.invalid {
		item.SourceKeyDigest = [32]byte{}
	}
	if reader.inconsistent {
		return []outboundport.HistoricalBroadcastJob{item}, 2, reader.err
	}
	return []outboundport.HistoricalBroadcastJob{item}, 1, reader.err
}

func (reader *broadcastJobHistoryAPIReader) GetHistoricalBroadcastJob(_ context.Context, id int64) (outboundport.HistoricalBroadcastJob, error) {
	reader.calls++
	if reader.missing {
		return outboundport.HistoricalBroadcastJob{}, outboundport.ErrBroadcastJobHistoryUnavailable
	}
	item := broadcastJobHistoryAPIValue(id)
	if reader.invalid {
		item.SourcePayloadDigest = [32]byte{}
	}
	if reader.wrongTarget {
		item.ID++
	}
	return item, reader.err
}

func broadcastJobHistoryAPIValue(id int64) outboundport.HistoricalBroadcastJob {
	digest := func(seed byte) [32]byte { return sha256.Sum256([]byte{seed}) }
	stamp := time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC)
	legacyID := int64(-915777)
	optional := "legacy"
	optionalDigest := digest(20)
	return outboundport.HistoricalBroadcastJob{
		ID: id, SourceID: 101, OriginalSourceType: "legacy_unknown", SourceReferenceDigest: digest(1), SourceTable: "legacy_table", ScheduledFor: stamp, Priority: -1,
		BatchKeyDigest: digest(2), OriginalStatus: "old_status", RequiresApproval: true, ApprovedByDigest: digest(3), CancelledByDigest: digest(4), CancelReasonDigest: digest(5),
		TargetCount: -2, TargetSummaryDigest: digest(6), ContentType: "legacy_blob", ContentPayloadDigest: digest(7), ContentSummaryDigest: digest(8), AttemptCount: -3,
		LastErrorDigest: digest(9), LegacyOutboundTaskID: &legacyID, SentCount: -4, FailedCount: -5, TraceIDDigest: digest(10), CreatedByDigest: digest(11),
		CreatedAt: stamp, UpdatedAt: stamp, ClaimTokenDigest: digest(12), BusinessDomain: &optional, IdempotencyKeyDigest: &optionalDigest, Channel: &optional,
		TargetKind: &optional, FailureType: &optional, RetryPolicyDigest: digest(13), MetadataDigest: digest(14), TargetUnionIDsDigest: digest(15), MaxAttempts: -6,
		SideEffectExecuted: true, ProviderResultReceived: true, ResultSummaryDigest: digest(16), ReconciliationRequired: true, HoldReasonDigest: digest(17),
		LegacyExternalEffectJobID: &legacyID, ExecutionIDDigest: digest(18), ExecutionOwnerDigest: digest(19), SourceKeyDigest: digest(21), SourcePayloadDigest: digest(22),
		SourceFieldDigest: digest(23), RedactedRoots: []string{"claim_token"},
	}
}

type broadcastJobHistoryAPIAuth struct {
	role         authport.Role
	csrfCalls    int
	capabilities []authport.Capability
}

func (service *broadcastJobHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: service.role}, nil
}
func (service *broadcastJobHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.Role != authport.RoleAdmin || capability != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	service.capabilities = append(service.capabilities, capability)
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}
func (service *broadcastJobHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	service.csrfCalls++
	return nil
}
func (*broadcastJobHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func broadcastJobHistoryAPIRouter(t *testing.T, reader outboundport.BroadcastJobHistoryReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.broadcastJobHistory = reader
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func broadcastJobHistoryBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal("invalid_json")
	}
	return body
}

func broadcastJobHistoryItem(t *testing.T, body map[string]any, detail bool) map[string]any {
	t.Helper()
	if detail {
		item, ok := body["item"].(map[string]any)
		if !ok {
			t.Fatal("missing_detail_item")
		}
		return item
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatal("missing_page_item")
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("invalid_page_item")
	}
	return item
}

func TestFinalRouterBroadcastJobHistoryAdminReadOnly(t *testing.T) {
	reader, auth := &broadcastJobHistoryAPIReader{}, &broadcastJobHistoryAPIAuth{role: authport.RoleAdmin}
	router := broadcastJobHistoryAPIRouter(t, reader, auth)
	for detail, path := range map[bool]string{false: "/api/admin/broadcast-job-history?limit=1&offset=0", true: "/api/admin/broadcast-job-history/12"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xb1)))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		body := broadcastJobHistoryBody(t, response)
		if body["source"] != "v1_history" || body["read_only"] != true || body["real_external_call_executed"] != false {
			t.Fatal("history_boundary_missing")
		}
		item := broadcastJobHistoryItem(t, body, detail)
		if item["source_id"] != float64(101) || item["original_side_effect_executed"] != true || item["original_provider_result_received"] != true || item["original_reconciliation_required"] != true {
			t.Fatal("original_observation_not_mapped")
		}
		for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "source_reference_digest", "batch_key_digest", "content_payload_digest", "outbound_task_id", "external_effect_job_id", "redacted_roots"} {
			if _, leaked := item[private]; leaked || strings.Contains(response.Body.String(), private) {
				t.Fatalf("private_field_leaked=%s", private)
			}
		}
		if _, current := item["side_effect_executed"]; current {
			t.Fatal("legacy_effect_was_presented_as_current")
		}
	}
	if reader.query.Limit != 1 || reader.query.Offset != 0 || auth.csrfCalls != 0 || len(auth.capabilities) != 2 {
		t.Fatal("query_or_read_auth_incorrect")
	}
	for _, capability := range auth.capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatal("non_read_capability")
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/broadcast-job-history", nil)
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(0xb2)})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || auth.csrfCalls != 0 {
		t.Fatal("current_session_read_failed")
	}
}

func TestFinalRouterBroadcastJobHistoryRejectsInvalidAndUnauthorizedReads(t *testing.T) {
	for _, test := range []struct {
		name string
		role authport.Role
		want int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"ops", authport.RoleOps, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &broadcastJobHistoryAPIReader{}
			router := broadcastJobHistoryAPIRouter(t, reader, &broadcastJobHistoryAPIAuth{role: test.role})
			for _, path := range []string{"/api/admin/broadcast-job-history", "/api/admin/broadcast-job-history/12"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if test.role != "" {
					request = legacyRequest(http.MethodGet, path, legacyToken(0xb3))
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != test.want {
					t.Fatalf("%s status=%d", path, response.Code)
				}
			}
			if reader.calls != 0 {
				t.Fatal("denied_read_reached_reader")
			}
		})
	}
	reader := &broadcastJobHistoryAPIReader{}
	router := broadcastJobHistoryAPIRouter(t, reader, &broadcastJobHistoryAPIAuth{role: authport.RoleAdmin})
	for _, query := range []string{"limit=0", "limit=101", "limit=1&limit=2", "offset=-1", "offset=1&offset=2", "unknown=true", "limit=%zz"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history?"+query, legacyToken(0xb4)))
		if response.Code != http.StatusBadRequest || reader.calls != 0 {
			t.Fatalf("query=%s status=%d calls=%d", query, response.Code, reader.calls)
		}
	}
	for _, id := range []string{"0", "01", "-1", "x", "9223372036854775808"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history/"+id, legacyToken(0xb5)))
		if response.Code != http.StatusBadRequest || reader.calls != 0 {
			t.Fatalf("id=%s status=%d calls=%d", id, response.Code, reader.calls)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history/12?limit=1", legacyToken(0xb6)))
	if response.Code != http.StatusBadRequest || reader.calls != 0 {
		t.Fatal("detail_query_reached_reader")
	}
	for _, path := range []string{"/api/admin/broadcast-job-history", "/api/admin/broadcast-job-history/12"} {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(0xb7)))
		if (response.Code >= 200 && response.Code < 300) || reader.calls != 0 {
			t.Fatalf("write_route=%s status=%d calls=%d", path, response.Code, reader.calls)
		}
	}
}

func TestFinalRouterBroadcastJobHistoryFailsClosedForReaderProblems(t *testing.T) {
	reader := &broadcastJobHistoryAPIReader{empty: true}
	router := broadcastJobHistoryAPIRouter(t, reader, &broadcastJobHistoryAPIAuth{role: authport.RoleAdmin})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history", legacyToken(0xb8)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) || reader.query.Limit != 50 || reader.query.Offset != 0 {
		t.Fatal("empty_history_not_readable")
	}
	for _, problem := range []string{"error", "missing", "inconsistent", "invalid", "wrong_target"} {
		reader.empty, reader.err, reader.inconsistent, reader.invalid, reader.wrongTarget, reader.missing = false, nil, false, false, false, false
		switch problem {
		case "error":
			reader.err = errors.New("private reader detail")
		case "missing":
			reader.missing = true
		case "inconsistent":
			reader.inconsistent = true
		case "invalid":
			reader.invalid = true
		case "wrong_target":
			reader.wrongTarget = true
		}
		response = httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history", legacyToken(0xb9)))
		listStatus := http.StatusServiceUnavailable
		if problem == "wrong_target" {
			listStatus = http.StatusOK
		}
		if response.Code != listStatus || strings.Contains(response.Body.String(), "private reader") || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s list=%d %s", problem, response.Code, response.Body.String())
		}
		if problem != "inconsistent" {
			response = httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history/12", legacyToken(0xba)))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s detail=%d", problem, response.Code)
			}
		}
	}
	for _, dependency := range []outboundport.BroadcastJobHistoryReader{nil, (*broadcastJobHistoryAPIReader)(nil)} {
		response := httptest.NewRecorder()
		broadcastJobHistoryAPIRouter(t, dependency, &broadcastJobHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/broadcast-job-history", legacyToken(0xbb)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatal("nil_dependency_accepted")
		}
	}
}

var _ outboundport.BroadcastJobHistoryReader = (*broadcastJobHistoryAPIReader)(nil)
var _ authport.Service = (*broadcastJobHistoryAPIAuth)(nil)
