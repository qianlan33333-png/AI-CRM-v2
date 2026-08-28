package main

import (
	"context"
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
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type surveyUnresolvedHistoryAPIReader struct {
	submission surveyport.HistoricalUnresolvedSurveySubmission
	answer     surveyport.HistoricalUnresolvedSurveyAnswer
	err        error
	empty      bool
	total      int64
	calls      int
	kind       string
	id         int64
	query      surveyport.SurveyUnresolvedHistoryQuery
}

func (reader *surveyUnresolvedHistoryAPIReader) GetHistoricalUnresolvedSurveySubmission(_ context.Context, id int64) (surveyport.HistoricalUnresolvedSurveySubmission, error) {
	reader.calls++
	reader.kind, reader.id = "submission", id
	return reader.submission, reader.err
}

func (reader *surveyUnresolvedHistoryAPIReader) ListHistoricalUnresolvedSurveySubmissions(_ context.Context, query surveyport.SurveyUnresolvedHistoryQuery) ([]surveyport.HistoricalUnresolvedSurveySubmission, int64, error) {
	reader.calls++
	reader.kind, reader.query = "submissions", query
	if reader.empty {
		return nil, reader.total, reader.err
	}
	return []surveyport.HistoricalUnresolvedSurveySubmission{reader.submission}, reader.total, reader.err
}

func (reader *surveyUnresolvedHistoryAPIReader) ListHistoricalUnresolvedSurveyAnswers(_ context.Context, id int64, query surveyport.SurveyUnresolvedHistoryQuery) ([]surveyport.HistoricalUnresolvedSurveyAnswer, int64, error) {
	reader.calls++
	reader.kind, reader.id, reader.query = "answers", id, query
	if reader.empty {
		return nil, reader.total, reader.err
	}
	return []surveyport.HistoricalUnresolvedSurveyAnswer{reader.answer}, reader.total, reader.err
}

func (reader *surveyUnresolvedHistoryAPIReader) GetHistoricalUnresolvedSurveyAnswer(_ context.Context, id int64) (surveyport.HistoricalUnresolvedSurveyAnswer, error) {
	reader.calls++
	reader.kind, reader.id = "answer", id
	return reader.answer, reader.err
}

func surveyUnresolvedHistoryFixture() *surveyUnresolvedHistoryAPIReader {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	questionnaire, customer := int64(20), int64(30)
	return &surveyUnresolvedHistoryAPIReader{
		total: 1,
		submission: surveyport.HistoricalUnresolvedSurveySubmission{
			ID: 7, SourceKeyDigest: surveyUnresolvedHistoryDigest(1), SourcePayloadDigest: surveyUnresolvedHistoryDigest(2), SourceFieldDigest: surveyUnresolvedHistoryDigest(3),
			SourceID: -1, QuestionnaireSourceID: 0, QuestionnaireID: &questionnaire, CustomerID: &customer, MatchedBy: "dm01", SourceChannel: "legacy", TotalScore: -1.5, FinalTags: []byte(`[]`), SubmittedAt: at, CreatedAt: at,
			UnionIDDigest: surveyUnresolvedHistoryDigest(4), FollowUserUserIDDigest: surveyUnresolvedHistoryDigest(5), CampaignIDDigest: surveyUnresolvedHistoryDigest(6), StaffIDDigest: surveyUnresolvedHistoryDigest(7), RedirectURLDigest: surveyUnresolvedHistoryDigest(8), AssessmentResultDigest: surveyUnresolvedHistoryDigest(9),
		},
		answer: surveyport.HistoricalUnresolvedSurveyAnswer{
			ID: 8, SourceKeyDigest: surveyUnresolvedHistoryDigest(10), SourcePayloadDigest: surveyUnresolvedHistoryDigest(11), SourceFieldDigest: surveyUnresolvedHistoryDigest(12),
			SourceID: 0, SubmissionID: 7, SubmissionSourceID: -1, QuestionSourceID: 0, QuestionType: "", QuestionTitleSnapshot: "历史题目", SelectedOptionIDs: []byte(`null`), SelectedOptionTexts: []byte(`[]`), SelectedOptionScores: []byte(`[]`), SelectedOptionTags: []byte(`[]`), TextValue: "", ScoreContribution: -2.5, CreatedAt: at,
		},
	}
}

func surveyUnresolvedHistoryDigest(seed byte) [32]byte {
	var digest [32]byte
	for index := range digest {
		digest[index] = seed
	}
	return digest
}

func surveyUnresolvedHistoryRouter(t *testing.T, reader surveyport.SurveyUnresolvedHistoryReader, auth *surveyUnresolvedHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.surveyUnresolvedHistory = reader
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

func TestSurveyUnresolvedHistoryFinalRoutesReadOnly(t *testing.T) {
	reader := surveyUnresolvedHistoryFixture()
	auth := &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin}
	router := surveyUnresolvedHistoryRouter(t, reader, auth)
	for _, test := range []struct {
		path, kind, want string
	}{
		{"/api/admin/survey-history/submissions?questionnaire_id=20&limit=1&offset=0", "submissions", `"source_id":-1`},
		{"/api/admin/survey-history/submissions/7", "submission", `"total_score":-1.5`},
		{"/api/admin/survey-history/submissions/7/answers?limit=1&offset=0", "answers", `"score_contribution":-2.5`},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(101)))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		for _, want := range []string{test.want, `"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"definition_mapping":"historical_source_only"`, `"created_at":"2026-08-28T01:02:03.123456Z"`} {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s missing %s: %s", test.path, want, response.Body.String())
			}
		}
		for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "union_id_digest", "follow_user_user_id_digest", "campaign_id_digest", "staff_id_digest", "redirect_url_digest", "assessment_result_digest"} {
			if strings.Contains(response.Body.String(), private) {
				t.Fatalf("%s leaked private field %s", test.path, private)
			}
		}
		if reader.kind != test.kind {
			t.Fatalf("%s reader kind=%q", test.path, reader.kind)
		}
	}
	if reader.calls != 3 || reader.query.Limit != 1 || reader.query.Offset != 0 || auth.csrfCalls != 0 || len(auth.capabilities) != 3 {
		t.Fatalf("reader/auth mismatch: reader=%+v auth=%+v", reader, auth)
	}
	if reader.query.QuestionnaireID != nil {
		t.Fatal("answers query must not receive questionnaire filter")
	}
}

