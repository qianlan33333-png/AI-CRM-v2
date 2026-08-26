package main

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

const legacyCustomerProfileQuestionnaireAnswersPath = "/api/admin/customers/profile/questionnaire-answers"

var (
	errInvalidLegacyCustomerProfileQuestionnaireAnswersQuery    = errors.New("invalid customer profile questionnaire answers query")
	errUnsupportedLegacyCustomerProfileQuestionnaireAnswersHint = errors.New("unsupported customer profile questionnaire answers identity hint")
)

// legacyCustomerProfileQuestionnaireAnswersHandler adapts the existing local
// Survey read model. The current Survey domain has no assessment engine, so
// latest_assessment_result is explicitly null rather than a derived score.
type legacyCustomerProfileQuestionnaireAnswersHandler struct {
	customerDetail customerDetailApplication
	identity       identityResolveApplication
	unionID        legacyMessageArchiveUnionResolver
	answers        surveyport.CustomerSurveyAnswerReader
	weComCorpID    string
}

type legacyCustomerProfileQuestionnaireAnswersQuery struct {
	UnionID        string
	ExternalUserID string
	Mobile         string
}

type legacyCustomerProfileQuestionnaireChoiceAnswer struct {
	QuestionID   int64   `json:"question_id"`
	QuestionType string  `json:"question_type"`
	SortOrder    int     `json:"sort_order"`
	OptionIDs    []int64 `json:"option_ids"`
}

type legacyCustomerProfileQuestionnaireAnswer struct {
	SubmissionID    int64                                            `json:"submission_id"`
	QuestionnaireID int64                                            `json:"questionnaire_id"`
	SubmittedAt     string                                           `json:"submitted_at"`
	Score           float64                                          `json:"score"`
	ChoiceAnswers   []legacyCustomerProfileQuestionnaireChoiceAnswer `json:"choice_answers"`
}

type legacyCustomerProfileQuestionnaireAnswersSuccess struct {
	OK                       bool                                       `json:"ok"`
	Answers                  []legacyCustomerProfileQuestionnaireAnswer `json:"answers"`
	Count                    int                                        `json:"count"`
	LatestAssessmentResult   any                                        `json:"latest_assessment_result"`
	AssessmentStatus         string                                     `json:"assessment_status"`
	SourceStatus             string                                     `json:"source_status"`
	RouteOwner               string                                     `json:"route_owner"`
	RealExternalCallExecuted bool                                       `json:"real_external_call_executed"`
}

