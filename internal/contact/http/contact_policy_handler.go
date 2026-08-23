package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type contactPolicyApplication interface {
	Get(context.Context, contactport.CustomerID) (contactapp.ContactPolicy, error)
	Set(context.Context, contactapp.SetContactPolicyCommand) (contactapp.ContactPolicy, error)
	Clear(context.Context, contactapp.ClearContactPolicyCommand) (contactapp.ContactPolicy, error)
}

type ContactPolicyHandler struct {
	application contactPolicyApplication
}

func NewContactPolicyHandler(application contactPolicyApplication) (*ContactPolicyHandler, error) {
	if nilContactPolicyApplication(application) {
		return nil, errors.New("contact policy application is required")
	}
	return &ContactPolicyHandler{application: application}, nil
}

func nilContactPolicyApplication(application contactPolicyApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface || value.Kind() == reflect.Map || value.Kind() == reflect.Slice || value.Kind() == reflect.Func) && value.IsNil()
}

func (handler *ContactPolicyHandler) Get(writer http.ResponseWriter, request *http.Request, customerID generated.CustomerID) {
	if _, err := handler.authorize(request, authport.CapabilityOperationsRead); err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	if handler == nil || handler.application == nil || customerID <= 0 {
		writeContactPolicyError(writer, request, contactapp.ErrContactPolicyNotFound)
		return
	}
	result, err := handler.application.Get(request.Context(), contactport.CustomerID(customerID))
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	handler.write(writer, request, result)
}

func (handler *ContactPolicyHandler) Set(writer http.ResponseWriter, request *http.Request, customerID generated.CustomerID, idempotencyKey string) {
	actorID, err := handler.authorize(request, authport.CapabilityOperationsManage)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	if handler == nil || handler.application == nil || customerID <= 0 || !validContactPolicyHeaderKey(request, idempotencyKey) {
		writeContactPolicyError(writer, request, contactapp.ErrInvalidContactPolicy)
		return
	}
	command, err := decodeSetContactPolicy(writer, request)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	command.CustomerID = contactport.CustomerID(customerID)
	command.ActorID = actorID
	command.IdempotencyKey = idempotencyKey
	result, err := handler.application.Set(request.Context(), command)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	handler.write(writer, request, result)
}

