package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func (handler *candidateHandler) GetPublicSurveyDefinition(writer http.ResponseWriter, request *http.Request, slug api.PublicSurveySlug) {
	if handler == nil || handler.surveyPublic == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	handler.surveyPublic.GetDefinition(writer, request, string(slug))
}

func (handler *candidateHandler) SubmitPublicSurvey(writer http.ResponseWriter, request *http.Request, slug api.PublicSurveySlug) {
	if handler == nil || handler.surveyPublic == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	handler.surveyPublic.Submit(writer, request, string(slug))
}

func (handler *candidateHandler) QueryPublicSurveySubmissionResult(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyPublic == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	handler.surveyPublic.QueryResult(writer, request)
}

func (handler *candidateHandler) PublishQuestionnairePublicDefinition(writer http.ResponseWriter, request *http.Request, questionnaireID api.QuestionnaireID, _ api.PublishQuestionnairePublicDefinitionParams) {
	actor, ok := surveyPublicActor(request)
	if handler == nil || handler.surveyPublic == nil || !ok {
		writeSurveyPublicAdminUnavailable(writer, request)
		return
	}
	handler.surveyPublic.Publish(writer, request, surveyport.ID(questionnaireID), actor)
}

func (handler *candidateHandler) DisableQuestionnairePublicDefinition(writer http.ResponseWriter, request *http.Request, questionnaireID api.QuestionnaireID, _ api.DisableQuestionnairePublicDefinitionParams) {
	actor, ok := surveyPublicActor(request)
	if handler == nil || handler.surveyPublic == nil || !ok {
		writeSurveyPublicAdminUnavailable(writer, request)
		return
	}
	handler.surveyPublic.Disable(writer, request, surveyport.ID(questionnaireID), actor)
}

func (handler *candidateHandler) GetQuestionnairePublicAnalytics(writer http.ResponseWriter, request *http.Request, questionnaireID api.QuestionnaireID, params api.GetQuestionnairePublicAnalyticsParams) {
	if handler == nil || handler.surveyPublic == nil {
		writeSurveyPublicAdminUnavailable(writer, request)
		return
	}
	var definitionVersion int64
	if params.DefinitionVersion != nil {
		definitionVersion = *params.DefinitionVersion
	}
	handler.surveyPublic.Analytics(writer, request, surveyport.ID(questionnaireID), definitionVersion)
}

func (handler *candidateHandler) GetPublicSurveyPage(writer http.ResponseWriter, request *http.Request, slug api.PublicSurveySlug) {
	if handler == nil || handler.surveyPublic == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	handler.surveyPublic.Page(writer, request, string(slug))
}

func surveyPublicActor(request *http.Request) (int64, bool) {
	if request == nil {
		return 0, false
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	return principal.AdminUserID, ok && principal.AdminUserID > 0
}

func writeSurveyPublicUnavailable(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusServiceUnavailable)
	_, _ = writer.Write([]byte("{\"code\":\"unavailable\"}\n"))
}

func writeSurveyPublicAdminUnavailable(writer http.ResponseWriter, request *http.Request) {
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
}

// deriveSurveyPublicKeys keeps result tokens, anonymous cookies, and source
// abuse budgets in separate cryptographic domains. An absent all-zero root
// stays disabled: the Survey service and edge handler then fail closed.
func deriveSurveyPublicKeys(root []byte) (token, cookie, abuse [32]byte) {
	if len(root) != 32 || allZero(root) {
		return token, cookie, abuse
	}
	return surveySubkey(root, "result.v1"), surveySubkey(root, "cookie.v1"), surveySubkey(root, "abuse.v1")
}

func surveySubkey(root []byte, label string) (out [32]byte) {
	mac := hmac.New(sha256.New, root)
	_, _ = mac.Write([]byte("aicrm.survey.public.key.v1\x00" + label))
	copy(out[:], mac.Sum(nil))
	return out
}

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
