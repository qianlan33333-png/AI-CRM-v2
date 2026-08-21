package safeadminhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type applicationStub struct {
	analysisFn func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error)
	previewFn  func(context.Context, surveyport.ID, surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error)
}

func (s applicationStub) SafeAnalysis(ctx context.Context, id surveyport.ID, limit, offset int32) (surveyport.SafeSubmissionAnalysis, error) {
	if s.analysisFn == nil {
		return surveyport.SafeSubmissionAnalysis{}, errors.New("unexpected SafeAnalysis")
	}
	return s.analysisFn(ctx, id, limit, offset)
}
func (s applicationStub) SafeExportPreview(ctx context.Context, id surveyport.ID, request surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error) {
	if s.previewFn == nil {
		return surveyport.SafeExportPreview{}, errors.New("unexpected SafeExportPreview")
	}
	return s.previewFn(ctx, id, request)
}

func TestResultsAllowsAdminAndOpsWithGlobalQuestionnaireRead(t *testing.T) {
	t.Parallel()
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			app := applicationStub{analysisFn: func(_ context.Context, id surveyport.ID, limit, offset int32) (surveyport.SafeSubmissionAnalysis, error) {
				if id != 17 || limit != 2 || offset != 1 {
					t.Fatalf("unexpected query id=%d limit=%d offset=%d", id, limit, offset)
				}
				return surveyport.SafeSubmissionAnalysis{
					OK: true, QuestionnaireID: id, Questions: []surveyport.SafeEnumQuestionAggregate{},
					Limit: limit, Offset: offset, AggregationComplete: true, Deidentified: true, LocalOnly: true,
				}, nil
			}}
			recorder := httptest.NewRecorder()
			request := authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/17/analysis?limit=2&offset=1", nil, role)
			New(app).Routes().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			assertSafetyFlags(t, recorder.Body.Bytes())
			if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control = %q", got)
			}
		})
	}
}

func TestResultsDirectHandlerCallDoesNotDependOnRouterPathValues(t *testing.T) {
	t.Parallel()
	app := applicationStub{analysisFn: func(_ context.Context, id surveyport.ID, limit, offset int32) (surveyport.SafeSubmissionAnalysis, error) {
		if id != 7 || limit != surveyapp.SafeAnalysisDefaultQuestionLimit || offset != 0 {
			t.Fatalf("direct handler request = %d/%d/%d", id, limit, offset)
		}
		return surveyport.SafeSubmissionAnalysis{
			OK: true, QuestionnaireID: id, Questions: []surveyport.SafeEnumQuestionAggregate{},
			Limit: limit, Offset: offset, AggregationComplete: true, Deidentified: true, LocalOnly: true,
		}, nil
	}}
	request := authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleAdmin)
	recorder := httptest.NewRecorder()
	New(app).Results(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("direct handler status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	assertSafetyFlags(t, recorder.Body.Bytes())
}

func TestResultsPermissionAndVisibilityErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request *http.Request
		app     Application
		status  int
		code    string
	}{
		{"unauthenticated", httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/7/analysis", nil), applicationStub{}, http.StatusUnauthorized, "authentication_required"},
		{"sales forbidden", authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleSales), applicationStub{}, http.StatusForbidden, "permission_denied"},
		{"not found", authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleAdmin), applicationStub{analysisFn: func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error) {
			return surveyport.SafeSubmissionAnalysis{}, surveyapp.ErrNotFound
		}}, http.StatusNotFound, "questionnaire_not_found"},
		{"unavailable", authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleOps), applicationStub{analysisFn: func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error) {
			return surveyport.SafeSubmissionAnalysis{}, surveyapp.ErrUnavailable
		}}, http.StatusServiceUnavailable, "survey_read_unavailable"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			New(test.app).Routes().ServeHTTP(recorder, test.request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertSafetyFlags(t, recorder.Body.Bytes())
		})
	}
}

