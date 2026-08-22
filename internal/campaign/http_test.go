package campaign

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type authorizerSpy struct {
	actor Actor
	err   error
	calls []AccessRequirement
}

func (s *authorizerSpy) Authorize(_ *http.Request, requirement AccessRequirement) (Actor, error) {
	s.calls = append(s.calls, requirement)
	return s.actor, s.err
}

type csrfSpy struct {
	err   error
	calls int
}

func (s *csrfSpy) Verify(_ *http.Request, _ Actor) error { s.calls++; return s.err }

func TestRouteFragmentReadWriteSecurityAndMethodClosure(t *testing.T) {
	svc, store := testService(t, testCampaign("spring", ApprovalDraft, RuntimeIdle, 1))
	store.SeedSteps("spring", []Step{{Index: 1, DelayMinutes: 0, Content: "hi"}})
	auth := &authorizerSpy{actor: Actor{ID: 11}}
	csrf := &csrfSpy{}
	handler, err := NewRouteFragment(svc, auth, csrf)
	if err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, RoutePrefix+"/spring", nil))
	if read.Code != http.StatusOK || len(auth.calls) != 1 || auth.calls[0].Capability != CapabilityAdminRead || csrf.calls != 0 {
		t.Fatalf("read status=%d calls=%+v csrf=%d body=%s", read.Code, auth.calls, csrf.calls, read.Body.String())
	}
	bad := httptest.NewRecorder()
	badRequest := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring/approve", strings.NewReader(`{"expected_version":1,"unknown":true}`))
	badRequest.Header.Set("Content-Type", "application/json")
	badRequest.Header.Set("Idempotency-Key", "key-http-approve1")
	handler.ServeHTTP(bad, badRequest)
	if bad.Code != http.StatusBadRequest || csrf.calls != 1 {
		t.Fatalf("bad status=%d csrf=%d", bad.Code, csrf.calls)
	}
	approve := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring/approve", strings.NewReader(`{"expected_version":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "key-http-approve2")
	handler.ServeHTTP(approve, request)
	if approve.Code != http.StatusOK || csrf.calls != 2 {
		t.Fatalf("approve status=%d csrf=%d body=%s", approve.Code, csrf.calls, approve.Body.String())
	}
	wrong := httptest.NewRecorder()
	handler.ServeHTTP(wrong, httptest.NewRequest(http.MethodPatch, RoutePrefix+"/spring/approve", nil))
	if wrong.Code != http.StatusMethodNotAllowed || wrong.Header().Get("Allow") != "POST" {
		t.Fatalf("wrong=%d allow=%s", wrong.Code, wrong.Header().Get("Allow"))
	}
}

func TestRouteFragmentFailsClosedForPermissionAndCSRF(t *testing.T) {
	svc, _ := testService(t, testCampaign("spring", ApprovalDraft, RuntimeIdle, 1))
	unauth, _ := NewRouteFragment(svc, &authorizerSpy{err: ErrUnauthenticated}, &csrfSpy{})
	rec := httptest.NewRecorder()
	unauth.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, RoutePrefix, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", rec.Code)
	}
	csrf := &csrfSpy{err: ErrCSRFInvalid}
	handler, _ := NewRouteFragment(svc, &authorizerSpy{actor: Actor{ID: 1}}, csrf)
	rec = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring/reject", strings.NewReader(`{"expected_version":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "key-http-csrf0001")
	handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden || csrf.calls != 1 {
		t.Fatalf("csrf=%d calls=%d", rec.Code, csrf.calls)
	}
}

func TestRouteFragmentListStatusFiltersAreClosed(t *testing.T) {
	svc, _ := testService(t,
		testCampaign("draft", ApprovalDraft, RuntimeIdle, 1),
		testCampaign("approved", ApprovalApproved, RuntimeIdle, 1),
	)
	handler, _ := NewRouteFragment(svc, &authorizerSpy{actor: Actor{ID: 1}}, &csrfSpy{})
	filtered := httptest.NewRecorder()
	handler.ServeHTTP(filtered, httptest.NewRequest(http.MethodGet, RoutePrefix+"?approval_status=approved&runtime_status=idle", nil))
	if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), `"campaign_code":"approved"`) || strings.Contains(filtered.Body.String(), `"campaign_code":"draft"`) {
		t.Fatalf("filtered status=%d body=%s", filtered.Code, filtered.Body.String())
	}
	duplicate := httptest.NewRecorder()
	handler.ServeHTTP(duplicate, httptest.NewRequest(http.MethodGet, RoutePrefix+"?approval_status=draft&approval_status=approved", nil))
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status=%d", duplicate.Code)
	}
}
