package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerListApplication interface {
	List(context.Context, contactapp.CustomerListInput) (contactapp.CustomerListResult, error)
}

type CustomerListHandler struct {
	application customerListApplication
}

func NewCustomerListHandler(application customerListApplication) (*CustomerListHandler, error) {
	if nilCustomerListApplication(application) {
		return nil, errors.New("customer list application is required")
	}
	return &CustomerListHandler{application: application}, nil
}

func (handler *CustomerListHandler) ListCustomers(
	writer http.ResponseWriter,
	request *http.Request,
	params generated.ListCustomersParams,
) {
	if handler == nil || nilCustomerListApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		writeCustomerListError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil), false)
		return
	}

	ownerStaffID, err := customerListOwner(request.Context(), params.OwnerStaffId)
	if err != nil {
		writeCustomerListError(writer, request, err, false)
		return
	}
	input, cursorSupplied, err := customerListInput(params, ownerStaffID)
	if err != nil {
		writeCustomerListError(writer, request, err, cursorSupplied)
		return
	}
	result, err := handler.application.List(request.Context(), input)
	if err != nil {
		writeCustomerListError(writer, request, err, cursorSupplied)
		return
	}
	response, err := customerListResponse(result)
	if err != nil {
		writeCustomerListError(writer, request, err, cursorSupplied)
		return
	}
	writeCustomerListJSON(writer, http.StatusOK, response)
}

func customerListOwner(ctx context.Context, requested *generated.OwnerStaffIDFilter) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomersRead {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(ctx)
	if !ok || principal.AdminUserID < 1 {
		return nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}

	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return cloneGeneratedInt64(requested), nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		if requested != nil && int64(*requested) != authorization.OwnerStaffID {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		ownerStaffID := authorization.OwnerStaffID
		return &ownerStaffID, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func customerListInput(params generated.ListCustomersParams, ownerStaffID *int64) (contactapp.CustomerListInput, bool, error) {
	input := contactapp.CustomerListInput{
		OwnerStaffID:       ownerStaffID,
		StageID:            cloneGeneratedInt64(params.StageId),
		ChannelID:          cloneGeneratedInt64(params.ChannelId),
		TagID:              cloneGeneratedInt64(params.TagId),
		AddedAfter:         cloneGeneratedTime(params.AddedAfter),
		AddedBefore:        cloneGeneratedTime(params.AddedBefore),
		LastInteractAfter:  cloneGeneratedTime(params.LastInteractAfter),
		LastInteractBefore: cloneGeneratedTime(params.LastInteractBefore),
	}
	if params.Cursor != nil {
		input.Cursor = string(*params.Cursor)
	}
	if params.Keyword != nil {
		input.Keyword = string(*params.Keyword)
	}
	if params.IsDeleted != nil {
		input.IsDeleted = bool(*params.IsDeleted)
	}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > int(contactapp.CustomerListMaximumLimit) {
			return contactapp.CustomerListInput{}, params.Cursor != nil, platformhttp.NewError(platformhttp.CodeMalformedRequest, contactapp.ErrInvalidCustomerListQuery)
		}
		input.Limit = int32(*params.Limit)
	}
	return input, params.Cursor != nil, nil
}

func customerListResponse(result contactapp.CustomerListResult) (generated.CustomerListResponse, error) {
	if result.Total < int64(len(result.Items)) || result.Total > contactapp.CustomerListExactTotalCap || result.Watermark.IsZero() ||
		(result.TotalIsEstimate && result.Total != contactapp.CustomerListExactTotalCap) ||
		(result.NextCursor != nil && (*result.NextCursor == "" || len(result.Items) == 0)) {
		return generated.CustomerListResponse{}, errors.New("customer list application returned an invalid result")
	}
	items := make([]generated.Customer, 0, len(result.Items))
	for _, item := range result.Items {
		converted, err := customerResponse(item)
		if err != nil {
			return generated.CustomerListResponse{}, err
		}
		items = append(items, converted)
	}
	return generated.CustomerListResponse{
		Items: items, NextCursor: cloneString(result.NextCursor), Total: result.Total,
		TotalIsEstimate: result.TotalIsEstimate, Watermark: result.Watermark.UTC(),
	}, nil
}

func customerResponse(item contactapp.CustomerRecord) (generated.Customer, error) {
	if item.ID < 1 || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.CreatedAt.After(item.UpdatedAt) || invalidOptionalID(item.StageID) ||
		invalidOptionalID(item.OwnerStaffID) || invalidOptionalID(item.ChannelID) || invalidOptionalTime(item.AddedAt) ||
		invalidOptionalTime(item.LastInteractAt) || invalidURI(item.AvatarURL) {
		return generated.Customer{}, errors.New("customer list application returned an invalid customer")
	}
	extra, err := decodeJSONObject(item.Extra)
	if err != nil {
		return generated.Customer{}, err
	}
	var gender *int32
	if item.Gender != nil {
		value := int32(*item.Gender)
		gender = &value
	}
	return generated.Customer{
		Id: int64(item.ID), Name: item.Name, AvatarUrl: cloneString(item.AvatarURL), Gender: gender,
		StageId: cloneInt64(item.StageID), OwnerStaffId: cloneInt64(item.OwnerStaffID), ChannelId: cloneInt64(item.ChannelID),
		AddedAt: cloneTimeUTC(item.AddedAt), LastInteractAt: cloneTimeUTC(item.LastInteractAt), IsDeleted: item.IsDeleted,
		Extra: extra, CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
	}, nil
}

func decodeJSONObject(raw json.RawMessage) (map[string]interface{}, error) {
	if !contactapp.IsChannelNeutralCustomerExtra(raw) {
		return nil, errors.New("customer extra is not channel neutral")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, errors.New("customer extra is not an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value map[string]interface{}
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("customer extra is invalid")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("customer extra has trailing data")
	}
	return value, nil
}

func writeCustomerListJSON(writer http.ResponseWriter, status int, value any) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeCustomerListError(writer http.ResponseWriter, request *http.Request, err error, cursorSupplied bool) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	if errors.Is(err, contactapp.ErrInvalidCustomerListQuery) {
		code = platformhttp.CodeMalformedRequest
		if cursorSupplied {
			code = platformhttp.CodeCursorInvalid
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilCustomerListApplication(application customerListApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func cloneGeneratedInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneGeneratedTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimeUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func invalidOptionalID(value *int64) bool {
	return value != nil && *value < 1
}

func invalidOptionalTime(value *time.Time) bool {
	return value != nil && value.IsZero()
}

func invalidURI(value *string) bool {
	if value == nil {
		return false
	}
	parsed, err := url.Parse(*value)
	return err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https")
}
