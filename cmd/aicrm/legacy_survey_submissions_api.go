package main

import (
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

// Questionnaire submission reads are pure Survey-owned snapshot reads. The
// export route carries PII, so it additionally requires the global customer
// read authorization granted by the route middleware; an owner-scoped
// customer read is never enough for a full export.
func (handler *Handler) GetQuestionnaireResults(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveySubmissions) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	id, err := legacySurveyID(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	results, err := handler.surveySubmissions.Results(request.Context(), id)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	var latest any
	if !results.LatestSubmittedAt.IsZero() {
		latest = results.LatestSubmittedAt.UTC()
	}
	payload := map[string]any{
		"submission_count":    results.SubmissionCount,
		"latest_submitted_at": latest,
		"average_score":       results.AverageScore,
		"rules":               results.Rules,
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "questionnaire_id": int64(results.QuestionnaireID),
		"results": payload, "data": map[string]any{"results": payload},
		"side_effect_executed": false,
	})
}

func (handler *Handler) ListQuestionnaireSubmissions(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveySubmissions) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	id, err := legacySurveyID(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	limit, offset, err := legacySubmissionPage(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	page, err := handler.surveySubmissions.List(request.Context(), id, limit, offset)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	items := make([]map[string]any, len(page.Items))
	for i, item := range page.Items {
		items[i] = legacySubmission(item)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok": true, "questionnaire_id": int64(id),
		"items": items, "submissions": items,
		"data":  map[string]any{"submissions": items},
		"total": page.Total, "limit": page.Limit, "offset": page.Offset,
		"side_effect_executed": false,
	})
}

func (handler *Handler) ExportQuestionnaireSubmissions(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.surveySubmissions) || request == nil {
		writeLegacySurveyError(writer, surveyapp.ErrUnavailable)
		return
	}
	if !questionnaireExportAuthorized(request) {
		writeLegacySurveyError(writer, authport.ErrUnauthorized)
		return
	}
	id, err := legacySurveyID(request)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	download, err := handler.surveySubmissions.Export(request.Context(), id)
	if err != nil {
		writeLegacySurveyError(writer, err)
		return
	}
	writer.Header().Set("Content-Type", download.ContentType)
	writer.Header().Set("Content-Disposition", `attachment; filename="`+download.Filename+`"`)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(download.Body); err != nil {
		return
	}
}

func questionnaireExportAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	return ok && authorization.Capability == authport.CapabilityCustomersRead &&
		authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacySubmissionPage(request *http.Request) (int32, int32, error) {
	query := request.URL.Query()
	for key := range query {
		if key != "limit" && key != "offset" {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
	}
	limit, offset := int64(surveyapp.SubmissionDefaultLimit), int64(0)
	var err error
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	if err == nil {
		if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
			offset, err = strconv.ParseInt(raw, 10, 32)
		}
	}
	if err != nil || limit < 1 || limit > int64(surveyapp.SubmissionMaximumLimit) || offset < 0 || offset > int64(surveyapp.SubmissionMaximumOffset) {
		return 0, 0, surveyapp.ErrInvalidSubmissionPage
	}
	return int32(limit), int32(offset), nil
}

func legacySubmission(item surveyport.Submission) map[string]any {
	answers := make([]map[string]any, len(item.Answers))
	for i, answer := range item.Answers {
		options := make([]map[string]any, len(answer.SelectedOptions))
		for j, option := range answer.SelectedOptions {
			options[j] = map[string]any{"option_id": option.OptionID, "option_text": option.OptionText}
		}
		answers[i] = map[string]any{
			"question_id": answer.QuestionID, "question_type": answer.QuestionType,
			"question_title": answer.QuestionTitle, "sort_order": answer.SortOrder,
			"selected_options": options, "text_value": answer.TextValue,
		}
	}
	tags := item.FinalTags
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{
		"id": item.ID, "submission_id": item.ID,
		"questionnaire_id": int64(item.QuestionnaireID),
		"submitted_at":     item.SubmittedAt.UTC(), "created_at": item.CreatedAt.UTC(),
		"respondent_key": item.RespondentKey, "openid": item.OpenID,
		"unionid": item.UnionID, "external_userid": item.ExternalUserID,
		"customer_name": item.CustomerName, "follow_user_userid": item.FollowUserUserID,
		"matched_by": item.MatchedBy, "mobile": item.Mobile,
		"source_channel": item.SourceChannel, "campaign_id": item.CampaignID, "staff_id": item.StaffID,
		"score": item.TotalScore, "total_score": item.TotalScore,
		"final_tags": tags, "result_token": item.ResultToken,
		"redirect_url_snapshot": item.RedirectURLSnapshot, "answers": answers,
	}
}
