package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
	IsDisabled        bool                         `json:"is_disabled"`
	Questions         []surveyport.Question        `json:"questions"`
	ScoreRules        []surveyport.ScoreRule       `json:"score_rules"`
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
			AssessmentConfig: body.AssessmentConfig, Slug: body.Slug, IsDisabled: body.IsDisabled,
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
	case errors.Is(err, surveyapp.ErrInvalidQuestionnaire), errors.Is(err, surveyapp.ErrInvalidSchema), errors.Is(err, surveyapp.ErrInvalidPage), errors.Is(err, surveyapp.ErrAssessmentUnavailable):
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