func TestSurveyUnresolvedHistoryFinalRoutesFailClosed(t *testing.T) {
	for _, path := range []string{
		"/api/admin/survey-history/submissions?limit=0",
		"/api/admin/survey-history/submissions?limit=101",
		"/api/admin/survey-history/submissions?offset=-1",
		"/api/admin/survey-history/submissions?questionnaire_id=20&questionnaire_id=21",
		"/api/admin/survey-history/submissions?unknown=1",
		"/api/admin/survey-history/submissions/0",
		"/api/admin/survey-history/submissions/01",
		"/api/admin/survey-history/submissions/7?limit=1",
		"/api/admin/survey-history/submissions/7/answers?questionnaire_id=20",
	} {
		reader := surveyUnresolvedHistoryFixture()
		response := httptest.NewRecorder()
		surveyUnresolvedHistoryRouter(t, reader, &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if response.Code != http.StatusBadRequest || reader.calls != 0 {
			t.Fatalf("invalid path=%s status=%d calls=%d", path, response.Code, reader.calls)
		}
	}

	for _, state := range []string{"downstream", "missing digest", "wrong detail", "cross questionnaire", "cross submission", "wrong count"} {
		t.Run(state, func(t *testing.T) {
			reader := surveyUnresolvedHistoryFixture()
			path := "/api/admin/survey-history/submissions?limit=1&offset=0"
			switch state {
			case "downstream":
				reader.err = errors.New("private downstream detail")
			case "missing digest":
				reader.submission.SourceKeyDigest = [32]byte{}
			case "wrong detail":
				path, reader.submission.ID = "/api/admin/survey-history/submissions/7", 8
			case "cross questionnaire":
				other := int64(21)
				path, reader.submission.QuestionnaireID = "/api/admin/survey-history/submissions?questionnaire_id=20&limit=1&offset=0", &other
			case "cross submission":
				path, reader.answer.SubmissionID = "/api/admin/survey-history/submissions/7/answers?limit=1&offset=0", 8
			case "wrong count":
				reader.total = 0
			}
			response := httptest.NewRecorder()
			surveyUnresolvedHistoryRouter(t, reader, &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(101)))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private downstream detail") {
				t.Fatalf("%s status=%d body=%s", state, response.Code, response.Body.String())
			}
		})
	}

	reader := surveyUnresolvedHistoryFixture()
	reader.empty, reader.total = true, 0
	response := httptest.NewRecorder()
	surveyUnresolvedHistoryRouter(t, reader, &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/survey-history/submissions?limit=1&offset=0", legacyToken(101)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("empty page status=%d body=%s", response.Code, response.Body.String())
	}
	for _, missing := range []surveyport.SurveyUnresolvedHistoryReader{nil, (*surveyUnresolvedHistoryAPIReader)(nil)} {
		response := httptest.NewRecorder()
		surveyUnresolvedHistoryRouter(t, missing, &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/survey-history/submissions", legacyToken(101)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("missing reader status=%d", response.Code)
		}
	}
}

