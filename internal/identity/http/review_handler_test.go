package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type reviewApplicationStub struct {
	page           identityport.MergeReviewPage
	result         identityport.MergeReview
	err            error
	listStatus     identityport.MergeReviewStatus
	listCursor     string
	listLimit      int32
	approveCommand identityport.ApproveMergeReviewCommand
	rejectCommand  identityport.RejectMergeReviewCommand
}

func (stub *reviewApplicationStub) ListMergeReviewsByStatus(_ context.Context, status identityport.MergeReviewStatus, cursor string, limit int32) (identityport.MergeReviewPage, error) {
	stub.listStatus, stub.listCursor, stub.listLimit = status, cursor, limit
	return stub.page, stub.err
}

func (stub *reviewApplicationStub) ApproveMergeReview(_ context.Context, command identityport.ApproveMergeReviewCommand) (identityport.MergeReview, error) {
	stub.approveCommand = command
	return stub.result, stub.err
}

func (stub *reviewApplicationStub) RejectMergeReview(_ context.Context, command identityport.RejectMergeReviewCommand) (identityport.MergeReview, error) {
	stub.rejectCommand = command
	return stub.result, stub.err
}

func TestReviewHandlerListsClosedFactsForAllStatusesAndDefaultsPending(t *testing.T) {
	cursor := generated.Cursor("opaque-cursor")
	limit := generated.Limit(25)
	for _, test := range []struct {
		name   string
		status identityport.MergeReviewStatus
		param  *generated.ListIdentityMergeReviewsParamsStatus
	}{
		{name: "default pending", status: identityport.MergeReviewPending},
		{name: "approved", status: identityport.MergeReviewApproved, param: reviewStatusParam(generated.ListIdentityMergeReviewsParamsStatus("approved"))},
		{name: "rejected", status: identityport.MergeReviewRejected, param: reviewStatusParam(generated.ListIdentityMergeReviewsParamsStatus("rejected"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact := reviewHTTPFact(test.status)
			if test.status != identityport.MergeReviewPending {
				resolved := fact.CreatedAt.Add(time.Hour)
				fact.ResolvedAt = &resolved
				fact.Version = 2
			}
			stub := &reviewApplicationStub{page: identityport.MergeReviewPage{
				Items: []identityport.MergeReview{fact}, NextCursor: "next-cursor",
			}}
			handler, err := NewReviewHandler(stub)
			if err != nil {
				t.Fatal(err)
			}
			request := reviewRequest(t, http.MethodGet, "", authport.CapabilityIdentityReviewRead)
			response := httptest.NewRecorder()
			handler.ListIdentityMergeReviews(response, request, generated.ListIdentityMergeReviewsParams{
				Status: test.param, Cursor: &cursor, Limit: &limit,
			})

			if response.Code != http.StatusOK || stub.listStatus != test.status || stub.listCursor != "opaque-cursor" || stub.listLimit != 25 {
				t.Fatalf("status=%d list_status=%q cursor=%q limit=%d body=%s", response.Code, stub.listStatus, stub.listCursor, stub.listLimit, response.Body.String())
			}
			var payload generated.IdentityMergeReviewPage
			if err = json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.Items) != 1 || payload.NextCursor == nil ||
				identityport.MergeReviewStatus(payload.Items[0].Status) != test.status {
				t.Fatalf("payload=%+v err=%v", payload, err)
			}
			body := response.Body.String()
			if payload.Items[0].IdentityFingerprint != fact.IdentityFingerprint {
				t.Fatalf("fingerprint=%q want=%q", payload.Items[0].IdentityFingerprint, fact.IdentityFingerprint)
			}
			for _, forbidden := range []string{"normalized", "unionid", "external_userid", "raw_identity", "payload"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("forbidden field %q escaped response: %s", forbidden, body)
				}
			}
		})
	}
}

func TestReviewHandlerRejectsInvalidStatusAndContradictoryResolvedTime(t *testing.T) {
	invalid := generated.ListIdentityMergeReviewsParamsStatus("other")
	stub := &reviewApplicationStub{}
	handler, _ := NewReviewHandler(stub)
	request := reviewRequest(t, http.MethodGet, "", authport.CapabilityIdentityReviewRead)
	response := httptest.NewRecorder()
	handler.ListIdentityMergeReviews(response, request, generated.ListIdentityMergeReviewsParams{Status: &invalid})
	if response.Code != http.StatusUnprocessableEntity || stub.listStatus != "" {
		t.Fatalf("invalid status response=%d body=%s called=%q", response.Code, response.Body.String(), stub.listStatus)
	}

	approved := reviewHTTPFact(identityport.MergeReviewApproved)
	before := approved.CreatedAt.Add(-time.Second)
	approved.ResolvedAt = &before
	stub.page = identityport.MergeReviewPage{Items: []identityport.MergeReview{approved}}
	approvedParam := reviewStatusParam(generated.ListIdentityMergeReviewsParamsStatus("approved"))
	response = httptest.NewRecorder()
	handler.ListIdentityMergeReviews(response, request, generated.ListIdentityMergeReviewsParams{Status: approvedParam})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("contradictory time response=%d body=%s", response.Code, response.Body.String())
	}
}

