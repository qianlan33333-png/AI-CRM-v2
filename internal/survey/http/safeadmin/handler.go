// Package safeadminhttp exposes the Survey-only, de-identified administrator
// read leaf. Central session, capability, CSRF and OpenAPI wiring remain Lane E
// responsibilities.
package safeadminhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const (
	maximumPreviewBodyBytes = 4 * 1024

	ResultsPath       = "/api/admin/questionnaires/{questionnaire_id}/analysis"
	ExportPreviewPath = "/api/admin/questionnaires/{questionnaire_id}/export/preview"
)

var canonicalUnsignedDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

type Application interface {
	SafeAnalysis(context.Context, surveyport.ID, int32, int32) (surveyport.SafeSubmissionAnalysis, error)
	SafeExportPreview(context.Context, surveyport.ID, surveyport.SafeExportPreviewRequest) (surveyport.SafeExportPreview, error)
}

var _ Application = (*surveyapp.SafeAdminService)(nil)

type Handler struct {
	Application Application
}

func New(application Application) *Handler { return &Handler{Application: application} }

// Routes returns a leaf mux only. Exact main already owns ResultsPath, so Lane
// E must replace that one route entry rather than mount a duplicate. Both
// routes stay behind authenticated questionnaires.read; ExportPreviewPath also
// stays behind the existing central CSRF middleware.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(ResultsPath, h.Results)
	mux.HandleFunc(ExportPreviewPath, h.ExportPreview)
	return mux
}