type legacyCustomerProfileQuestionnaireAnswersError struct {
	OK                       bool   `json:"ok"`
	StatusCode               int    `json:"status_code"`
	ErrorCode                string `json:"error_code"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
}

func newLegacyCustomerProfileQuestionnaireAnswersHandler(
	customerDetail customerDetailApplication,
	identity identityResolveApplication,
	unionID legacyMessageArchiveUnionResolver,
	answers surveyport.CustomerSurveyAnswerReader,
	weComCorpID string,
) (*legacyCustomerProfileQuestionnaireAnswersHandler, error) {
	if nilLegacyDependency(customerDetail) || nilLegacyDependency(identity) || nilLegacyDependency(unionID) ||
		nilLegacyDependency(answers) || strings.TrimSpace(weComCorpID) == "" {
		return nil, surveyapp.ErrCustomerAnswersUnavailable
	}
	return &legacyCustomerProfileQuestionnaireAnswersHandler{
		customerDetail: customerDetail, identity: identity, unionID: unionID, answers: answers, weComCorpID: strings.TrimSpace(weComCorpID),
	}, nil
}

func (handler *legacyCustomerProfileQuestionnaireAnswersHandler) Get(writer http.ResponseWriter, request *http.Request) {
	setLegacyCustomerProfileQuestionnaireAnswersSecurityHeaders(writer)
	if request != nil && request.Method != http.MethodGet {
		writeLegacyCustomerProfileQuestionnaireAnswersMethodNotAllowed(writer)
		return
	}
	if !legacyCustomerProfileMessagesAuthorized(request) {
		writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusForbidden, "customer_profile_questionnaire_answers_forbidden")
		return
	}
	if handler == nil || request == nil || request.URL == nil {
		writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer)
		return
	}
	query, err := parseLegacyCustomerProfileQuestionnaireAnswersQuery(request.URL.RawQuery)
	if errors.Is(err, errUnsupportedLegacyCustomerProfileQuestionnaireAnswersHint) {
		writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusUnprocessableEntity, "unsupported_identity_hint")
		return
	}
	if err != nil {
		writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusUnprocessableEntity, "invalid_identity_hint")
		return
	}
	customerID, status := handler.resolveCustomerID(request.Context(), query)
	if status != 0 {
		if status == http.StatusNotFound {
			writeLegacyCustomerProfileQuestionnaireAnswersNotFound(writer)
		} else if status == http.StatusConflict {
			writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusConflict, "identity_hint_conflict")
		} else if status == http.StatusUnprocessableEntity {
			writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusUnprocessableEntity, "invalid_identity_hint")
		} else {
			writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer)
		}
		return
	}
	if _, err = handler.customerDetail.Get(request.Context(), contactapp.CustomerDetailInput{ID: customerID}); errors.Is(err, contactapp.ErrCustomerNotFound) {
		writeLegacyCustomerProfileQuestionnaireAnswersNotFound(writer)
		return
	} else if err != nil {
		writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer)
		return
	}
	page, err := handler.answers.ListCustomerSurveyAnswers(request.Context(), customerID, surveyapp.CustomerAnswerMaximumLimit)
	if err != nil || page.CustomerID != customerID || page.ResultTruncated || page.ScanTruncated {
		writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer)
		return
	}
	answers, ok := legacyCustomerProfileQuestionnaireAnswers(page)
	if !ok {
		writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer)
		return
	}
	writeLegacyCustomerProfileQuestionnaireAnswersJSON(writer, http.StatusOK, legacyCustomerProfileQuestionnaireAnswersSuccess{
		OK: true, Answers: answers, Count: len(answers), LatestAssessmentResult: nil, AssessmentStatus: "v2_assessment_unavailable",
		SourceStatus: "survey_customer_answer_read_model", RouteOwner: "ai_crm_v2", RealExternalCallExecuted: false,
	})
}

func parseLegacyCustomerProfileQuestionnaireAnswersQuery(rawQuery string) (legacyCustomerProfileQuestionnaireAnswersQuery, error) {
	if !utf8.ValidString(rawQuery) {
		return legacyCustomerProfileQuestionnaireAnswersQuery{}, errInvalidLegacyCustomerProfileQuestionnaireAnswersQuery
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return legacyCustomerProfileQuestionnaireAnswersQuery{}, errInvalidLegacyCustomerProfileQuestionnaireAnswersQuery
	}
	if _, exists := values["user_id"]; exists {
		return legacyCustomerProfileQuestionnaireAnswersQuery{}, errUnsupportedLegacyCustomerProfileQuestionnaireAnswersHint
	}
	for key, entries := range values {
		if (key != "unionid" && key != "external_userid" && key != "mobile") || len(entries) != 1 || !validLegacyCustomerProfileQuestionnaireAnswersHint(entries[0]) {
			return legacyCustomerProfileQuestionnaireAnswersQuery{}, errInvalidLegacyCustomerProfileQuestionnaireAnswersQuery
		}
	}
	query := legacyCustomerProfileQuestionnaireAnswersQuery{}
	if value, ok := values["unionid"]; ok {
		query.UnionID = strings.TrimSpace(value[0])
	}
	if value, ok := values["external_userid"]; ok {
		query.ExternalUserID = strings.TrimSpace(value[0])
	}
	if value, ok := values["mobile"]; ok {
		query.Mobile = strings.TrimSpace(value[0])
	}
	if query.UnionID == "" && query.ExternalUserID == "" && query.Mobile == "" {
		return legacyCustomerProfileQuestionnaireAnswersQuery{}, errInvalidLegacyCustomerProfileQuestionnaireAnswersQuery
	}
	return query, nil
}

func validLegacyCustomerProfileQuestionnaireAnswersHint(value string) bool {
	return utf8.ValidString(value) && len(value) <= 1024 && strings.TrimSpace(value) != "" && !strings.ContainsFunc(value, unicode.IsControl)
}

func (handler *legacyCustomerProfileQuestionnaireAnswersHandler) resolveCustomerID(ctx context.Context, query legacyCustomerProfileQuestionnaireAnswersQuery) (contactport.CustomerID, int) {
	if handler == nil || nilLegacyDependency(handler.identity) || nilLegacyDependency(handler.unionID) || handler.weComCorpID == "" {
		return 0, http.StatusServiceUnavailable
	}
	results := make([]identityport.ResolveResult, 0, 3)
	if query.UnionID != "" {
		result, err := handler.unionID.ResolveUnionID(ctx, query.UnionID)
		if err != nil || !validLegacyCustomerProfileTagsResolution(result) {
			return 0, http.StatusServiceUnavailable
		}
		results = append(results, result)
	}
	if query.ExternalUserID != "" {
		result, err := handler.identity.Resolve(ctx, identityport.IDRef{Kind: identityport.KindWeComExternalUserID, Scope: "wecom-corp:" + handler.weComCorpID,
			Value: query.ExternalUserID, Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-questionnaire-answers"})
		if err != nil || !validLegacyCustomerProfileTagsResolution(result) {
			return 0, http.StatusServiceUnavailable
		}
		results = append(results, result)
	}
	if query.Mobile != "" {
		ref, err := identityapp.Normalize(identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: query.Mobile,
			Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-questionnaire-answers"})
		if err != nil || ref.NormalizedValue != query.Mobile {
			return 0, http.StatusUnprocessableEntity
		}
		result, err := handler.identity.Resolve(ctx, identityport.IDRef{Kind: ref.Kind, Scope: ref.Scope, Value: ref.NormalizedValue,
			Assurance: identityport.AssuranceVerified, Source: "legacy-customer-profile-questionnaire-answers"})
		if err != nil || !validLegacyCustomerProfileTagsResolution(result) {
			return 0, http.StatusServiceUnavailable
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return 0, http.StatusUnprocessableEntity
	}
	customerID := results[0].CustomerID
	for _, result := range results {
		if result.Status == identityport.ResolveNotFound && len(results) == 1 {
			return 0, http.StatusNotFound
		}
		if result.Status != identityport.ResolveFound || result.CustomerID != customerID || customerID <= 0 {
			return 0, http.StatusConflict
		}
	}
	return customerID, 0
}

func legacyCustomerProfileQuestionnaireAnswers(page surveyport.CustomerSurveyAnswerPage) ([]legacyCustomerProfileQuestionnaireAnswer, bool) {
	if page.CustomerID <= 0 || page.Limit != surveyapp.CustomerAnswerMaximumLimit || page.ScanLimit != surveyapp.CustomerAnswerScanLimit ||
		page.ScannedCount < 0 || page.MatchedCount != int32(len(page.Items)) || len(page.Items) > int(page.Limit) {
		return nil, false
	}
	answers := make([]legacyCustomerProfileQuestionnaireAnswer, len(page.Items))
	seenSubmissions := make(map[int64]struct{}, len(page.Items))
	var previous time.Time
	for index, item := range page.Items {
		if item.SubmissionID <= 0 || item.QuestionnaireID <= 0 || item.SubmittedAt.IsZero() || math.IsNaN(item.Score) || math.IsInf(item.Score, 0) ||
			(!previous.IsZero() && previous.Before(item.SubmittedAt)) {
			return nil, false
		}
		if _, duplicate := seenSubmissions[item.SubmissionID]; duplicate {
			return nil, false
		}
		seenSubmissions[item.SubmissionID] = struct{}{}
		previous = item.SubmittedAt
		seenQuestions := make(map[int64]struct{}, len(item.ChoiceAnswers))
		choiceAnswers := make([]legacyCustomerProfileQuestionnaireChoiceAnswer, len(item.ChoiceAnswers))
		for answerIndex, answer := range item.ChoiceAnswers {
			if answer.QuestionID <= 0 || answer.SortOrder < 0 || (answer.QuestionType != surveyport.SingleChoice && answer.QuestionType != surveyport.MultiChoice) {
				return nil, false
			}
			if _, duplicate := seenQuestions[answer.QuestionID]; duplicate {
				return nil, false
			}
			seenQuestions[answer.QuestionID] = struct{}{}
			seenOptions := make(map[int64]struct{}, len(answer.OptionIDs))
			for _, optionID := range answer.OptionIDs {
				if optionID <= 0 {
					return nil, false
				}
				if _, duplicate := seenOptions[optionID]; duplicate {
					return nil, false
				}
				seenOptions[optionID] = struct{}{}
			}
			choiceAnswers[answerIndex] = legacyCustomerProfileQuestionnaireChoiceAnswer{QuestionID: answer.QuestionID, QuestionType: string(answer.QuestionType), SortOrder: answer.SortOrder, OptionIDs: append([]int64(nil), answer.OptionIDs...)}
		}
		answers[index] = legacyCustomerProfileQuestionnaireAnswer{SubmissionID: item.SubmissionID, QuestionnaireID: int64(item.QuestionnaireID),
			SubmittedAt: item.SubmittedAt.UTC().Format(time.RFC3339), Score: item.Score, ChoiceAnswers: choiceAnswers}
	}
	return answers, true
}

func writeLegacyCustomerProfileQuestionnaireAnswersError(writer http.ResponseWriter, status int, code string) {
	switch status {
	case http.StatusForbidden:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeUnauthorized)
	case http.StatusNotFound:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeNotFound)
	case http.StatusConflict:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeConflict)
	case http.StatusUnprocessableEntity:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeValidationFailed)
	default:
		platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	}
	writeLegacyCustomerProfileQuestionnaireAnswersJSON(writer, status, legacyCustomerProfileQuestionnaireAnswersError{OK: false, StatusCode: status, ErrorCode: code})
}

func writeLegacyCustomerProfileQuestionnaireAnswersNotFound(writer http.ResponseWriter) {
	writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusNotFound, "customer_not_found")
}

func writeLegacyCustomerProfileQuestionnaireAnswersUnavailable(writer http.ResponseWriter) {
	writeLegacyCustomerProfileQuestionnaireAnswersError(writer, http.StatusServiceUnavailable, "customer_profile_questionnaire_answers_unavailable")
}

func writeLegacyCustomerProfileQuestionnaireAnswersMethodNotAllowed(writer http.ResponseWriter) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func setLegacyCustomerProfileQuestionnaireAnswersSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeLegacyCustomerProfileQuestionnaireAnswersJSON(writer http.ResponseWriter, status int, value any) {
	writeJSON(legacyCustomerProfileQuestionnaireAnswersHeaderWriter{ResponseWriter: writer}, status, value)
}

type legacyCustomerProfileQuestionnaireAnswersHeaderWriter struct{ http.ResponseWriter }

func (writer legacyCustomerProfileQuestionnaireAnswersHeaderWriter) WriteHeader(status int) {
	setLegacyCustomerProfileQuestionnaireAnswersSecurityHeaders(writer.ResponseWriter)
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyCustomerProfileQuestionnaireAnswersHeaderWriter) Write(payload []byte) (int, error) {
	setLegacyCustomerProfileQuestionnaireAnswersSecurityHeaders(writer.ResponseWriter)
	return writer.ResponseWriter.Write(payload)
}
