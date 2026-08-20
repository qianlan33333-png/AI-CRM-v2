package http

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type mergeHistoryApplication interface {
	ListCustomerMergeHistory(context.Context, contactport.CustomerID, string, int32) (identityport.CustomerMergeHistoryPage, error)
}

type MergeHistoryHandler struct {
	application mergeHistoryApplication
}

// CustomerMergeHistoryQuery is the domain-local adapter boundary for the
// central generated route. Cursor parsing remains owned by the application.
type CustomerMergeHistoryQuery struct {
	Cursor string
	Limit  int32
}

type customerMergeHistoryItemResponse struct {
	MergeAuditID      int64     `json:"merge_audit_id"`
	PrimaryCustomerID int64     `json:"primary_customer_id"`
	MergedCustomerID  int64     `json:"merged_customer_id"`
	Mode              string    `json:"mode"`
	PolicyVersion     string    `json:"policy_version"`
	MergedAt          time.Time `json:"merged_at"`
}

type customerMergeHistoryPageResponse struct {
	CustomerID                  int64                              `json:"customer_id"`
	Scope                       string                             `json:"scope"`
	Items                       []customerMergeHistoryItemResponse `json:"items"`
	NextCursor                  *string                            `json:"next_cursor"`
	IdentityValuesIncluded      bool                               `json:"identity_values_included"`
	OperatorIdentifiersIncluded bool                               `json:"operator_identifiers_included"`
	ChatContentIncluded         bool                               `json:"chat_content_included"`
	RealExternalCallExecuted    bool                               `json:"real_external_call_executed"`
}

func NewMergeHistoryHandler(application mergeHistoryApplication) (*MergeHistoryHandler, error) {
	if nilMergeHistoryApplication(application) {
		return nil, identityapp.ErrCustomerMergeHistoryUnavailable
	}
	return &MergeHistoryHandler{application: application}, nil
}

func (handler *MergeHistoryHandler) GetCustomerMergeHistory(
	writer http.ResponseWriter,
	request *http.Request,
	customerID contactport.CustomerID,
	query CustomerMergeHistoryQuery,
) {
	if err := handler.authorize(request); err != nil {
		writeMergeHistoryError(writer, request, err, query.Cursor != "")
		return
	}
	page, err := handler.application.ListCustomerMergeHistory(request.Context(), customerID, query.Cursor, query.Limit)
	if err != nil {
		writeMergeHistoryError(writer, request, err, query.Cursor != "")
		return
	}
	response, err := mergeHistoryResponse(customerID, page)
	if err != nil {
		writeMergeHistoryError(writer, request, err, false)
		return
	}
	writeReviewJSON(writer, http.StatusOK, response)
}

func (handler *MergeHistoryHandler) authorize(request *http.Request) error {
	if handler == nil || nilMergeHistoryApplication(handler.application) || request == nil {
		return platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrCustomerMergeHistoryUnavailable)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityIdentityReviewRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID <= 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return nil
}

func mergeHistoryResponse(expected contactport.CustomerID, page identityport.CustomerMergeHistoryPage) (customerMergeHistoryPageResponse, error) {
	if expected <= 0 || page.CustomerID != expected || len(page.Items) > int(identityapp.CustomerMergeHistoryMaximumLimit) ||
		(page.NextCursor != "" && (len(page.NextCursor) > 512 || strings.TrimSpace(page.NextCursor) != page.NextCursor)) {
		return customerMergeHistoryPageResponse{}, identityapp.ErrCustomerMergeHistoryUnavailable
	}
	response := customerMergeHistoryPageResponse{
		CustomerID: int64(expected), Scope: "connected_component",
		Items: make([]customerMergeHistoryItemResponse, 0, len(page.Items)),
	}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}
	previousID := int64(0)
	for index, item := range page.Items {
		if item.MergeAuditID <= 0 || item.PrimaryCustomerID <= 0 || item.MergedCustomerID <= 0 || item.PrimaryCustomerID == item.MergedCustomerID ||
			(index > 0 && previousID <= item.MergeAuditID) || (item.Mode != "auto" && item.Mode != "manual") ||
			!safeMergeHistoryText(item.PolicyVersion) || item.MergedAt.IsZero() {
			return customerMergeHistoryPageResponse{}, identityapp.ErrCustomerMergeHistoryUnavailable
		}
		previousID = item.MergeAuditID
		response.Items = append(response.Items, customerMergeHistoryItemResponse{
			MergeAuditID: item.MergeAuditID, PrimaryCustomerID: int64(item.PrimaryCustomerID), MergedCustomerID: int64(item.MergedCustomerID),
			Mode: item.Mode, PolicyVersion: item.PolicyVersion, MergedAt: item.MergedAt.UTC(),
		})
	}
	return response, nil
}

func safeMergeHistoryText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 200 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func writeMergeHistoryError(writer http.ResponseWriter, request *http.Request, err error, cursor bool) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	if errors.Is(err, identityapp.ErrCustomerMergeHistoryInvalid) {
		code = platformhttp.CodeValidationFailed
		if cursor {
			code = platformhttp.CodeCursorInvalid
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilMergeHistoryApplication(application mergeHistoryApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
