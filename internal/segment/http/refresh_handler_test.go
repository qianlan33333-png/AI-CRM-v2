package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestRefreshHandlerReturnsStableAcceptedFact(t *testing.T) {
	t.Parallel()
	requester := &refreshRequesterStub{result: segmentport.Segment{ID: 7}}
	handler, err := NewRefreshHandler(requester)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.RequestSegmentRefresh(response, refreshRequest(t, 42, authport.CapabilitySegmentsWrite), 7, generated.RequestSegmentRefreshParams{IdempotencyKey: "same-command-key"})
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", response.Code, response.Body.String())
	}
	var body generated.SegmentRefreshAccepted
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != generated.Accepted || body.SegmentId != 7 || requester.command.Actor != "admin:42" || requester.command.SegmentID != 7 {
		t.Fatalf("body/command = %#v/%#v, want accepted segment 7 admin actor", body, requester.command)
	}
}

func TestRefreshHandlerClassifiesFrozenBoundaryFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		capability authport.Capability
		principal  int64
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{name: "wrong capability", capability: authport.CapabilitySegmentsRead, principal: 42, wantStatus: 403, wantCode: platformhttp.CodeUnauthorized},
		{name: "not found", capability: authport.CapabilitySegmentsWrite, principal: 42, err: segmentapp.ErrSegmentNotFound, wantStatus: 404, wantCode: platformhttp.CodeNotFound},
		{name: "different command conflicts", capability: authport.CapabilitySegmentsWrite, principal: 42, err: segmentapp.ErrRefreshCommandConflict, wantStatus: 409, wantCode: platformhttp.CodeConflict},
		{name: "queue dependency", capability: authport.CapabilitySegmentsWrite, principal: 42, err: errors.New("queue down"), wantStatus: 503, wantCode: platformhttp.CodeDependencyUnavailable},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler, err := NewRefreshHandler(&refreshRequesterStub{err: test.err})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.RequestSegmentRefresh(response, refreshRequest(t, test.principal, test.capability), 7, generated.RequestSegmentRefreshParams{IdempotencyKey: "same-command-key"})
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			var body struct {
				Code platformhttp.ErrorCode `json:"code"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", body.Code, test.wantCode)
			}
		})
	}
}

type refreshRequesterStub struct {
	result  segmentport.Segment
	err     error
	command segmentport.RefreshCommand
}

func (stub *refreshRequesterStub) RequestRefresh(_ context.Context, command segmentport.RefreshCommand) (segmentport.Segment, error) {
	stub.command = command
	return stub.result, stub.err
}

func refreshRequest(t *testing.T, adminID int64, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/segments/7/refresh", nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: adminID, Role: authport.RoleAdmin}, "session")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}
