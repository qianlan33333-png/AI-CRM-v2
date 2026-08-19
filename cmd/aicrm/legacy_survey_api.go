package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type legacyQuestionnaireRequest struct {
	Name              string                       `json:"name"`
	Title             string                       `json:"title"`
	Description       string                       `json:"description"`
	AnswerDisplayMode surveyport.AnswerDisplayMode `json:"answer_display_mode"`
	AssessmentEnabled bool                         `json:"assessment_enabled"`
	AssessmentConfig  json.RawMessage              `json:"assessment_config"`
	Slug              string                       `json:"slug"`
	IsDisabled        *bool                        `json:"is_disabled"`
	Questions         []surveyport.Question        `json:"questions"`
	ScoreRules        []surveyport.ScoreRule       `json:"score_rules"`
}

const (
	legacyQuestionnairePagePath      = "/admin/questionnaires"
	legacyQuestionnairePreflightPath = "/api/admin/questionnaires/preflight"
)

type legacyQuestionnairePreflightChecks struct {
	WechatOAuthConfigured       bool `json:"wechat_oauth_configured"`
	WeComContactConfigured      bool `json:"wecom_contact_configured"`
	DebugSessionAPIEnabled      bool `json:"debug_session_api_enabled"`
	WeComTagsAPIAvailable       bool `json:"wecom_tags_api_available"`
	QuestionnaireAdminUIEnabled bool `json:"questionnaire_admin_ui_enabled"`
	IdentityMapAvailable        bool `json:"identity_map_available"`
}

type legacyQuestionnairePreflightResponse struct {
	OK     bool                               `json:"ok"`
	Checks legacyQuestionnairePreflightChecks `json:"checks"`
	Status string                             `json:"status"`
}

func legacyQuestionnaireSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(legacyQuestionnaireHeaderWriter{ResponseWriter: writer}, request)
	})
}

type legacyQuestionnaireHeaderWriter struct{ http.ResponseWriter }

func (writer legacyQuestionnaireHeaderWriter) WriteHeader(status int) {
	writer.setSecurityHeaders()
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyQuestionnaireHeaderWriter) Write(payload []byte) (int, error) {
	writer.setSecurityHeaders()
	return writer.ResponseWriter.Write(payload)
}

func (writer legacyQuestionnaireHeaderWriter) setSecurityHeaders() {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyQuestionnaireMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func (handler *Handler) QuestionnaireListPage(writer http.ResponseWriter, request *http.Request) {
	if request != nil && request.URL.Path == legacyQuestionnairePagePath+"/ui" {
		http.Redirect(writer, request, legacyQuestionnairePagePath, http.StatusFound)
		return
	}
	http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape(legacyQuestionnairePagePath), http.StatusFound)
}

// QuestionnairePreflight is deliberately a local declaration-only snapshot.
// It does not read secrets, query a provider, inspect the database, or infer
// readiness from the existence of an unrelated route. The UI flag is true
// because this exact build registers the questionnaire admin page routes.
func (handler *Handler) QuestionnairePreflight(writer http.ResponseWriter, _ *http.Request) {
	checks := legacyQuestionnairePreflightChecks{
		WechatOAuthConfigured:       false,
		WeComContactConfigured:      false,
		DebugSessionAPIEnabled:      false,
		WeComTagsAPIAvailable:       false,
		QuestionnaireAdminUIEnabled: true,
		IdentityMapAvailable:        false,
	}
	status := "partial"
	if checks.WechatOAuthConfigured && checks.WeComContactConfigured && checks.WeComTagsAPIAvailable {
		status = "ok"
	}
	writeJSON(writer, http.StatusOK, legacyQuestionnairePreflightResponse{OK: true, Checks: checks, Status: status})
}

func (handler *Handler) ListQuestionnaires(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveys) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	limit, offset, err := legacySurveyPage(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	page, err := handler.surveys.ListLegacy(request.Context(), limit, offset)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	items := make([]map[string]any, len(page.Items))
	for i, item := range page.Items {
		items[i] = legacyQuestionnaire(item)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "questionnaires": items, "items": items,
		"data":  map[string]any{"questionnaires": items},
		"total": page.Total, "limit": page.Limit, "offset": page.Offset,
	})
}