func TestCanonicalQuestionnaireIDParser(t *testing.T) {
	t.Parallel()
	valid := map[string]surveyport.ID{"1": 1, "9223372036854775807": surveyport.ID(9223372036854775807)}
	for raw, want := range valid {
		got, err := ParseQuestionnaireID(raw)
		if err != nil || got != want {
			t.Fatalf("ParseQuestionnaireID(%q) = %d, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "0", "-1", "+1", "01", " 1", "1 ", "1/2", `1\\2`, "%2F", "9223372036854775808", "１"} {
		if _, err := ParseQuestionnaireID(raw); err == nil {
			t.Fatalf("ParseQuestionnaireID(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestRoutesRejectEncodedOrAlternateQuestionnairePaths(t *testing.T) {
	t.Parallel()
	app := applicationStub{analysisFn: func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error) {
		t.Fatal("application must not be called for a non-canonical path")
		return surveyport.SafeSubmissionAnalysis{}, nil
	}}
	for _, target := range []string{
		"/api/admin/questionnaires/%37/analysis",
		"/api/admin/questionnaires/7%2F8/analysis",
		`/api/admin/questionnaires/7%5C8/analysis`,
	} {
		request := authorizedRequest(t, http.MethodGet, target, nil, authport.RoleAdmin)
		recorder := httptest.NewRecorder()
		New(app).Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
			t.Fatalf("target %q status = %d, body = %s", target, recorder.Code, recorder.Body.String())
		}
		if recorder.Code == http.StatusBadRequest {
			assertSafetyFlags(t, recorder.Body.Bytes())
		}
	}
}

func TestAnalysisQueryParserIsClosed(t *testing.T) {
	t.Parallel()
	limit, offset, err := ParseAnalysisQuery("")
	if err != nil || limit != surveyapp.SafeAnalysisDefaultQuestionLimit || offset != 0 {
		t.Fatalf("defaults = %d/%d, %v", limit, offset, err)
	}
	limit, offset, err = ParseAnalysisQuery("limit=100&offset=1000000")
	if err != nil || limit != 100 || offset != 1_000_000 {
		t.Fatalf("boundary = %d/%d, %v", limit, offset, err)
	}
	for _, raw := range []string{
		"unknown=1", "limit=", "offset=", "limit=1&limit=2", "offset=1&offset=2",
		"limit=00", "limit=01", "limit=+1", "limit=-1", "limit=101", "offset=1000001",
		"offset=1.0", "offset=%", "limit=１", "limit=1&", "&limit=1", "limit=1&&offset=0",
		"limit=%31", "l%69mit=1", "limit=1;offset=0",
	} {
		if _, _, err := ParseAnalysisQuery(raw); err == nil {
			t.Fatalf("ParseAnalysisQuery(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestPreviewStrictParserAndSafeSuccess(t *testing.T) {
	t.Parallel()
	app := applicationStub{previewFn: func(_ context.Context, id surveyport.ID, request surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error) {
		if id != 7 || request.Limit != 2 || request.Offset != 1 || len(request.Fields) != 2 || request.Fields[0] != surveyport.SafePreviewRowNumber || request.Fields[1] != surveyport.SafePreviewChoiceOptionIDs {
			t.Fatalf("unexpected request: id=%d request=%#v", id, request)
		}
		rowNumber := int64(2)
		choices := []surveyport.SafeChoiceAnswerPreview{{QuestionID: 10, QuestionType: surveyport.SingleChoice, SortOrder: 0, OptionIDs: []int64{101}}}
		return surveyport.SafeExportPreview{
			OK: true, QuestionnaireID: id, Fields: request.Fields,
			Rows:  []surveyport.SafeExportPreviewRow{{RowNumber: &rowNumber, ChoiceOptionIDs: &choices}},
			Total: 2, Limit: 2, Offset: 1, HasMore: false, FileCreated: false, Deidentified: true,
			ContainsRawIdentity: false, ContainsFreeText: false, LocalOnly: true, RealExternalCallExecuted: false,
		}, nil
	}}
	body := `{"fields":["row_number","choice_option_ids"],"limit":2,"offset":1}`
	request := authorizedRequest(t, http.MethodPost, "/api/admin/questionnaires/7/export/preview", strings.NewReader(body), authport.RoleOps)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	recorder := httptest.NewRecorder()
	New(app).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertSafetyFlags(t, recorder.Body.Bytes())
	for _, forbidden := range []string{"external_userid", "unionid", "openid", "mobile", "respondent", "text_value", "option_text", "question_title", "raw_payload", "provider_receipt"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("response contains forbidden field %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestPreviewParserRejectsOpenOrUnsafePayloads(t *testing.T) {
	t.Parallel()
	payloads := []struct {
		name        string
		body        string
		contentType string
	}{
		{"empty", "", "application/json"},
		{"array", `[]`, "application/json"},
		{"unknown key", `{"x":1}`, "application/json"},
		{"duplicate key", `{"limit":1,"limit":2}`, "application/json"},
		{"null fields", `{"fields":null}`, "application/json"},
		{"unsafe identity field", `{"fields":["external_userid"]}`, "application/json"},
		{"unsafe answers field", `{"fields":["answers"]}`, "application/json"},
		{"duplicate field", `{"fields":["score","score"]}`, "application/json"},
		{"too many rows", `{"limit":4}`, "application/json"},
		{"decimal", `{"limit":1.0}`, "application/json"},
		{"exponent", `{"limit":1e0}`, "application/json"},
		{"leading zero invalid JSON", `{"limit":01}`, "application/json"},
		{"negative offset", `{"offset":-1}`, "application/json"},
		{"trailing", `{} {}`, "application/json"},
		{"invalid utf8", "{\"fields\":[\"\xff\"]}", "application/json"},
		{"wrong content type", `{}`, "text/plain"},
	}
	for _, test := range payloads {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := authorizedRequest(t, http.MethodPost, "/api/admin/questionnaires/7/export/preview", strings.NewReader(test.body), authport.RoleAdmin)
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			New(applicationStub{}).Routes().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_export_preview"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertSafetyFlags(t, recorder.Body.Bytes())
		})
	}
}

func TestPreviewBodyLimitMethodAndExtraSegment(t *testing.T) {
	t.Parallel()
	oversized := `{"fields":[] ,"padding":"` + strings.Repeat("x", maximumPreviewBodyBytes) + `"}`
	request := authorizedRequest(t, http.MethodPost, "/api/admin/questionnaires/7/export/preview", strings.NewReader(oversized), authport.RoleAdmin)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	New(applicationStub{}).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized status = %d", recorder.Code)
	}

	request = authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/export/preview", nil, authport.RoleAdmin)
	recorder = httptest.NewRecorder()
	New(applicationStub{}).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method status/allow = %d/%q", recorder.Code, recorder.Header().Get("Allow"))
	}
	assertSafetyFlags(t, recorder.Body.Bytes())

	request = authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis/extra", nil, authport.RoleAdmin)
	recorder = httptest.NewRecorder()
	New(applicationStub{}).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("extra segment status = %d", recorder.Code)
	}
}

func TestPreviewErrorMapping(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", surveyapp.ErrNotFound, http.StatusNotFound, "questionnaire_not_found"},
		{"unavailable", surveyapp.ErrUnavailable, http.StatusServiceUnavailable, "survey_read_unavailable"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app := applicationStub{previewFn: func(context.Context, surveyport.ID, surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error) {
				return surveyport.SafeExportPreview{}, test.err
			}}
			request := authorizedRequest(t, http.MethodPost, "/api/admin/questionnaires/7/export/preview", strings.NewReader(`{}`), authport.RoleOps)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			New(app).Routes().ServeHTTP(recorder, request)
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertSafetyFlags(t, recorder.Body.Bytes())
		})
	}
}

func TestDecodePreviewRequestDefaults(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	got, err := DecodePreviewRequest(request)
	if err != nil || got.Limit != surveyapp.SafeExportPreviewDefaultLimit || got.Offset != 0 || len(got.Fields) != 0 {
		t.Fatalf("defaults = %#v, %v", got, err)
	}
}

func authorizedRequest(t *testing.T, method, target string, body *strings.Reader, role authport.Role) *http.Request {
	t.Helper()
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	principal := authport.Principal{AdminUserID: 41, Role: role}
	ctx := authport.WithAuthenticatedSession(request.Context(), principal, authport.SessionRef("session"))
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityQuestionnairesRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

func assertSafetyFlags(t *testing.T, body []byte) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", body, err)
	}
	if local, ok := value["local_only"].(bool); !ok || !local {
		t.Fatalf("local_only missing/false: %s", body)
	}
	if executed, ok := value["real_external_call_executed"].(bool); !ok || executed {
		t.Fatalf("real_external_call_executed missing/true: %s", body)
	}
}