// Results serves the de-identified submission statistics read.
func (h *Handler) Results(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if status, code := authorizeRead(r); status != 0 {
		writeError(w, status, code)
		return
	}
	questionnaireID, err := parseRouteQuestionnaireID(r, "/analysis")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	limit, offset, err := ParseAnalysisQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_read_unavailable")
		return
	}
	result, err := h.Application.SafeAnalysis(r.Context(), questionnaireID, limit, offset)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validSafeAnalysisResponse(result, questionnaireID, limit, offset) {
		writeError(w, http.StatusServiceUnavailable, "survey_read_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ExportPreview serves the bounded local projection; it never creates a file.
func (h *Handler) ExportPreview(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if status, code := authorizeRead(r); status != 0 {
		writeError(w, status, code)
		return
	}
	questionnaireID, err := parseRouteQuestionnaireID(r, "/export/preview")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_questionnaire_id")
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "invalid_export_preview")
		return
	}
	request, err := DecodePreviewRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_export_preview")
		return
	}
	if h == nil || h.Application == nil {
		writeError(w, http.StatusServiceUnavailable, "survey_read_unavailable")
		return
	}
	result, err := h.Application.SafeExportPreview(r.Context(), questionnaireID, request)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	if !validSafePreviewResponse(result, questionnaireID, request) {
		writeError(w, http.StatusServiceUnavailable, "survey_read_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func ParseQuestionnaireID(raw string) (surveyport.ID, error) {
	if raw == "" || !canonicalUnsignedDecimal.MatchString(raw) || raw == "0" {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 || strconv.FormatInt(value, 10) != raw {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	return surveyport.ID(value), nil
}

func parseRouteQuestionnaireID(r *http.Request, suffix string) (surveyport.ID, error) {
	if r == nil || r.URL == nil {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	const prefix = "/api/admin/questionnaires/"
	escapedPath := r.URL.EscapedPath()
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	rawID := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)
	id, err := ParseQuestionnaireID(rawID)
	if err != nil {
		return 0, err
	}
	want := prefix + strconv.FormatInt(int64(id), 10) + suffix
	if escapedPath != want {
		// Reject encoded digits/slashes, backslashes, extra segments and every
		// alternate spelling without depending on a specific router's params.
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	return id, nil
}

func ParseAnalysisQuery(raw string) (int32, int32, error) {
	limit := surveyapp.SafeAnalysisDefaultQuestionLimit
	offset := int32(0)
	if raw == "" {
		return limit, offset, nil
	}
	seen := map[string]struct{}{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" || strings.ContainsAny(pair, "%+;") {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
		key := parts[0]
		if key != "limit" && key != "offset" {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
		if _, duplicate := seen[key]; duplicate {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
		seen[key] = struct{}{}
		parsed, err := parseCanonicalInt32(parts[1])
		if err != nil {
			return 0, 0, surveyapp.ErrInvalidSubmissionPage
		}
		switch key {
		case "limit":
			if parsed < 1 || parsed > surveyapp.SafeAnalysisMaximumQuestionLimit {
				return 0, 0, surveyapp.ErrInvalidSubmissionPage
			}
			limit = parsed
		case "offset":
			if parsed < 0 || parsed > surveyapp.SafeAnalysisMaximumQuestionOffset {
				return 0, 0, surveyapp.ErrInvalidSubmissionPage
			}
			offset = parsed
		}
	}
	return limit, offset, nil
}

func DecodePreviewRequest(r *http.Request) (surveyport.SafeExportPreviewRequest, error) {
	if r == nil || r.Body == nil {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || len(parameters) > 1 {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	for name, value := range parameters {
		if name != "charset" || !strings.EqualFold(value, "utf-8") {
			return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maximumPreviewBodyBytes+1))
	if err != nil || len(body) == 0 || len(body) > maximumPreviewBodyBytes || !utf8.Valid(body) {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	seen := map[string]struct{}{}
	request := surveyport.SafeExportPreviewRequest{Limit: surveyapp.SafeExportPreviewDefaultLimit}
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok {
			return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
		}
		if _, duplicate := seen[key]; duplicate {
			return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
		}
		switch key {
		case "fields":
			fields, fieldErr := decodePreviewFields(raw)
			if fieldErr != nil {
				return surveyport.SafeExportPreviewRequest{}, fieldErr
			}
			request.Fields = fields
		case "limit":
			value, valueErr := decodeCanonicalInt32(raw)
			if valueErr != nil || value < 1 || value > surveyapp.SafeExportPreviewMaximumLimit {
				return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
			}
			request.Limit = value
		case "offset":
			value, valueErr := decodeCanonicalInt32(raw)
			if valueErr != nil || value < 0 || value > surveyapp.SafeExportPreviewMaximumOffset {
				return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
			}
			request.Offset = value
		default:
			return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	if token, trailingErr := decoder.Token(); trailingErr != io.EOF || token != nil {
		return surveyport.SafeExportPreviewRequest{}, surveyapp.ErrInvalidSubmissionPage
	}
	return request, nil
}

func decodePreviewFields(raw json.RawMessage) ([]surveyport.SafeExportPreviewField, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, surveyapp.ErrInvalidSubmissionPage
	}
	var values []string
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&values) != nil {
		return nil, surveyapp.ErrInvalidSubmissionPage
	}
	if len(values) > 4 {
		return nil, surveyapp.ErrInvalidSubmissionPage
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]surveyport.SafeExportPreviewField, len(values))
	for index, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			return nil, surveyapp.ErrInvalidSubmissionPage
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, surveyapp.ErrInvalidSubmissionPage
		}
		seen[value] = struct{}{}
		field := surveyport.SafeExportPreviewField(value)
		switch field {
		case surveyport.SafePreviewRowNumber, surveyport.SafePreviewSubmittedAt, surveyport.SafePreviewScore, surveyport.SafePreviewChoiceOptionIDs:
			result[index] = field
		default:
			return nil, surveyapp.ErrInvalidSubmissionPage
		}
	}
	return result, nil
}

func decodeCanonicalInt32(raw json.RawMessage) (int32, error) {
	value := string(bytes.TrimSpace(raw))
	return parseCanonicalInt32(value)
}

func parseCanonicalInt32(value string) (int32, error) {
	if !canonicalUnsignedDecimal.MatchString(value) {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || strconv.FormatInt(parsed, 10) != value {
		return 0, surveyapp.ErrInvalidSubmissionPage
	}
	return int32(parsed), nil
}

func validSafeAnalysisResponse(value surveyport.SafeSubmissionAnalysis, questionnaireID surveyport.ID, limit, offset int32) bool {
	if !value.OK || value.QuestionnaireID != questionnaireID || value.Limit != limit || value.Offset != offset ||
		!value.Deidentified || value.ContainsRawIdentity || value.ContainsFreeText || !value.LocalOnly || value.RealExternalCallExecuted ||
		value.Stats.SubmissionCount < 0 || math.IsNaN(value.Stats.AverageScore) || math.IsInf(value.Stats.AverageScore, 0) ||
		value.TotalQuestions < 0 || value.ScannedSubmissionCount < 0 || value.Questions == nil {
		return false
	}
	if value.Stats.SubmissionCount == 0 {
		if value.Stats.LatestSubmittedAt != nil || value.Stats.AverageScore != 0 {
			return false
		}
	} else if value.Stats.LatestSubmittedAt == nil || value.Stats.LatestSubmittedAt.IsZero() {
		return false
	}
	expectedScanned := value.Stats.SubmissionCount
	if expectedScanned > surveyapp.SafeAnalysisScanLimit {
		expectedScanned = surveyapp.SafeAnalysisScanLimit
	}
	if value.ScannedSubmissionCount != expectedScanned || value.AggregationComplete != (value.Stats.SubmissionCount <= surveyapp.SafeAnalysisScanLimit) {
		return false
	}
	expectedQuestions := int32(0)
	if offset < value.TotalQuestions {
		expectedQuestions = value.TotalQuestions - offset
		if expectedQuestions > limit {
			expectedQuestions = limit
		}
	}
	if int32(len(value.Questions)) != expectedQuestions {
		return false
	}
	for index, question := range value.Questions {
		if question.QuestionID < 1 || !safeChoiceType(question.QuestionType) || question.SortOrder < 0 ||
			question.AnsweredCount < 0 || question.AnsweredCount > value.ScannedSubmissionCount || question.Options == nil {
			return false
		}
		if index > 0 {
			previous := value.Questions[index-1]
			if previous.SortOrder > question.SortOrder || previous.SortOrder == question.SortOrder && previous.QuestionID >= question.QuestionID {
				return false
			}
		}
		for optionIndex, option := range question.Options {
			if option.OptionID < 1 || option.SelectionCount < 1 || option.SelectionCount > question.AnsweredCount ||
				optionIndex > 0 && question.Options[optionIndex-1].OptionID >= option.OptionID {
				return false
			}
		}
	}
	return true
}

func validSafePreviewResponse(value surveyport.SafeExportPreview, questionnaireID surveyport.ID, request surveyport.SafeExportPreviewRequest) bool {
	expectedFields := request.Fields
	if len(expectedFields) == 0 {
		expectedFields = []surveyport.SafeExportPreviewField{
			surveyport.SafePreviewRowNumber,
			surveyport.SafePreviewSubmittedAt,
			surveyport.SafePreviewScore,
			surveyport.SafePreviewChoiceOptionIDs,
		}
	}
	if !value.OK || value.QuestionnaireID != questionnaireID || value.Limit != request.Limit || value.Offset != request.Offset ||
		value.Total < 0 || int64(value.Offset) > value.Total || value.FileCreated || !value.Deidentified ||
		value.ContainsRawIdentity || value.ContainsFreeText || !value.LocalOnly || value.RealExternalCallExecuted ||
		value.Rows == nil || !samePreviewFields(value.Fields, expectedFields) {
		return false
	}
	expectedRows := int64(value.Limit)
	if remaining := value.Total - int64(value.Offset); remaining < expectedRows {
		expectedRows = remaining
	}
	if int64(len(value.Rows)) != expectedRows || value.HasMore != (int64(value.Offset)+expectedRows < value.Total) {
		return false
	}
	selected := make(map[surveyport.SafeExportPreviewField]bool, len(value.Fields))
	for _, field := range value.Fields {
		selected[field] = true
	}
	for index, row := range value.Rows {
		if selected[surveyport.SafePreviewRowNumber] != (row.RowNumber != nil) ||
			selected[surveyport.SafePreviewSubmittedAt] != (row.SubmittedAt != nil) ||
			selected[surveyport.SafePreviewScore] != (row.Score != nil) ||
			selected[surveyport.SafePreviewChoiceOptionIDs] != (row.ChoiceOptionIDs != nil) {
			return false
		}
		if row.RowNumber != nil && *row.RowNumber != int64(value.Offset)+int64(index)+1 {
			return false
		}
		if row.SubmittedAt != nil && row.SubmittedAt.IsZero() {
			return false
		}
		if row.Score != nil && (math.IsNaN(*row.Score) || math.IsInf(*row.Score, 0)) {
			return false
		}
		if row.ChoiceOptionIDs == nil {
			continue
		}
		for answerIndex, answer := range *row.ChoiceOptionIDs {
			if answer.QuestionID < 1 || !safeChoiceType(answer.QuestionType) || answer.SortOrder < 0 {
				return false
			}
			if answer.QuestionType == surveyport.SingleChoice && len(answer.OptionIDs) > 1 {
				return false
			}
			if answerIndex > 0 {
				previous := (*row.ChoiceOptionIDs)[answerIndex-1]
				if previous.SortOrder > answer.SortOrder || previous.SortOrder == answer.SortOrder && previous.QuestionID >= answer.QuestionID {
					return false
				}
			}
			for optionIndex, optionID := range answer.OptionIDs {
				if optionID < 1 || optionIndex > 0 && answer.OptionIDs[optionIndex-1] >= optionID {
					return false
				}
			}
		}
	}
	return true
}

func samePreviewFields(left, right []surveyport.SafeExportPreviewField) bool {
	if len(left) != len(right) || left == nil || right == nil {
		return false
	}
	seen := make(map[surveyport.SafeExportPreviewField]struct{}, len(left))
	for index, field := range left {
		switch field {
		case surveyport.SafePreviewRowNumber, surveyport.SafePreviewSubmittedAt, surveyport.SafePreviewScore, surveyport.SafePreviewChoiceOptionIDs:
		default:
			return false
		}
		if field != right[index] {
			return false
		}
		if _, duplicate := seen[field]; duplicate {
			return false
		}
		seen[field] = struct{}{}
	}
	return true
}

func safeChoiceType(value surveyport.QuestionType) bool {
	return value == surveyport.SingleChoice || value == surveyport.MultiChoice
}

func authorizeRead(r *http.Request) (int, string) {
	if r == nil {
		return http.StatusUnauthorized, "authentication_required"
	}
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 {
		return http.StatusUnauthorized, "authentication_required"
	}
	if principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps {
		return http.StatusForbidden, "permission_denied"
	}
	authorization, ok := authport.AuthorizationFromContext(r.Context())
	if !ok || authorization.Capability != authport.CapabilityQuestionnairesRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return http.StatusForbidden, "permission_denied"
	}
	return 0, ""
}

func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, surveyapp.ErrInvalidSubmissionPage):
		writeError(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, surveyapp.ErrNotFound):
		writeError(w, http.StatusNotFound, "questionnaire_not_found")
	default:
		writeError(w, http.StatusServiceUnavailable, "survey_read_unavailable")
	}
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
}

type errorBody struct {
	OK    bool `json:"ok"`
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
	LocalOnly                bool `json:"local_only"`
	RealExternalCallExecuted bool `json:"real_external_call_executed"`
}

func writeError(w http.ResponseWriter, status int, code string) {
	body := errorBody{LocalOnly: true, RealExternalCallExecuted: false}
	body.Error.Code = code
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