func (handler *ContactPolicyHandler) Clear(writer http.ResponseWriter, request *http.Request, customerID generated.CustomerID, idempotencyKey string) {
	actorID, err := handler.authorize(request, authport.CapabilityOperationsManage)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	if handler == nil || handler.application == nil || customerID <= 0 || !validContactPolicyHeaderKey(request, idempotencyKey) {
		writeContactPolicyError(writer, request, contactapp.ErrInvalidContactPolicy)
		return
	}
	expectedVersion, err := decodeClearContactPolicy(writer, request)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	result, err := handler.application.Clear(request.Context(), contactapp.ClearContactPolicyCommand{
		CustomerID: contactport.CustomerID(customerID), ExpectedVersion: expectedVersion,
		ActorID: actorID, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	handler.write(writer, request, result)
}

func (handler *ContactPolicyHandler) authorize(request *http.Request, capability authport.Capability) (int64, error) {
	if handler == nil || request == nil {
		return 0, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !principalOK || principal.AdminUserID <= 0 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !authorizationOK || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 ||
		(principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal.AdminUserID, nil
}

func (handler *ContactPolicyHandler) write(writer http.ResponseWriter, request *http.Request, policy contactapp.ContactPolicy) {
	response, err := contactPolicyResponse(policy)
	if err != nil {
		writeContactPolicyError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(response)
}

func decodeSetContactPolicy(writer http.ResponseWriter, request *http.Request) (contactapp.SetContactPolicyCommand, error) {
	object, err := decodeCustomerMutationObject(writer, request)
	if err != nil || len(object) < 2 || len(object) > 3 {
		return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
	}
	for key := range object {
		if key != "expected_version" && key != "reason_code" && key != "suppressed_until" {
			return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
		}
	}
	expected, err := decodeNullableInteger(object["expected_version"], 64)
	if err != nil || expected == nil || *expected < 0 {
		return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
	}
	reason, err := decodeRequiredString(object["reason_code"])
	if err != nil || (reason != contactapp.ContactPolicyReasonManualOptOut && reason != contactapp.ContactPolicyReasonCompliance && reason != contactapp.ContactPolicyReasonOperatorHold) {
		return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
	}
	command := contactapp.SetContactPolicyCommand{ExpectedVersion: *expected, ReasonCode: reason}
	if raw, present := object["suppressed_until"]; present {
		value, decodeErr := decodeNullableString(raw)
		if decodeErr != nil {
			return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
		}
		if value != nil {
			parsed, parseErr := time.Parse(time.RFC3339Nano, *value)
			if parseErr != nil {
				return contactapp.SetContactPolicyCommand{}, contactapp.ErrInvalidContactPolicy
			}
			command.SuppressedUntil = &parsed
		}
	}
	return command, nil
}

func decodeClearContactPolicy(writer http.ResponseWriter, request *http.Request) (int64, error) {
	object, err := decodeCustomerMutationObject(writer, request)
	if err != nil || len(object) != 1 {
		return 0, contactapp.ErrInvalidContactPolicy
	}
	raw, present := object["expected_version"]
	if !present {
		return 0, contactapp.ErrInvalidContactPolicy
	}
	expected, err := decodeNullableInteger(raw, 64)
	if err != nil || expected == nil || *expected <= 0 {
		return 0, contactapp.ErrInvalidContactPolicy
	}
	return *expected, nil
}

func validContactPolicyHeaderKey(request *http.Request, key string) bool {
	if request == nil || len(request.Header.Values("Idempotency-Key")) != 1 {
		return false
	}
	return key != "" && strings.TrimSpace(key) == key
}

func contactPolicyResponse(policy contactapp.ContactPolicy) (generated.CustomerContactPolicyResponse, error) {
	if policy.CustomerID <= 0 || policy.Version < 0 || !policy.LocalOnly || policy.ProviderExecutionEligible || policy.RealExternalCallExecuted || policy.DeliveryProven || policy.Eligible == policy.SuppressionActive {
		return generated.CustomerContactPolicyResponse{}, contactapp.ErrContactPolicyUnavailable
	}
	result := generated.CustomerContactPolicyResponse{
		CustomerId: int64(policy.CustomerID), Version: policy.Version,
		PolicyPresent: policy.PolicyPresent, Eligible: policy.Eligible, SuppressionActive: policy.SuppressionActive,
		LocalOnly:                 generated.CustomerContactPolicyResponseLocalOnlyTrue,
		ProviderExecutionEligible: generated.CustomerContactPolicyResponseProviderExecutionEligibleFalse,
		RealExternalCallExecuted:  generated.CustomerContactPolicyResponseRealExternalCallExecutedFalse,
		DeliveryProven:            generated.CustomerContactPolicyResponseDeliveryProvenFalse,
	}
	if policy.ReasonCode != nil {
		reason := generated.CustomerContactPolicyReasonCode(*policy.ReasonCode)
		if !reason.Valid() {
			return generated.CustomerContactPolicyResponse{}, contactapp.ErrContactPolicyUnavailable
		}
		result.ReasonCode = &reason
	}
	if policy.SuppressedUntil != nil {
		until := policy.SuppressedUntil.UTC()
		result.SuppressedUntil = &until
	}
	return result, nil
}

func writeContactPolicyError(writer http.ResponseWriter, request *http.Request, err error) {
	var httpError *platformhttp.HTTPError
	if errors.As(err, &httpError) {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactapp.ErrInvalidContactPolicy):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, contactapp.ErrContactPolicyNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrContactPolicyConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