func TestSurveyUnresolvedHistoryFinalRoutesAuthorizationAndNoWrites(t *testing.T) {
	for _, test := range []struct {
		role  authport.Role
		token string
		want  int
	}{
		{authport.RoleAdmin, "", http.StatusUnauthorized},
		{authport.Role("ops"), legacyToken(101), http.StatusForbidden},
	} {
		for _, path := range []string{"/api/admin/survey-history/submissions", "/api/admin/survey-history/submissions/7", "/api/admin/survey-history/submissions/7/answers"} {
			reader := surveyUnresolvedHistoryFixture()
			response := httptest.NewRecorder()
			surveyUnresolvedHistoryRouter(t, reader, &surveyUnresolvedHistoryAPIAuth{role: test.role}).ServeHTTP(response, legacyRequest(http.MethodGet, path, test.token))
			if response.Code != test.want || reader.calls != 0 {
				t.Fatalf("auth path=%s status=%d calls=%d", path, response.Code, reader.calls)
			}
		}
	}

	reader := surveyUnresolvedHistoryFixture()
	router := surveyUnresolvedHistoryRouter(t, reader, &surveyUnresolvedHistoryAPIAuth{role: authport.RoleAdmin})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(method, "/api/admin/survey-history/submissions", legacyToken(101)))
		if response.Code < http.StatusBadRequest || reader.calls != 0 {
			t.Fatalf("write method=%s status=%d calls=%d", method, response.Code, reader.calls)
		}
	}
}

type surveyUnresolvedHistoryAPIAuth struct {
	role         authport.Role
	csrfCalls    int
	capabilities []authport.Capability
}

func (auth *surveyUnresolvedHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: auth.role}, nil
}

func (auth *surveyUnresolvedHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.Role != authport.RoleAdmin || capability != authport.CapabilityQuestionnairesRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	auth.capabilities = append(auth.capabilities, capability)
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (auth *surveyUnresolvedHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	auth.csrfCalls++
	return nil
}

func (*surveyUnresolvedHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

var _ surveyport.SurveyUnresolvedHistoryReader = (*surveyUnresolvedHistoryAPIReader)(nil)
var _ authport.Service = (*surveyUnresolvedHistoryAPIAuth)(nil)