func TestReviewHandlerMapsApproveAndRejectCommandsFromAuthenticatedActor(t *testing.T) {
	for _, test := range []struct {
		name   string
		status identityport.MergeReviewStatus
		body   string
		call   func(*ReviewHandler, http.ResponseWriter, *http.Request, generated.IdempotencyKey)
		check  func(*reviewApplicationStub) bool
	}{
		{
			name: "approve", status: identityport.MergeReviewApproved,
			body: `{"expected_version":1,"primary_customer_id":84,"reason":"运营确认"}`,
			call: func(handler *ReviewHandler, writer http.ResponseWriter, request *http.Request, key generated.IdempotencyKey) {
				handler.ApproveIdentityMergeReview(writer, request, 23, generated.ApproveIdentityMergeReviewParams{IdempotencyKey: key})
			},
			check: func(stub *reviewApplicationStub) bool {
				return stub.approveCommand.ReviewID == 23 && stub.approveCommand.ExpectedVersion == 1 &&
					stub.approveCommand.PrimaryCustomerID == 84 && stub.approveCommand.Reason == "运营确认" &&
					stub.approveCommand.Actor == "admin:7" && stub.approveCommand.IdempotencyKey == "review-command-23"
			},
		},
		{
			name: "reject", status: identityport.MergeReviewRejected,
			body: `{"expected_version":1,"reason":"手机号换主"}`,
			call: func(handler *ReviewHandler, writer http.ResponseWriter, request *http.Request, key generated.IdempotencyKey) {
				handler.RejectIdentityMergeReview(writer, request, 23, generated.RejectIdentityMergeReviewParams{IdempotencyKey: key})
			},
			check: func(stub *reviewApplicationStub) bool {
				return stub.rejectCommand.ReviewID == 23 && stub.rejectCommand.ExpectedVersion == 1 &&
					stub.rejectCommand.Reason == "手机号换主" && stub.rejectCommand.Actor == "admin:7" &&
					stub.rejectCommand.IdempotencyKey == "review-command-23"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact := reviewHTTPFact(test.status)
			resolved := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
			fact.ResolvedAt = &resolved
			fact.Version = 2
			stub := &reviewApplicationStub{result: fact}
			handler, _ := NewReviewHandler(stub)
			request := reviewRequest(t, http.MethodPost, test.body, authport.CapabilityIdentityReviewWrite)
			response := httptest.NewRecorder()
			test.call(handler, response, request, generated.IdempotencyKey("review-command-23"))
			if response.Code != http.StatusOK || !test.check(stub) {
				t.Fatalf("status=%d body=%s approve=%+v reject=%+v", response.Code, response.Body.String(), stub.approveCommand, stub.rejectCommand)
			}
		})
	}
}

func TestReviewHandlerFailsClosedForAuthorizationBodyAndApplicationErrors(t *testing.T) {
	tests := []struct {
		name       string
		capability authport.Capability
		body       string
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{name: "wrong capability", capability: authport.CapabilityIdentityReviewRead, body: `{"expected_version":1,"reason":"reject"}`, wantStatus: 403, wantCode: platformhttp.CodeUnauthorized},
		{name: "unknown field", capability: authport.CapabilityIdentityReviewWrite, body: `{"expected_version":1,"reason":"reject","identity":"raw"}`, wantStatus: 400, wantCode: platformhttp.CodeMalformedRequest},
		{name: "conflict", capability: authport.CapabilityIdentityReviewWrite, body: `{"expected_version":1,"reason":"reject"}`, err: identityapp.ErrMergeReviewConflict, wantStatus: 409, wantCode: platformhttp.CodeConflict},
		{name: "not found", capability: authport.CapabilityIdentityReviewWrite, body: `{"expected_version":1,"reason":"reject"}`, err: identityapp.ErrMergeReviewNotFound, wantStatus: 404, wantCode: platformhttp.CodeNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &reviewApplicationStub{err: test.err}
			handler, _ := NewReviewHandler(stub)
			request := reviewRequest(t, http.MethodPost, test.body, test.capability)
			response := httptest.NewRecorder()
			handler.RejectIdentityMergeReview(response, request, 23, generated.RejectIdentityMergeReviewParams{IdempotencyKey: "key"})
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			var payload struct {
				Code platformhttp.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Code != test.wantCode {
				t.Fatalf("payload=%+v err=%v", payload, err)
			}
			if strings.Contains(response.Body.String(), "raw") {
				t.Fatal("malformed body value escaped response")
			}
		})
	}
}

func TestReviewHandlerRejectsTypedNilApplication(t *testing.T) {
	var typedNil *reviewApplicationStub
	if handler, err := NewReviewHandler(typedNil); handler != nil || !errors.Is(err, identityapp.ErrMergeReviewUnavailable) {
		t.Fatalf("handler=%v err=%v", handler, err)
	}
}

func reviewRequest(t *testing.T, method, body string, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/identity/merge-reviews", strings.NewReader(body))
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "session")
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

func reviewStatusParam(status generated.ListIdentityMergeReviewsParamsStatus) *generated.ListIdentityMergeReviewsParamsStatus {
	return &status
}

func reviewHTTPFact(status identityport.MergeReviewStatus) identityport.MergeReview {
	return identityport.MergeReview{
		ReviewID: 23, Status: status, Kind: identityport.KindPhone, Scope: "phone:e164",
		CustomerIDs: []contactport.CustomerID{42, 84}, Version: 1,
		IdentityFingerprint: "hmac-sha256-v1:AQEBAQEBAQEBAQEBAQEBAQ",
		CreatedAt:           time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC),
	}
}
