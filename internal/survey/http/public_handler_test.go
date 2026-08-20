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

func TestPublicSubmitRejectsNonJSONAndUnknownFields(t *testing.T) {
	var key [32]byte
	key[0] = 1
	h := NewPublicHandler(fakeService{}, key, key)
	for _, tc := range []struct{ contentType, body string }{{"text/plain", `{}`}, {"application/json", `{"version":1,"unknown":true}`}} {
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
