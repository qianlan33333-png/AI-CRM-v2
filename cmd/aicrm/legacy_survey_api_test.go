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

type legacySurveyStub struct {
	item    surveyport.Questionnaire
	page    surveyport.LegacyPage
	err     error
	command surveyport.CreateCommand
	creates int
}

func (stub *legacySurveyStub) ListLegacy(context.Context, int32, int32) (surveyport.LegacyPage, error) {
	return stub.page, stub.err
}
func (stub *legacySurveyStub) Get(context.Context, surveyport.ID) (surveyport.Questionnaire, error) {
	return stub.item, stub.err
}
func (stub *legacySurveyStub) Create(_ context.Context, command surveyport.CreateCommand) (surveyport.Questionnaire, error) {
	stub.command, stub.creates = command, stub.creates+1
	return stub.item, stub.err
}

func TestF01ALegacyQuestionnaireExactBodyRoundTripWithoutExtraHeader(t *testing.T) {
	item := legacySurveyItem()
	stub := &legacySurveyStub{item: item, page: surveyport.LegacyPage{Items: []surveyport.Questionnaire{item}, Total: 1, Limit: 50}}
	router, auth := legacySurveyRouter(t, stub)

	create := httptest.NewRecorder()
	router.ServeHTTP(create, legacySurveyCreateRequest(`{"name":"入门问卷","title":"欢迎填写","description":"说明","answer_display_mode":"all_in_one","assessment_enabled":false,"assessment_config":{},"slug":"welcome","is_disabled":false,"questions":[{"type":"single_choice","title":"你的目标","assessment_dimension_key":"","sidebar_profile_field":"","required":true,"sort_order":0,"placeholder_text":"","options":[{"option_text":"增长","score":0,"assessment_type_key":"","tag_codes":[],"is_other":false,"other_placeholder":"","other_max_length":0,"sort_order":0}]}],"score_rules":[]}`))
	if create.Code != http.StatusOK || stub.command.Actor != 1 || stub.command.Name != "入门问卷" || stub.command.IdempotencyKey == "" || len(stub.command.Questions) != 1 {
		t.Fatalf("create status=%d command=%#v body=%s", create.Code, stub.command, create.Body.String())
	}
	var createBody map[string]any
	if json.NewDecoder(create.Body).Decode(&createBody) != nil || createBody["ok"] != true || createBody["questionnaire_id"] != float64(41) {
		t.Fatalf("create envelope=%#v", createBody)
	}
	data := createBody["data"].(map[string]any)
	questionnaire := data["questionnaire"].(map[string]any)
	if questionnaire["id"] != float64(41) || len(questionnaire["questions"].([]any)) != 1 {
		t.Fatalf("data.questionnaire=%#v", questionnaire)
	}
	seen := auth.capabilities()
	if len(seen) != 1 || seen[0] != authport.CapabilityQuestionnairesWrite {
		t.Fatalf("capabilities=%v", seen)
	}

	auth.reset()
	list := httptest.NewRecorder()
	router.ServeHTTP(list, legacyRequest(http.MethodGet, "/api/admin/questionnaires", legacyToken(72)))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"questionnaires"`) || !strings.Contains(list.Body.String(), `"items"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, legacyRequest(http.MethodGet, "/api/admin/questionnaires/41", legacyToken(73)))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"questionnaire"`) || !strings.Contains(detail.Body.String(), `"questions"`) {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
}

func TestF01ALegacyQuestionnaireRejectsCrossOriginF02AndStableErrors(t *testing.T) {
	stub := &legacySurveyStub{item: legacySurveyItem()}
	router, _ := legacySurveyRouter(t, stub)
	crossOrigin := legacySurveyCreateRequest(`{"name":"问卷","title":"问卷","description":"","answer_display_mode":"all_in_one","assessment_enabled":false,"assessment_config":{},"slug":"","is_disabled":false,"questions":[],"score_rules":[]}`)
	crossOrigin.Header.Set("Origin", "https://cross-origin.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, crossOrigin)
	if response.Code != http.StatusForbidden || stub.creates != 0 {
		t.Fatalf("cross-origin status=%d creates=%d body=%s", response.Code, stub.creates, response.Body.String())
	}

	stub.err = surveyapp.ErrAssessmentUnavailable
	f02 := httptest.NewRecorder()
	router.ServeHTTP(f02, legacySurveyCreateRequest(`{"name":"问卷","title":"问卷","description":"","answer_display_mode":"all_in_one","assessment_enabled":true,"assessment_config":{},"slug":"","is_disabled":false,"questions":[],"score_rules":[]}`))
	if f02.Code != http.StatusBadRequest {
		t.Fatalf("F02 status=%d body=%s", f02.Code, f02.Body.String())
	}
	var body map[string]any
	if json.NewDecoder(f02.Body).Decode(&body) != nil || body["ok"] != false || body["message"] != surveyapp.ErrAssessmentUnavailable.Error() || body["detail"] != surveyapp.ErrAssessmentUnavailable.Error() {
		t.Fatalf("F02 body=%#v", body)
	}

	stub.err = surveyapp.ErrNotFound
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, legacyRequest(http.MethodGet, "/api/admin/questionnaires/999", legacyToken(74)))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"ok":false`) {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func legacySurveyRouter(t *testing.T, surveys legacySurveyApplication) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(service, &legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{}, &legacyCancelStub{}, &legacyRetryStub{}, &legacyProductStub{}, &legacyMediaStub{}, surveys)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, service
}

func legacySurveyCreateRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/admin/questionnaires", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(70)})
	request.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: legacyToken(71)})
	return request
}

func legacySurveyItem() surveyport.Questionnaire {
	now := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	return surveyport.Questionnaire{
		ID: 41, Name: "入门问卷", Title: "欢迎填写", Description: "说明", Slug: "welcome",
		AnswerDisplayMode: surveyport.AllInOne, AssessmentConfig: json.RawMessage(`{}`), CreatedBy: 1,
		Version: 1, CreatedAt: now, UpdatedAt: now, ScoreRules: []surveyport.ScoreRule{},
		Questions: []surveyport.Question{{ID: 51, Type: surveyport.SingleChoice, Title: "你的目标", Required: true, SortOrder: 0,
			Validation: surveyport.Validation{MinimumSelections: intTestRef(1), MaximumSelections: intTestRef(1)},
			Options:    []surveyport.Option{{ID: 61, OptionText: "增长", TagCodes: []string{}, SortOrder: 0}}}},
	}
}

func intTestRef(value int) *int { return &value }
