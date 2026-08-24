package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

func TestOwnerReassignmentHandlerAdminOnlyAndSafeDownloads(t *testing.T) {
	application := &ownerReassignmentHTTPFake{preview: contactapp.OwnerReassignmentPreview{ID: "cor_abcdefghijklmnopqrstuv", Hash: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Minute), Issues: []contactapp.OwnerReassignmentIssue{{Line: 2, Code: "invalid_row"}}}}
	handler, err := NewOwnerReassignmentHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	for name, request := range map[string]*http.Request{
		"anonymous": httptest.NewRequest(http.MethodGet, "/template", nil),
		"ops":       ownerReassignmentRequest(t, http.MethodGet, "/template", authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, authport.Authorization{Capability: authport.CapabilityContactOwnerReassignment, Scope: authport.ScopeGlobal}),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.Template(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status=%d", response.Code)
			}
		})
	}
	request := ownerReassignmentRequest(t, http.MethodGet, "/errors", authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authport.Authorization{Capability: authport.CapabilityContactOwnerReassignment, Scope: authport.ScopeGlobal})
	response := httptest.NewRecorder()
	handler.ErrorsCSV(response, request, application.preview.ID)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(response.Body.String(), "external_userid") || !strings.Contains(response.Body.String(), "line,code") {
		t.Fatalf("response=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestOwnerReassignmentHandlerSixOperationsAndStrictCSVMediaType(t *testing.T) {
	preview := contactapp.OwnerReassignmentPreview{ID: "cor_abcdefghijklmnopqrstuv", Hash: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Minute)}
	application := &ownerReassignmentHTTPFake{preview: preview}
	handler, err := NewOwnerReassignmentHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	admin := authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityContactOwnerReassignment, Scope: authport.ScopeGlobal}
	request := func(method, path string, body *bytes.Reader) *http.Request {
		var r *http.Request
		if body == nil {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, body)
		}
		return r.WithContext(ownerReassignmentRequest(t, method, path, admin, authorization).Context())
	}
	// Preserve the authenticated context while keeping a request body.
	withBody := func(method, path string, raw []byte) *http.Request {
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		ctx := ownerReassignmentRequest(t, method, path, admin, authorization).Context()
		return r.WithContext(ctx)
	}
	response := httptest.NewRecorder()
	handler.Template(response, request(http.MethodGet, "/template", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("template status=%d", response.Code)
	}
	csvBody := []byte("customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id\n41,7,2026-08-24T09:00:00Z,9\n")
	response = httptest.NewRecorder()
	create := withBody(http.MethodPost, "/previews", csvBody)
	create.Header.Set("Content-Type", "text/csv; charset=utf-8")
	create.Header.Set("Idempotency-Key", "owner-preview-key-01")
	handler.CreatePreview(response, create)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.Preview(response, request(http.MethodGet, "/previews/"+preview.ID, nil), preview.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	execute := withBody(http.MethodPost, "/execute", []byte(`{"preview_hash":"`+preview.Hash+`","confirmation":"CONFIRM OWNER REASSIGNMENT"}`))
	execute.Header.Set("Idempotency-Key", strings.Repeat("e", 16))
	handler.Execute(response, execute, preview.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("execute status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ErrorsCSV(response, request(http.MethodGet, "/errors", nil), preview.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("errors status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	handler.ResultsCSV(response, request(http.MethodGet, "/results", nil), preview.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("results status=%d", response.Code)
	}
	for _, contentType := range []string{"text/csv; charset=latin1", "text/csv; boundary=x", "application/csv"} {
		response = httptest.NewRecorder()
		bad := withBody(http.MethodPost, "/previews", csvBody)
		bad.Header.Set("Content-Type", contentType)
		bad.Header.Set("Idempotency-Key", "owner-preview-key-02")
		handler.CreatePreview(response, bad)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("content type %q status=%d", contentType, response.Code)
		}
	}
}

type ownerReassignmentHTTPFake struct {
	preview contactapp.OwnerReassignmentPreview
}

func (f *ownerReassignmentHTTPFake) CreatePreview(_ context.Context, _ int64, _ []byte, _ string) (contactapp.OwnerReassignmentPreview, error) {
	return f.preview, nil
}
func (f *ownerReassignmentHTTPFake) Preview(_ context.Context, _ int64, _ string) (contactapp.OwnerReassignmentPreview, error) {
	return f.preview, nil
}
func (f *ownerReassignmentHTTPFake) Execute(_ context.Context, _ int64, _ string, _ string, _ string, _ string) (contactapp.OwnerReassignmentPreview, error) {
	f.preview.Executed = true
	return f.preview, nil
}
func ownerReassignmentRequest(t *testing.T, method, path string, p authport.Principal, a authport.Authorization) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(r.Context(), p, "owner-reassignment-session")
	var err error
	ctx, err = authport.WithAuthorization(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	return r.WithContext(ctx)
}
