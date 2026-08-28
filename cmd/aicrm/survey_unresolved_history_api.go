package main

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func (h *Handler) ListSurveyUnresolvedHistorySubmissions(w http.ResponseWriter, r *http.Request) {
	h.serveSurveyUnresolvedHistory(w, r, "list")
}
func (h *Handler) GetSurveyUnresolvedHistorySubmission(w http.ResponseWriter, r *http.Request) {
	h.serveSurveyUnresolvedHistory(w, r, "detail")
}
func (h *Handler) ListSurveyUnresolvedHistoryAnswers(w http.ResponseWriter, r *http.Request) {
	h.serveSurveyUnresolvedHistory(w, r, "answers")
}

func (h *Handler) serveSurveyUnresolvedHistory(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || r == nil || nilLegacyDependency(h.surveyUnresolvedHistory) {
		surveyUnresolvedHistoryUnavailable(w)
		return
	}
	params, parseErr := url.ParseQuery(r.URL.RawQuery)
	query := surveyport.SurveyUnresolvedHistoryQuery{}
	valid := parseErr == nil
	if values, present := params["questionnaire_id"]; present {
		id, ok := audienceHistoryID(params.Get("questionnaire_id"))
		valid = valid && kind == "list" && len(values) == 1 && ok
		query.QuestionnaireID = &id
		params.Del("questionnaire_id")
	}
	limit, offset, pageOK := audienceHistoryPage(params.Encode())
	query.Limit, query.Offset = limit, offset
	valid = valid && pageOK
	var id int64
	if kind != "list" {
		var ok bool
		id, ok = audienceHistoryID(chi.URLParam(r, "history_id"))
		valid = valid && ok && (kind != "detail" || r.URL.RawQuery == "")
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_survey_unresolved_history_query"})
		return
	}
	response := map[string]any{"source": "v1_history", "read_only": true, "real_external_call_executed": false, "definition_mapping": "historical_source_only"}
	var err error
	var total int64
	var count int
	switch kind {
	case "detail":
		var item surveyport.HistoricalUnresolvedSurveySubmission
		item, err = h.surveyUnresolvedHistory.GetHistoricalUnresolvedSurveySubmission(r.Context(), id)
		if err == nil {
			_, err = surveyapp.HistoricalUnresolvedSurveySubmissionDigest(item)
		}
		if item.ID != id {
			err = surveyport.ErrSurveyUnresolvedHistoryUnavailable
		}
		response["item"] = item
	case "list":
		var items []surveyport.HistoricalUnresolvedSurveySubmission
		items, total, err = h.surveyUnresolvedHistory.ListHistoricalUnresolvedSurveySubmissions(r.Context(), query)
		if items == nil {
			items = []surveyport.HistoricalUnresolvedSurveySubmission{}
		}
		for _, item := range items {
			if err != nil {
				break
			}
			_, err = surveyapp.HistoricalUnresolvedSurveySubmissionDigest(item)
			if query.QuestionnaireID != nil && (item.QuestionnaireID == nil || *item.QuestionnaireID != *query.QuestionnaireID) {
				err = surveyport.ErrSurveyUnresolvedHistoryUnavailable
			}
		}
		response["items"], count = items, len(items)
	case "answers":
		var items []surveyport.HistoricalUnresolvedSurveyAnswer
		items, total, err = h.surveyUnresolvedHistory.ListHistoricalUnresolvedSurveyAnswers(r.Context(), id, query)
		if items == nil {
			items = []surveyport.HistoricalUnresolvedSurveyAnswer{}
		}
		for _, item := range items {
			if err != nil {
				break
			}
			_, err = surveyapp.HistoricalUnresolvedSurveyAnswerDigest(item)
			if item.SubmissionID != id {
				err = surveyport.ErrSurveyUnresolvedHistoryUnavailable
			}
		}
		response["items"], count = items, len(items)
	default:
		err = surveyport.ErrSurveyUnresolvedHistoryUnavailable
	}
	if err != nil || kind != "detail" && (total < 0 || int64(count) != min(int64(limit), max(0, total-int64(offset)))) {
		surveyUnresolvedHistoryUnavailable(w)
		return
	}
	if kind != "detail" {
		response["total"], response["limit"], response["offset"] = total, limit, offset
	}
	writeJSON(w, http.StatusOK, response)
}

func surveyUnresolvedHistoryUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "survey_unresolved_history_unavailable"})
}
