package main

import (
	"errors"
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func (handler *candidateHandler) listCustomersByMobile(
	writer http.ResponseWriter,
	request *http.Request,
	params api.ListCustomersParams,
) {
	if handler == nil || handler.customers == nil || nilLegacyDependency(handler.customerIdentity) || request == nil || params.Mobile == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed))
		return
	}
	if _, err := customerReadOwner(request); err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	ref := identityport.IDRef{
		Kind: identityport.KindPhone, Scope: "phone:e164", Value: *params.Mobile,
		Assurance: identityport.AssuranceVerified, Source: "customer.list.mobile",
	}
	normalized, normalizeErr := identityapp.Normalize(ref)
	if normalizeErr != nil || normalized.NormalizedValue != *params.Mobile {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrInvalidIdentity))
		return
	}
	result, err := handler.customerIdentity.Resolve(request.Context(), ref)
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, identityapp.ErrInvalidIdentity) {
			code = platformhttp.CodeMalformedRequest
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	params.Mobile = nil
	switch result.Status {
	case identityport.ResolveNotFound:
		if result.CustomerID != 0 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed))
			return
		}
		handler.customers.ListCustomersForResolvedCustomer(writer, request, params, nil)
	case identityport.ResolveFound:
		if result.CustomerID <= 0 {
			platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed))
			return
		}
		customerID := contactport.CustomerID(result.CustomerID)
		handler.customers.ListCustomersForResolvedCustomer(writer, request, params, &customerID)
	default:
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed))
	}
}

func (handler *candidateHandler) ListCustomerSurveyAnswers(
	writer http.ResponseWriter,
	request *http.Request,
	customerID api.CustomerID,
	params api.ListCustomerSurveyAnswersParams,
) {
	if handler == nil || nilLegacyDependency(handler.customerDetailReader) || nilLegacyDependency(handler.customerSurveyAnswers) || request == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, surveyapp.ErrCustomerAnswersUnavailable))
		return
	}
	ownerStaffID, err := customerReadOwner(request)
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	if customerID < 1 {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, contactapp.ErrCustomerNotFound))
		return
	}
	_, err = handler.customerDetailReader.Get(request.Context(), contactapp.CustomerDetailInput{
		ID: contactport.CustomerID(customerID), OwnerStaffID: ownerStaffID,
	})
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, contactapp.ErrCustomerNotFound) || errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery) {
			code = platformhttp.CodeNotFound
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	var limit int32
	if params.Limit != nil {
		limit = *params.Limit
	}
	page, err := handler.customerSurveyAnswers.ListCustomerSurveyAnswers(request.Context(), contactport.CustomerID(customerID), limit)
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, surveyapp.ErrInvalidCustomerAnswerQuery) {
			code = platformhttp.CodeMalformedRequest
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return
	}
	if page.CustomerID != contactport.CustomerID(customerID) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, surveyapp.ErrCustomerAnswersUnavailable))
		return
	}
	response, err := customerSurveyAnswerResponse(page)
	if err != nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, err))
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func customerReadOwner(request *http.Request) (*int64, error) {
	if request == nil {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		ownerStaffID := authorization.OwnerStaffID
		return &ownerStaffID, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func customerSurveyAnswerResponse(page surveyport.CustomerSurveyAnswerPage) (api.CustomerSurveyAnswerResponse, error) {
	if page.CustomerID <= 0 || page.Limit < 1 || page.Limit > surveyapp.CustomerAnswerMaximumLimit ||
		page.ScanLimit != surveyapp.CustomerAnswerScanLimit || page.ScannedCount < 0 || page.ScannedCount > page.ScanLimit ||
		page.MatchedCount < int32(len(page.Items)) || page.MatchedCount > page.ScannedCount || len(page.Items) > int(page.Limit) ||
		(page.ScanTruncated && page.ScannedCount != page.ScanLimit) || (page.ResultTruncated && page.MatchedCount <= int32(len(page.Items))) {
		return api.CustomerSurveyAnswerResponse{}, surveyapp.ErrCustomerAnswersUnavailable
	}
	items := make([]api.CustomerSurveyAnswerItem, len(page.Items))
	for index, item := range page.Items {
		if item.SubmissionID <= 0 || item.QuestionnaireID <= 0 || item.SubmittedAt.IsZero() {
			return api.CustomerSurveyAnswerResponse{}, surveyapp.ErrCustomerAnswersUnavailable
		}
		answers := make([]api.CustomerSurveyChoiceAnswer, len(item.ChoiceAnswers))
		for answerIndex, answer := range item.ChoiceAnswers {
			questionType := api.CustomerSurveyChoiceAnswerQuestionType(answer.QuestionType)
			if answer.QuestionID <= 0 || answer.SortOrder < 0 || !questionType.Valid() {
				return api.CustomerSurveyAnswerResponse{}, surveyapp.ErrCustomerAnswersUnavailable
			}
			answers[answerIndex] = api.CustomerSurveyChoiceAnswer{
				QuestionId: answer.QuestionID, QuestionType: questionType, SortOrder: int32(answer.SortOrder),
				OptionIds: append([]int64(nil), answer.OptionIDs...),
			}
		}
		items[index] = api.CustomerSurveyAnswerItem{
			SubmissionId: item.SubmissionID, QuestionnaireId: int64(item.QuestionnaireID),
			SubmittedAt: item.SubmittedAt.UTC(), Score: item.Score, ChoiceAnswers: answers,
		}
	}
	return api.CustomerSurveyAnswerResponse{
		CustomerId: int64(page.CustomerID), Items: items, Limit: page.Limit,
		ScanLimit: page.ScanLimit, ScannedCount: page.ScannedCount, MatchedCount: page.MatchedCount,
		ScanTruncated: page.ScanTruncated, ResultTruncated: page.ResultTruncated,
		NonAtomicSnapshot: true, IdentityValuesIncluded: false, FreeTextIncluded: false, RealExternalCallExecuted: false,
	}, nil
}
