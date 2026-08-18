package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type legacySubmissionStub struct {
	result       surveyport.SubmissionResult
	page         surveyport.SubmissionPage
	download     surveyport.SubmissionCSVDownload
	err          error
	listLimit    int32
	listOffset   int32
	exportCalled bool
}

func (stub *legacySubmissionStub) Results(context.Context, surveyport.ID) (surveyport.SubmissionResult, error) {
	return stub.result, stub.err
}

func (stub *legacySubmissionStub) List(_ context.Context, _ surveyport.ID, limit, offset int32) (surveyport.SubmissionPage, error) {
	stub.listLimit, stub.listOffset = limit, offset
	stub.page.Limit, stub.page.Offset = limit, offset
	return stub.page, stub.err
}

func (stub *legacySubmissionStub) Export(context.Context, surveyport.ID) (surveyport.SubmissionCSVDownload, error) {
	stub.exportCalled = true
	return stub.download, stub.err
}

func TestF03QuestionnaireResultsRouteEnvelopeAndCapability(t *testing.T) {
	latest := time.Date(2026, 8, 16, 9, 2, 3, 0, time.UTC)
	stub := &legacySubmissionStub{result: surveyport.SubmissionResult{
		QuestionnaireID: 41, SubmissionCount: 2, LatestSubmittedAt: latest, AverageScore: 7.5, Rules: []surveyport.ScoreRule{},
	}}
	router, auth := legacySubmissionRouter(t, &recordingAuth{}, stub)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/results", legacyToken(81)))
	if response.Code != http.StatusOK {
		t.Fatalf("results status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if json.NewDecoder(response.Body).Decode(&body) != nil {
		t.Fatal("results body is not JSON")
	}
	if body["ok"] != true || body["questionnaire_id"] != float64(41) || body["side_effect_executed"] != false {
		t.Fatalf("results envelope=%#v", body)
	}
	results := body["results"].(map[string]any)
	if results["submission_count"] != float64(2) || results["latest_submitted_at"] != "2026-08-16T09:02:03Z" || results["average_score"] != 7.5 {
		t.Fatalf("results payload=%#v", results)
	}
	if rules, ok := results["rules"].([]any); !ok || len(rules) != 0 {
		t.Fatalf("rules=%#v", results["rules"])
	}
	seen := auth.capabilities()
	if len(seen) != 1 || seen[0] != authport.CapabilityQuestionnairesRead {
		t.Fatalf("capabilities=%v", seen)
	}

	stub.result = surveyport.SubmissionResult{QuestionnaireID: 41, Rules: []surveyport.ScoreRule{}}
	empty := httptest.NewRecorder()
	router.ServeHTTP(empty, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/results", legacyToken(82)))
	var emptyBody map[string]any
	if json.NewDecoder(empty.Body).Decode(&emptyBody) != nil {
		t.Fatal("empty results body is not JSON")
	}
	emptyResults := emptyBody["results"].(map[string]any)
	if empty.Code != http.StatusOK || emptyResults["submission_count"] != float64(0) || emptyResults["latest_submitted_at"] != nil || emptyResults["average_score"] != float64(0) {
		t.Fatalf("empty results=%d %#v", empty.Code, emptyResults)
	}
}

func TestF03QuestionnaireSubmissionsRoutePagingAndStableKeys(t *testing.T) {
	submittedAt := time.Date(2026, 8, 16, 9, 2, 3, 0, time.UTC)
	stub := &legacySubmissionStub{page: surveyport.SubmissionPage{Total: 2, Items: []surveyport.Submission{
		{
			ID: 42, QuestionnaireID: 41, SubmittedAt: submittedAt, CreatedAt: submittedAt,
			ExternalUserID: "ext-42", UnionID: "union-42", CustomerName: "小璨", Mobile: "138",
			SourceChannel: "wecom", CampaignID: "camp-1", StaffID: "staff-1", FollowUserUserID: "follow-1",
			MatchedBy: "external_userid", RespondentKey: "resp-1", TotalScore: 7.5,
			FinalTags: []string{"qualified"}, ResultToken: "token", RedirectURLSnapshot: "https://example.test/done",
			Answers: []surveyport.SubmissionAnswer{{
				QuestionID: 51, QuestionType: surveyport.SingleChoice, QuestionTitle: "目标", SortOrder: 0,
				SelectedOptions: []surveyport.SubmissionAnswerOption{{OptionID: 61, OptionText: "增长"}},
			}},
		},
		{
			ID: 41, QuestionnaireID: 41, SubmittedAt: submittedAt.Add(-time.Hour), CreatedAt: submittedAt,
			FinalTags: []string{}, Answers: []surveyport.SubmissionAnswer{},
		},
	}}}
	router, auth := legacySubmissionRouter(t, &recordingAuth{}, stub)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/submissions", legacyToken(83)))
	if response.Code != http.StatusOK || stub.listLimit != 20 || stub.listOffset != 0 {
		t.Fatalf("submissions status=%d paging=%d/%d body=%s", response.Code, stub.listLimit, stub.listOffset, response.Body.String())
	}
	var body map[string]any
	if json.NewDecoder(response.Body).Decode(&body) != nil {
		t.Fatal("submissions body is not JSON")
	}
	items, itemsOK := body["items"].([]any)
	submissions, submissionsOK := body["submissions"].([]any)
	if !itemsOK || !submissionsOK || len(items) != 2 || len(submissions) != 2 || body["total"] != float64(2) || body["limit"] != float64(20) || body["offset"] != float64(0) {
		t.Fatalf("submissions envelope=%#v", body)
	}
	firstJSON, secondJSON := items[0].(map[string]any), submissions[0].(map[string]any)
	if firstJSON["submission_id"] != float64(42) || firstJSON["external_userid"] != "ext-42" || firstJSON["score"] != 7.5 || firstJSON["total_score"] != 7.5 {
		t.Fatalf("first row=%#v", firstJSON)
	}
	if !mapsEqual(firstJSON, secondJSON) {
		t.Fatal("items and submissions diverge")
	}
	answers := firstJSON["answers"].([]any)
	answer := answers[0].(map[string]any)
	options := answer["selected_options"].([]any)
	if answer["question_id"] != float64(51) || answer["question_title"] != "目标" || options[0].(map[string]any)["option_text"] != "增长" {
		t.Fatalf("answer snapshot=%#v", answer)
	}
	emptyRow := items[1].(map[string]any)
	if tags, ok := emptyRow["final_tags"].([]any); !ok || len(tags) != 0 {
		t.Fatalf("empty final_tags=%#v", emptyRow["final_tags"])
	}
	if answers, ok := emptyRow["answers"].([]any); !ok || len(answers) != 0 {
		t.Fatalf("empty answers=%#v", emptyRow["answers"])
	}
	seen := auth.capabilities()
	if len(seen) != 1 || seen[0] != authport.CapabilityQuestionnairesRead {
		t.Fatalf("capabilities=%v", seen)
	}

	bounded := httptest.NewRecorder()
	router.ServeHTTP(bounded, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/submissions?limit=100&offset=40", legacyToken(84)))
	if bounded.Code != http.StatusOK || stub.listLimit != 100 || stub.listOffset != 40 {
		t.Fatalf("bounded paging status=%d paging=%d/%d", bounded.Code, stub.listLimit, stub.listOffset)
	}
}

func TestF03QuestionnaireSubmissionRoutesRejectBadInputAndMapErrors(t *testing.T) {
	stub := &legacySubmissionStub{}
	router, _ := legacySubmissionRouter(t, &recordingAuth{}, stub)
	for _, path := range []string{
		"/api/admin/questionnaires/41/submissions?limit=0",
		"/api/admin/questionnaires/41/submissions?limit=101",
		"/api/admin/questionnaires/41/submissions?offset=-1",
		"/api/admin/questionnaires/41/submissions?limit=abc",
		"/api/admin/questionnaires/41/submissions?role=admin",
		"/api/admin/questionnaires/0/submissions",
		"/api/admin/questionnaires/abc/results",
		"/api/admin/questionnaires/0/export",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(85)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	stub.err = surveyapp.ErrNotFound
	for _, path := range []string{"/api/admin/questionnaires/41/results", "/api/admin/questionnaires/41/submissions", "/api/admin/questionnaires/41/export"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(86)))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"ok":false`) {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	stub.err = surveyapp.ErrUnavailable
	for _, path := range []string{"/api/admin/questionnaires/41/results", "/api/admin/questionnaires/41/submissions", "/api/admin/questionnaires/41/export"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(87)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Header().Get("Content-Type"), "text/csv") {
			t.Fatalf("GET %s leaked a CSV content type on error", path)
		}
	}

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/41/submissions", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d", anonymous.Code)
	}
}

func TestF03QuestionnaireExportRouteStreamsSafeCSV(t *testing.T) {
	stub := &legacySubmissionStub{download: surveyport.SubmissionCSVDownload{
		Filename: "questionnaire-activation-submissions.csv", ContentType: "text/csv; charset=utf-8",
		Body: []byte("\ufeffsubmission_id,submitted_at\r\n42,2026-08-16 09:02:03\r\n"), Total: 1,
	}}
	router, auth := legacySubmissionRouter(t, &recordingAuth{}, stub)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/export", legacyToken(88)))
	if response.Code != http.StatusOK || !stub.exportCalled {
		t.Fatalf("export status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		response.Header().Get("Content-Disposition") != `attachment; filename="questionnaire-activation-submissions.csv"` {
		t.Fatalf("export headers=%v", response.Header())
	}
	if !strings.HasPrefix(response.Body.String(), "\ufeff") || !strings.Contains(response.Body.String(), "\r\n") {
		t.Fatalf("export body=%q", response.Body.String())
	}
	seen := auth.capabilities()
	if len(seen) != 1 || seen[0] != authport.CapabilityCustomersRead {
		t.Fatalf("capabilities=%v", seen)
	}
}

func TestF03QuestionnaireExportRejectsOwnerScopedAuthorization(t *testing.T) {
	stub := &legacySubmissionStub{download: surveyport.SubmissionCSVDownload{
		Filename: "questionnaire-a-submissions.csv", ContentType: "text/csv; charset=utf-8", Body: []byte("\ufeffx"),
	}}
	router, auth := legacySubmissionRouter(t, &ownerScopedAuth{}, stub)

	export := httptest.NewRecorder()
	router.ServeHTTP(export, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/export", legacyToken(89)))
	if export.Code != http.StatusForbidden || stub.exportCalled || strings.Contains(export.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("owner-scoped export status=%d called=%t body=%s", export.Code, stub.exportCalled, export.Body.String())
	}

	stub.result = surveyport.SubmissionResult{QuestionnaireID: 41, Rules: []surveyport.ScoreRule{}}
	results := httptest.NewRecorder()
	router.ServeHTTP(results, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41/results", legacyToken(90)))
	if results.Code != http.StatusOK {
		t.Fatalf("global-capable results under owner-scoped auth status=%d", results.Code)
	}
	seen := auth.capabilities()
	if len(seen) != 2 || seen[0] != authport.CapabilityCustomersRead || seen[1] != authport.CapabilityQuestionnairesRead {
		t.Fatalf("capabilities=%v", seen)
	}
}

func mapsEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

type submissionTestAuth interface {
	authport.Service
	capabilities() []authport.Capability
}

type ownerScopedAuth struct {
	recordingAuth
}

func (*ownerScopedAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	staff := int64(7)
	return authport.Principal{AdminUserID: 1, Role: authport.RoleSales, StaffID: &staff}, nil
}

func (service *ownerScopedAuth) Authorize(ctx context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if capability == authport.CapabilityCustomersRead {
		service.mu.Lock()
		service.seen = append(service.seen, capability)
		service.mu.Unlock()
		return authport.Authorization{Capability: capability, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 7}, nil
	}
	return service.recordingAuth.Authorize(ctx, principal, capability)
}

func legacySubmissionRouter(t *testing.T, auth submissionTestAuth, submissions legacySurveySubmissionApplication) (http.Handler, submissionTestAuth) {
	t.Helper()
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(auth, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, &legacySurveyStub{item: legacySurveyItem()})
	if err != nil {
		t.Fatal(err)
	}
	legacy.surveySubmissions = submissions
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, auth
}