func (handler *Handler) GetQuestionnaire(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveys) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "questionnaire_id")), 10, 64)
	if err != nil || id < 1 {
		writeLegacySurveyError(writer, surveyapp.ErrInvalidQuestionnaire)
		return
	}
	item, err := handler.surveys.Get(request.Context(), surveyport.ID(id))
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	mapped := legacyQuestionnaire(item)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "questionnaire": mapped, "questions": item.Questions,
		"data": map[string]any{"questionnaire": mapped},
	})
}

func (handler *Handler) CreateQuestionnaire(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveys) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacySurveyError(writer, authport.ErrUnauthorized)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var body legacyQuestionnaireRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		writeLegacySurveyError(writer, surveyapp.ErrInvalidQuestionnaire)
		return
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
			return
		}
		key = "legacy-questionnaire:" + hex.EncodeToString(random[:])
	}
	created, err := handler.surveys.Create(request.Context(), surveyport.CreateCommand{
		Questionnaire: surveyport.Questionnaire{
			Name: body.Name, Title: body.Title, Description: body.Description,
			AnswerDisplayMode: body.AnswerDisplayMode, AssessmentEnabled: body.AssessmentEnabled,
			AssessmentConfig: body.AssessmentConfig, Slug: body.Slug, IsDisabled: body.IsDisabled != nil && *body.IsDisabled,
			Questions: body.Questions, ScoreRules: body.ScoreRules,
		},
		Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	mapped := legacyQuestionnaire(created)
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "questionnaire": mapped, "questionnaire_id": int64(created.ID),
		"questions": created.Questions, "data": map[string]any{"questionnaire": mapped},
	})
}

func (handler *Handler) UpdateQuestionnaire(writer http.ResponseWriter, request *http.Request) {
	id, principal, body, key, ok := handler.legacySurveyWriteInput(writer, request)
	if !ok {
		return
	}
	updated, err := handler.surveys.Update(request.Context(), id, surveyport.UpdateCommand{Questionnaire: questionnaireFromRequest(body), Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	writeLegacySurveyMutation(writer, updated, "updated", map[string]any{"questionnaire_id": int64(updated.ID)})
}

func (handler *Handler) DuplicateQuestionnaire(writer http.ResponseWriter, request *http.Request) {
	id, principal, body, key, ok := handler.legacySurveyWriteInput(writer, request)
	if !ok {
		return
	}
	item, err := handler.surveys.Duplicate(request.Context(), id, principal.AdminUserID, key, strings.TrimSpace(body.Title), strings.TrimSpace(body.Slug))
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	writeLegacySurveyMutation(writer, item, "duplicated", map[string]any{"questionnaire_id": int64(item.ID), "source_questionnaire_id": int64(id)})
}

func (handler *Handler) SetQuestionnaireDisabled(writer http.ResponseWriter, request *http.Request) {
	id, principal, body, key, ok := handler.legacySurveyWriteInput(writer, request)
	if !ok {
		return
	}
	disabled := !strings.HasSuffix(request.URL.Path, "/enable")
	if body.IsDisabled != nil {
		disabled = *body.IsDisabled
	}
	item, err := handler.surveys.SetDisabled(request.Context(), id, disabled, principal.AdminUserID, key)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	status := "enabled"
	if disabled {
		status = "disabled"
	}
	writeLegacySurveyMutation(writer, item, status, map[string]any{"questionnaire_id": int64(id)})
}

func (handler *Handler) DeleteQuestionnaire(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveys) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	id, err := legacySurveyID(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacySurveyError(writer, authport.ErrUnauthorized)
		return
	}
	key, err := legacySurveyIdempotencyKey(request)
	if err != nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	result, err := handler.surveys.Delete(request.Context(), id, principal.AdminUserID, key)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	writeLegacySurveyMutation(writer, result.Questionnaire, "deleted", map[string]any{"questionnaire_id": int64(id), "deleted": result.Deleted, "delete_mode": "hard_delete"})
}

func (handler *Handler) legacySurveyWriteInput(writer http.ResponseWriter, request *http.Request) (surveyport.ID, authport.Principal, legacyQuestionnaireRequest, string, bool) {
	if handler == nil || nilLegacyDependency(handler.surveys) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	id, err := legacySurveyID(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacySurveyError(writer, authport.ErrUnauthorized)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	var body legacyQuestionnaireRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.UseNumber()
	decodeErr := decoder.Decode(&body)
	if decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		writeLegacySurveyError(writer, surveyapp.ErrInvalidQuestionnaire)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	if decodeErr == nil && !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		writeLegacySurveyError(writer, surveyapp.ErrInvalidQuestionnaire)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	key, err := legacySurveyIdempotencyKey(request)
	if err != nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return 0, authport.Principal{}, legacyQuestionnaireRequest{}, "", false
	}
	return id, principal, body, key, true
}

func legacySurveyID(request *http.Request) (surveyport.ID, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(request, "questionnaire_id")), 10, 64)
	if err != nil || id < 1 {
		return 0, surveyapp.ErrInvalidQuestionnaire
	}
	return surveyport.ID(id), nil
}