func TestResultsRequiresAuthorizationContextAndReadyApplication(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/7/analysis", nil)
	principal := authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin}
	request = request.WithContext(authport.WithAuthenticatedSession(request.Context(), principal, authport.SessionRef("session")))
	recorder := httptest.NewRecorder()
	New(applicationStub{}).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"permission_denied"`) {
		t.Fatalf("missing authorization status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	assertSafetyFlags(t, recorder.Body.Bytes())

	request = authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleAdmin)
	recorder = httptest.NewRecorder()
	New(nil).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"survey_read_unavailable"`) {
		t.Fatalf("nil app status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	assertSafetyFlags(t, recorder.Body.Bytes())
}

func TestPreviewRejectsQueryAndNonUTF8MediaParameters(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		target      string
		contentType string
	}{
		{"query", "/api/admin/questionnaires/7/export/preview?limit=1", "application/json"},
		{"wrong charset", "/api/admin/questionnaires/7/export/preview", "application/json; charset=iso-8859-1"},
		{"unknown parameter", "/api/admin/questionnaires/7/export/preview", "application/json; profile=safe"},
		{"multiple parameters", "/api/admin/questionnaires/7/export/preview", "application/json; charset=utf-8; profile=safe"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := authorizedRequest(t, http.MethodPost, test.target, strings.NewReader(`{}`), authport.RoleAdmin)
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			New(applicationStub{}).Routes().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_export_preview"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertSafetyFlags(t, recorder.Body.Bytes())
		})
	}
}

