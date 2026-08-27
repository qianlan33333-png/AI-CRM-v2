package surveyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type fakeService struct{ token string }
type captureService struct {
	fakeService
	commands []surveyport.PublicSubmissionCommand
}
type analyticsCaptureService struct {
	fakeService
	version int64
	err     error
}

func (f *analyticsCaptureService) Analytics(_ context.Context, _ surveyport.ID, version int64) (surveyport.PublicAnalytics, error) {
	f.version = version
	return surveyport.PublicAnalytics{QuestionnaireID: 1, DefinitionVersion: 2, Slug: "public", State: "public", Questions: []surveyport.PublicAnalyticsQuestion{}}, f.err
}

func (f *captureService) Submit(ctx context.Context, in surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionReceipt, string, error) {
	f.commands = append(f.commands, in)
	return f.fakeService.Submit(ctx, in)
}

func (f fakeService) Definition(context.Context, string) (surveyport.PublicQuestionnaire, error) {
	return surveyport.PublicQuestionnaire{ID: 1, Slug: "public", Title: "匿名问卷", Version: 1, Questions: []surveyport.PublicQuestion{}}, nil
}
func (f fakeService) Submit(_ context.Context, in surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionReceipt, string, error) {
	if in.AnonymousDigest == [32]byte{} {
		return surveyport.PublicSubmissionReceipt{}, "", surveyapp.ErrInvalidPublicInput
	}
	return surveyport.PublicSubmissionReceipt{QuestionnaireID: 1, DefinitionVersion: 1, SubmissionID: 9}, f.token, nil
}
func (f fakeService) Result(context.Context, string) (surveyport.PublicSubmissionResult, error) {
	return surveyport.PublicSubmissionResult{SubmissionID: 9, DefinitionVersion: 1, SubmittedAt: time.Now(), LocalOnly: true}, nil
}
func (f fakeService) Publish(context.Context, surveyport.PublishPublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error) {
	return surveyapp.PublicDefinitionRecord{}, nil
}
func (f fakeService) Disable(context.Context, surveyport.DisablePublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error) {
	return surveyapp.PublicDefinitionRecord{}, nil
}
func (f fakeService) Analytics(context.Context, surveyport.ID, int64) (surveyport.PublicAnalytics, error) {
	return surveyport.PublicAnalytics{}, nil
}
func TestPublicSubmitCreatesHardenedAnonymousCookie(t *testing.T) {
	var key [32]byte
	key[0] = 1
	h := NewPublicHandler(fakeService{token: strings.Repeat("a", 43)}, key, key)
	r := httptest.NewRequest(http.MethodPost, "/api/public/questionnaires/public/submissions", strings.NewReader(`{"version":1,"submission_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","answers":[]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Submit(w, r, "public")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	c := w.Result().Cookies()
	if len(c) != 1 || !c[0].HttpOnly || !c[0].Secure || c[0].SameSite != http.SameSiteLaxMode || c[0].Path != "/api/public" || len(c[0].Value) != 43 {
		t.Fatalf("unsafe cookie=%+v", c)
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers=%v", w.Header())
	}
}
func TestPublicResultTokenNeverAcceptedInURL(t *testing.T) {
	var key [32]byte
	key[0] = 1
	h := NewPublicHandler(fakeService{}, key, key)
	r := httptest.NewRequest(http.MethodGet, "/api/public/survey-submission-results/query?result_token=x", nil)
	w := httptest.NewRecorder()
	h.QueryResult(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPublicAnalyticsAcceptsZeroVersionForCurrentSnapshot(t *testing.T) {
	service := &analyticsCaptureService{}
	h := NewPublicHandler(service, [32]byte{}, [32]byte{})
	w := httptest.NewRecorder()
	h.Analytics(w, httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/1/public-analytics", nil), 1, 0)
	if w.Code != http.StatusOK || service.version != 0 {
		t.Fatalf("status=%d version=%d", w.Code, service.version)
	}
	service.err = surveyapp.ErrNotFound
	w = httptest.NewRecorder()
	h.Analytics(w, httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/1/public-analytics", nil), 1, 0)
	if w.Code != http.StatusNotFound {
		t.Fatalf("no current public definition status=%d", w.Code)
	}
}

func TestPublicCarrierRejectsUnsafeSlugWithoutRedirectBody(t *testing.T) {
	h := NewPublicHandler(fakeService{}, [32]byte{}, [32]byte{})
	for _, slug := range []string{"Public", "public survey", "public_1", "-public"} {
		w := httptest.NewRecorder()
		h.Page(w, httptest.NewRequest(http.MethodGet, "/q/unsafe", nil), slug)
		if w.Code != http.StatusBadRequest || w.Header().Get("Location") != "" {
			t.Fatalf("slug=%q status=%d location=%q", slug, w.Code, w.Header().Get("Location"))
		}
	}
	w := httptest.NewRecorder()
	h.Page(w, httptest.NewRequest(http.MethodGet, "/q/public-1", nil), "public-1")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/h5/all.html?slug=public-1" || w.Body.Len() != 0 {
		t.Fatalf("status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
}

func TestPublicSubmitRejectsNonJSONAndUnknownFields(t *testing.T) {
	var key [32]byte
	key[0] = 1
	h := NewPublicHandler(fakeService{}, key, key)
	for _, tc := range []struct{ contentType, body string }{{"text/plain", `{}`}, {"application/jsonp", `{}`}, {"application/json", `{"version":1,"unknown":true}`}} {
		r := httptest.NewRequest(http.MethodPost, "/api/public/questionnaires/public/submissions", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.contentType)
		w := httptest.NewRecorder()
		h.Submit(w, r, "public")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", tc.contentType, w.Code)
		}
	}
}

func TestPublicSubmitFailsClosedWithoutDigestKey(t *testing.T) {
	h := NewPublicHandler(fakeService{}, [32]byte{}, [32]byte{})
	r := httptest.NewRequest(http.MethodPost, "/api/public/questionnaires/public/submissions", strings.NewReader(`{"version":1,"submission_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","answers":[]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Submit(w, r, "public")
	if w.Code != http.StatusServiceUnavailable || len(w.Result().Cookies()) != 0 {
		t.Fatalf("status/cookies=%d/%v", w.Code, w.Result().Cookies())
	}
}
func TestPublicManagementRequiresOneExactIdempotencyKey(t *testing.T) {
	h := NewPublicHandler(fakeService{}, [32]byte{}, [32]byte{})
	for _, values := range [][]string{nil, {"short"}, {" repeated-survey-key ", "other-survey-key"}} {
		r := httptest.NewRequest(http.MethodPost, "/api/admin/questionnaires/1/public-publish", strings.NewReader(`{"expected_questionnaire_version":1}`))
		r.Header.Set("Content-Type", "application/json")
		for _, value := range values {
			r.Header.Add("Idempotency-Key", value)
		}
		w := httptest.NewRecorder()
		h.Publish(w, r, 1, 1)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("values=%v status=%d", values, w.Code)
		}
	}
}
func TestPublicSubmitIgnoresForwardedForAndDerivesDistinctSourceDigest(t *testing.T) {
	var key [32]byte
	key[0] = 1
	service := &captureService{fakeService: fakeService{token: strings.Repeat("a", 43)}}
	h := NewPublicHandler(service, key, key)
	var cookie *http.Cookie
	for index, remote := range []string{"198.51.100.8:443", "198.51.100.9:443"} {
		r := httptest.NewRequest(http.MethodPost, "/api/public/questionnaires/public/submissions", strings.NewReader(`{"version":1,"submission_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","answers":[]}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", "203.0.113.99")
		r.RemoteAddr = remote
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.Submit(w, r, "public")
		if w.Code != http.StatusAccepted {
			t.Fatalf("remote=%s status=%d", remote, w.Code)
		}
		if index == 0 {
			cookie = w.Result().Cookies()[0]
		}
	}
	if len(service.commands) != 2 || service.commands[0].AnonymousDigest != service.commands[1].AnonymousDigest || service.commands[0].RateDigest == service.commands[1].RateDigest {
		t.Fatalf("cookie/source digests not isolated")
	}
}

func TestPublicSourceAddressTrustsOnlyLoopbackProxyRightMostHop(t *testing.T) {
	direct := httptest.NewRequest(http.MethodPost, "/", nil)
	direct.RemoteAddr = "198.51.100.8:443"
	direct.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got, err := controlledRemoteIP(direct); err != nil || got.String() != "198.51.100.8" {
		t.Fatalf("direct client accepted spoofed forwarding header: %v %v", got, err)
	}
	proxied := httptest.NewRequest(http.MethodPost, "/", nil)
	proxied.RemoteAddr = "127.0.0.1:43210"
	proxied.Header.Set("X-Forwarded-For", "198.51.100.200, 203.0.113.9")
	if got, err := controlledRemoteIP(proxied); err != nil || got.String() != "203.0.113.9" {
		t.Fatalf("trusted proxy did not select its appended right-most hop: %v %v", got, err)
	}
	for _, forwarded := range []string{"", "not-an-ip", "203.0.113.9, "} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.RemoteAddr = "[::1]:43210"
		if forwarded != "" {
			request.Header.Set("X-Forwarded-For", forwarded)
		}
		if _, err := controlledRemoteIP(request); err == nil {
			t.Fatalf("trusted proxy forwarding value %q was accepted", forwarded)
		}
	}
}