func legacySurveyIdempotencyKey(request *http.Request) (string, error) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key != "" {
		return key, nil
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "legacy-questionnaire:" + hex.EncodeToString(random[:]), nil
}

func questionnaireFromRequest(body legacyQuestionnaireRequest) surveyport.Questionnaire {
	return surveyport.Questionnaire{Name: body.Name, Title: body.Title, Description: body.Description,
		AnswerDisplayMode: body.AnswerDisplayMode, AssessmentEnabled: body.AssessmentEnabled, AssessmentConfig: body.AssessmentConfig,
		Slug: body.Slug, IsDisabled: body.IsDisabled != nil && *body.IsDisabled, Questions: body.Questions, ScoreRules: body.ScoreRules}
}

func writeLegacySurveyMutation(writer http.ResponseWriter, item surveyport.Questionnaire, status string, extra map[string]any) {
	mapped := legacyQuestionnaire(item)
	payload := map[string]any{"ok": true, "questionnaire": mapped, "questions": item.Questions, "data": map[string]any{"questionnaire": mapped}, "write_model_status": status}
	for key, value := range extra {
		payload[key] = value
	}
	writeJSON(writer, http.StatusOK, payload)
}

func legacySurveyPage(request *http.Request) (int32, int32, error) {
	query := request.URL.Query()
	for key := range query {
		if key != "limit" && key != "offset" {
			return 0, 0, surveyapp.ErrInvalidPage
		}
	}
	limit, offset := int64(surveyapp.DefaultLimit), int64(0)
	var err error
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	if err == nil {
		if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
			offset, err = strconv.ParseInt(raw, 10, 32)
		}
	}
	if err != nil || limit < 1 || limit > int64(surveyapp.MaximumLimit) || offset < 0 || offset > int64(surveyapp.MaximumLegacyOffset) {
		return 0, 0, surveyapp.ErrInvalidPage
	}
	return int32(limit), int32(offset), nil
}

func legacyQuestionnaire(item surveyport.Questionnaire) map[string]any {
	status := "active"
	if item.IsDisabled {
		status = "disabled"
	}
	return map[string]any{
		"id": int64(item.ID), "name": item.Name, "title": item.Title, "description": item.Description,
		"slug": item.Slug, "enabled": !item.IsDisabled, "is_disabled": item.IsDisabled,
		"status": status, "version": item.Version, "question_count": len(item.Questions),
		"submission_count": item.SubmissionCount, "assessment_enabled": item.AssessmentEnabled,
		"assessment_config": item.AssessmentConfig, "answer_display_mode": item.AnswerDisplayMode,
		"questions": item.Questions, "score_rules": item.ScoreRules,
		"created_at": item.CreatedAt.UTC(), "updated_at": item.UpdatedAt.UTC(),
		"public_path":    "/q/" + item.Slug,
		"submitted_path": "/admin/questionnaires/" + strconv.FormatInt(int64(item.ID), 10) + "/submissions",
	}
}

func writeLegacySurveyError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, surveyapp.ErrInvalidQuestionnaire), errors.Is(err, surveyapp.ErrInvalidSchema), errors.Is(err, surveyapp.ErrInvalidPage), errors.Is(err, surveyapp.ErrInvalidSubmissionPage), errors.Is(err, surveyapp.ErrAssessmentUnavailable):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, surveyapp.ErrNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, surveyapp.ErrConflict):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "message": err.Error(), "detail": err.Error()})
}
