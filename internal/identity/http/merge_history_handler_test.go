package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

type mergeHistoryApplicationStub struct {
	page       identityport.CustomerMergeHistoryPage
	err        error
	customerID contactport.CustomerID
	cursor     string
	limit      int32
	calls      int
}

func (stub *mergeHistoryApplicationStub) ListCustomerMergeHistory(
	_ context.Context,
	customerID contactport.CustomerID,
	cursor string,
	limit int32,
) (identityport.CustomerMergeHistoryPage, error) {
	stub.calls++
	stub.customerID, stub.cursor, stub.limit = customerID, cursor, limit
	return stub.page, stub.err
}

func TestMergeHistoryHandlerReturnsOnlyRedactedLocalAuditFacts(t *testing.T) {
	stub := &mergeHistoryApplicationStub{page: identityport.CustomerMergeHistoryPage{
		CustomerID: 41,
		Items: []identityport.CustomerMergeHistory{{
			MergeAuditID: 9, PrimaryCustomerID: 41, MergedCustomerID: 42, Mode: "manual",
			PolicyVersion: "manual_review_v1", MergedAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		}},
		NextCursor: "next",
	}}
	handler, err := NewMergeHistoryHandler(stub)
	if err != nil {
		t.Fatal(err)
	}
	request := reviewRequest(t, http.MethodGet, "", authport.CapabilityIdentityReviewRead)
	response := httptest.NewRecorder()
	handler.GetCustomerMergeHistory(response, request, 41, CustomerMergeHistoryQuery{Cursor: "opaque", Limit: 25})
	if response.Code != http.StatusOK || stub.calls != 1 || stub.customerID != 41 || stub.cursor != "opaque" || stub.limit != 25 {
		t.Fatalf("status=%d stub=%#v body=%s", response.Code, stub, response.Body.String())
	}
	var payload customerMergeHistoryPageResponse
	if err = json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.CustomerID != 41 || len(payload.Items) != 1 || payload.NextCursor == nil ||
		payload.IdentityValuesIncluded || payload.OperatorIdentifiersIncluded || payload.ChatContentIncluded || payload.RealExternalCallExecuted {
		t.Fatalf("payload=%#v err=%v", payload, err)
	}
	for _, forbidden := range []string{"identity", "fingerprint", "operated_by", "detail", "content", "provider", "receipt"} {
		if strings.Contains(strings.ToLower(response.Body.String()), `"`+forbidden+`"`) && forbidden != "identity" && forbidden != "content" {
			t.Fatalf("unsafe field %q in %s", forbidden, response.Body.String())
		}
	}
}

func TestMergeHistoryHandlerFailsClosedForRoleCursorAndProjection(t *testing.T) {
	stub := &mergeHistoryApplicationStub{page: identityport.CustomerMergeHistoryPage{CustomerID: 41}}
	handler, _ := NewMergeHistoryHandler(stub)

	wrong := reviewRequest(t, http.MethodGet, "", authport.CapabilityCustomersRead)
	response := httptest.NewRecorder()
	handler.GetCustomerMergeHistory(response, wrong, 41, CustomerMergeHistoryQuery{})
	if response.Code != http.StatusForbidden || stub.calls != 0 {
		t.Fatalf("wrong capability status=%d calls=%d", response.Code, stub.calls)
	}

	sales := httptest.NewRequest(http.MethodGet, "/api/v1/customers/41/merge-history", nil)
	ctx := authport.WithAuthenticatedSession(sales.Context(), authport.Principal{AdminUserID: 8, Role: authport.RoleSales, StaffID: int64Pointer(7)}, "session")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityIdentityReviewRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.GetCustomerMergeHistory(response, sales.WithContext(ctx), 41, CustomerMergeHistoryQuery{})
	if response.Code != http.StatusForbidden || stub.calls != 0 {
		t.Fatalf("sales status=%d calls=%d", response.Code, stub.calls)
	}

	stub.err = identityapp.ErrCustomerMergeHistoryInvalid
	response = httptest.NewRecorder()
	handler.GetCustomerMergeHistory(response, reviewRequest(t, http.MethodGet, "", authport.CapabilityIdentityReviewRead), 41, CustomerMergeHistoryQuery{Cursor: "bad"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("cursor status=%d body=%s", response.Code, response.Body.String())
	}

	stub.err = nil
	stub.page = identityport.CustomerMergeHistoryPage{CustomerID: 41, Items: []identityport.CustomerMergeHistory{{
		MergeAuditID: 1, PrimaryCustomerID: 41, MergedCustomerID: 42, Mode: "manual", PolicyVersion: "bad\npolicy", MergedAt: time.Now(),
	}}}
	response = httptest.NewRecorder()
	handler.GetCustomerMergeHistory(response, reviewRequest(t, http.MethodGet, "", authport.CapabilityIdentityReviewRead), 41, CustomerMergeHistoryQuery{})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unsafe projection status=%d body=%s", response.Code, response.Body.String())
	}
}

func int64Pointer(value int64) *int64 { return &value }