func TestHandlersRejectUnsafeApplicationProjections(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	t.Run("analysis effect flag", func(t *testing.T) {
		t.Parallel()
		app := applicationStub{analysisFn: func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error) {
			return surveyport.SafeSubmissionAnalysis{
				OK: true, QuestionnaireID: 7,
				Stats:     surveyport.SafeSubmissionStats{SubmissionCount: 1, LatestSubmittedAt: &stamp, AverageScore: 1},
				Questions: []surveyport.SafeEnumQuestionAggregate{}, Limit: 50, Offset: 0,
				ScannedSubmissionCount: 1, AggregationComplete: true, Deidentified: true, LocalOnly: true,
				RealExternalCallExecuted: true,
			}, nil
		}}
		request := authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleAdmin)
		recorder := httptest.NewRecorder()
		New(app).Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("unsafe analysis status/body = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSafetyFlags(t, recorder.Body.Bytes())
	})

	t.Run("analysis open enum", func(t *testing.T) {
		t.Parallel()
		app := applicationStub{analysisFn: func(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error) {
			return surveyport.SafeSubmissionAnalysis{
				OK: true, QuestionnaireID: 7,
				Stats: surveyport.SafeSubmissionStats{SubmissionCount: 1, LatestSubmittedAt: &stamp, AverageScore: 1},
				Questions: []surveyport.SafeEnumQuestionAggregate{{
					QuestionID: 1, QuestionType: surveyport.QuestionType("respondent-secret"), SortOrder: 0,
					AnsweredCount: 1, Options: []surveyport.SafeEnumOptionAggregate{{OptionID: 1, SelectionCount: 1}},
				}},
				TotalQuestions: 1, Limit: 50, Offset: 0, ScannedSubmissionCount: 1,
				AggregationComplete: true, Deidentified: true, LocalOnly: true,
			}, nil
		}}
		request := authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis", nil, authport.RoleOps)
		recorder := httptest.NewRecorder()
		New(app).Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "respondent-secret") {
			t.Fatalf("open enum status/body = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSafetyFlags(t, recorder.Body.Bytes())
	})

	t.Run("preview file created", func(t *testing.T) {
		t.Parallel()
		app := applicationStub{previewFn: func(context.Context, surveyport.ID, surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error) {
			return surveyport.SafeExportPreview{
				OK: true, QuestionnaireID: 7,
				Fields: []surveyport.SafeExportPreviewField{surveyport.SafePreviewRowNumber, surveyport.SafePreviewSubmittedAt, surveyport.SafePreviewScore, surveyport.SafePreviewChoiceOptionIDs},
				Rows:   []surveyport.SafeExportPreviewRow{}, Total: 0, Limit: 3, Offset: 0,
				FileCreated: true, Deidentified: true, LocalOnly: true,
			}, nil
		}}
		request := authorizedRequest(t, http.MethodPost, "/api/admin/questionnaires/7/export/preview", strings.NewReader(`{}`), authport.RoleAdmin)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		New(app).Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("unsafe preview status/body = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSafetyFlags(t, recorder.Body.Bytes())
	})
}

func TestResultsAcceptsClosedNonEmptyAggregateProjection(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, time.August, 21, 2, 0, 0, 0, time.UTC)
	app := applicationStub{analysisFn: func(_ context.Context, id surveyport.ID, limit, offset int32) (surveyport.SafeSubmissionAnalysis, error) {
		return surveyport.SafeSubmissionAnalysis{
			OK: true, QuestionnaireID: id,
			Stats: surveyport.SafeSubmissionStats{SubmissionCount: 2, LatestSubmittedAt: &stamp, AverageScore: 7.5},
			Questions: []surveyport.SafeEnumQuestionAggregate{{
				QuestionID: 10, QuestionType: surveyport.SingleChoice, SortOrder: 0, AnsweredCount: 2,
				Options: []surveyport.SafeEnumOptionAggregate{{OptionID: 101, SelectionCount: 1}, {OptionID: 102, SelectionCount: 1}},
			}},
			TotalQuestions: 1, Limit: limit, Offset: offset, ScannedSubmissionCount: 2,
			AggregationComplete: true, Deidentified: true, LocalOnly: true,
		}, nil
	}}
	request := authorizedRequest(t, http.MethodGet, "/api/admin/questionnaires/7/analysis?limit=1&offset=0", nil, authport.RoleAdmin)
	recorder := httptest.NewRecorder()
	New(app).Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	assertSafetyFlags(t, recorder.Body.Bytes())
}
