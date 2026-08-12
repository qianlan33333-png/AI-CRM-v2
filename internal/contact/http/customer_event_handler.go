package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerEventApplication interface {
	List(context.Context, contactapp.CustomerEventInput) (contactapp.CustomerEventResult, error)
}

type CustomerEventHandler struct {
	application customerEventApplication
}

func NewCustomerEventHandler(application customerEventApplication) (*CustomerEventHandler, error) {
	if nilCustomerEventApplication(application) {
		return nil, errors.New("customer event application is required")
	}
	return &CustomerEventHandler{application: application}, nil
}

func (handler *CustomerEventHandler) ListCustomerEvents(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	params generated.ListCustomerEventsParams,
) {
	if handler == nil || nilCustomerEventApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		writeCustomerEventError(writer, request, contactapp.ErrCustomerEventsUnavailable, false)
		return
	}
	if customerID <= 0 {
		writeCustomerEventError(writer, request, contactapp.ErrCustomerNotFound, false)
		return
	}

	ownerStaffID, err := customerEventOwner(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	input, cursorSupplied, err := customerEventInput(customerID, params, ownerStaffID)
	if err != nil {
		writeCustomerEventError(writer, request, err, cursorSupplied)
		return
	}
	result, err := handler.application.List(request.Context(), input)
	if err != nil {
		writeCustomerEventError(writer, request, err, cursorSupplied)
		return
	}
	response, err := customerEventResponse(contactport.CustomerID(customerID), result)
	if err != nil {
		writeCustomerEventError(writer, request, contactapp.ErrCustomerEventsUnavailable, cursorSupplied)
		return
	}
	writeCustomerListJSON(writer, http.StatusOK, response)
}

func customerEventOwner(ctx context.Context) (*int64, error) {
	authorization, ok := authport.AuthorizationFromContext(ctx)
	if !ok || authorization.Capability != authport.CapabilityCustomerEventsRead {
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
		return nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil ||
			*principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		ownerStaffID := authorization.OwnerStaffID
		return &ownerStaffID, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func customerEventInput(
	customerID generated.CustomerID,
	params generated.ListCustomerEventsParams,
	ownerStaffID *int64,
) (contactapp.CustomerEventInput, bool, error) {
	input := contactapp.CustomerEventInput{CustomerID: contactport.CustomerID(customerID), OwnerStaffID: ownerStaffID}
	if params.Cursor != nil {
		input.Cursor = string(*params.Cursor)
	}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > int(contactapp.CustomerListMaximumLimit) {
			return contactapp.CustomerEventInput{}, params.Cursor != nil, contactapp.ErrInvalidCustomerEventQuery
		}
		input.Limit = int32(*params.Limit)
	}
	return input, params.Cursor != nil, nil
}

func customerEventResponse(expectedCustomerID contactport.CustomerID, result contactapp.CustomerEventResult) (generated.CustomerEventListResponse, error) {
	if result.Items == nil || (result.NextCursor != nil && (*result.NextCursor == "" || len(result.Items) == 0)) {
		return generated.CustomerEventListResponse{}, errors.New("customer event application returned an invalid result")
	}
	items := make([]generated.CustomerEvent, 0, len(result.Items))
	for _, item := range result.Items {
		payload, err := decodeCustomerEventPayload(item.Payload)
		if err != nil || expectedCustomerID <= 0 || item.ID <= 0 || item.CustomerID != expectedCustomerID ||
			item.EventType == "" || item.Actor == "" || item.OccurredAt.IsZero() {
			return generated.CustomerEventListResponse{}, errors.New("customer event application returned an invalid item")
		}
		items = append(items, generated.CustomerEvent{
			Id: item.ID, CustomerId: int64(item.CustomerID), EventType: item.EventType,
			Payload: payload, Actor: item.Actor, OccurredAt: item.OccurredAt.UTC(),
		})
	}
	return generated.CustomerEventListResponse{Items: items, NextCursor: cloneString(result.NextCursor)}, nil
}

func decodeCustomerEventPayload(raw json.RawMessage) (map[string]interface{}, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, errors.New("customer event payload is not an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, errors.New("customer event payload is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("customer event payload has trailing data")
	}
	return payload, nil
}

func writeCustomerEventError(writer http.ResponseWriter, request *http.Request, err error, cursorSupplied bool) {
	if request == nil {
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrCustomerNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrInvalidCustomerEventQuery):
		code = platformhttp.CodeMalformedRequest
		if cursorSupplied {
			code = platformhttp.CodeCursorInvalid
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilCustomerEventApplication(application customerEventApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
