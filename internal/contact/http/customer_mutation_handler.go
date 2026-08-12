package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maxCustomerMutationBodyBytes = 1 << 20

var (
	errCustomerMutationBodyMalformed = errors.New("customer mutation body is malformed")
	errCustomerMutationBodyInvalid   = errors.New("customer mutation body is invalid")
)

type customerMutationApplication interface {
	Update(context.Context, contactapp.CustomerUpdateCommand) (contactapp.CustomerRecord, error)
	SetStage(context.Context, contactapp.CustomerStageCommand) (contactapp.CustomerRecord, error)
	AddTag(context.Context, contactapp.CustomerTagCommand) error
	RemoveTag(context.Context, contactapp.CustomerTagCommand) error
}

// CustomerMutationHandler maps the frozen generated HTTP operations to the
// transaction-bound contact mutation application service.
type CustomerMutationHandler struct {
	application customerMutationApplication
}

func NewCustomerMutationHandler(application customerMutationApplication) (*CustomerMutationHandler, error) {
	if nilCustomerMutationApplication(application) {
		return nil, errors.New("customer mutation application is required")
	}
	return &CustomerMutationHandler{application: application}, nil
}

func (handler *CustomerMutationHandler) UpdateCustomer(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	_ generated.UpdateCustomerParams,
) {
	principal, scopeOwnerStaffID, err := handler.operation(request)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	if customerID <= 0 {
		writeCustomerMutationError(writer, request, contactapp.ErrCustomerNotFound)
		return
	}
	command, err := decodeCustomerUpdate(writer, request)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	command.ID = contactport.CustomerID(customerID)
	command.ScopeOwnerStaffID = scopeOwnerStaffID
	command.Actor = actor(principal)

	customer, err := handler.application.Update(request.Context(), command)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	response, err := customerResponse(customer)
	if err != nil {
		writeCustomerMutationError(writer, request, contactapp.ErrCustomerMutationFailed)
		return
	}
	writeCustomerListJSON(writer, http.StatusOK, response)
}

func (handler *CustomerMutationHandler) SetCustomerStage(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	_ generated.SetCustomerStageParams,
) {
	principal, scopeOwnerStaffID, err := handler.operation(request)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	if customerID <= 0 {
		writeCustomerMutationError(writer, request, contactapp.ErrCustomerNotFound)
		return
	}
	stageID, err := decodeCustomerStage(writer, request)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	customer, err := handler.application.SetStage(request.Context(), contactapp.CustomerStageCommand{
		ID: contactport.CustomerID(customerID), ScopeOwnerStaffID: scopeOwnerStaffID,
		StageID: stageID, Actor: actor(principal),
	})
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	response, err := customerResponse(customer)
	if err != nil {
		writeCustomerMutationError(writer, request, contactapp.ErrCustomerMutationFailed)
		return
	}
	writeCustomerListJSON(writer, http.StatusOK, response)
}

func (handler *CustomerMutationHandler) AddCustomerTag(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	tagID generated.TagID,
	_ generated.AddCustomerTagParams,
) {
	handler.mutateTag(writer, request, customerID, tagID, true)
}

func (handler *CustomerMutationHandler) RemoveCustomerTag(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	tagID generated.TagID,
	_ generated.RemoveCustomerTagParams,
) {
	handler.mutateTag(writer, request, customerID, tagID, false)
}

func (handler *CustomerMutationHandler) mutateTag(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	tagID generated.TagID,
	add bool,
) {
	principal, scopeOwnerStaffID, err := handler.operation(request)
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	if customerID <= 0 || tagID <= 0 {
		writeCustomerMutationError(writer, request, contactapp.ErrCustomerNotFound)
		return
	}
	command := contactapp.CustomerTagCommand{
		ID: contactport.CustomerID(customerID), ScopeOwnerStaffID: scopeOwnerStaffID,
		TagID: int64(tagID), Actor: actor(principal),
	}
	if add {
		err = handler.application.AddTag(request.Context(), command)
	} else {
		err = handler.application.RemoveTag(request.Context(), command)
	}
	if err != nil {
		writeCustomerMutationError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *CustomerMutationHandler) operation(
	request *http.Request,
) (authport.Principal, *int64, error) {
	if handler == nil || nilCustomerMutationApplication(handler.application) || request == nil {
		return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityCustomersWrite {
		return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}

	switch authorization.Scope {
	case authport.ScopeGlobal:
		if authorization.OwnerStaffID != 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
			return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		return principal, nil, nil
	case authport.ScopeOwnerStaff:
		if principal.Role != authport.RoleSales || principal.StaffID == nil ||
			*principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		ownerStaffID := authorization.OwnerStaffID
		return principal, &ownerStaffID, nil
	default:
		return authport.Principal{}, nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func decodeCustomerUpdate(writer http.ResponseWriter, request *http.Request) (contactapp.CustomerUpdateCommand, error) {
	object, err := decodeCustomerMutationObject(writer, request)
	if err != nil {
		return contactapp.CustomerUpdateCommand{}, err
	}
	allowed := map[string]bool{
		"name": true, "avatar_url": true, "gender": true, "owner_staff_id": true,
		"channel_id": true, "extra": true,
	}
	for key := range object {
		if !allowed[key] {
			return contactapp.CustomerUpdateCommand{}, errCustomerMutationBodyMalformed
		}
	}
	if len(object) == 0 {
		return contactapp.CustomerUpdateCommand{}, errCustomerMutationBodyInvalid
	}

	var command contactapp.CustomerUpdateCommand
	if raw, ok := object["name"]; ok {
		value, decodeErr := decodeRequiredString(raw)
		if decodeErr != nil {
			return command, decodeErr
		}
		command.Name = &value
	}
	if raw, ok := object["avatar_url"]; ok {
		value, decodeErr := decodeNullableString(raw)
		if decodeErr != nil {
			return command, decodeErr
		}
		command.AvatarURL = contactapp.NullablePatch[string]{Set: true, Value: value}
	}
	if raw, ok := object["gender"]; ok {
		value, decodeErr := decodeNullableInteger(raw, 16)
		if decodeErr != nil {
			return command, decodeErr
		}
		command.Gender.Set = true
		if value != nil {
			converted := int16(*value)
			command.Gender.Value = &converted
		}
	}
	if raw, ok := object["owner_staff_id"]; ok {
		value, decodeErr := decodeNullableInteger(raw, 64)
		if decodeErr != nil {
			return command, decodeErr
		}
		command.OwnerStaffID = contactapp.NullablePatch[int64]{Set: true, Value: value}
	}
	if raw, ok := object["channel_id"]; ok {
		value, decodeErr := decodeNullableInteger(raw, 64)
		if decodeErr != nil {
			return command, decodeErr
		}
		command.ChannelID = contactapp.NullablePatch[int64]{Set: true, Value: value}
	}
	if raw, ok := object["extra"]; ok {
		if err = validateStrictJSONObject(raw); err != nil {
			return command, err
		}
		copyOfRaw := append(json.RawMessage(nil), raw...)
		command.Extra = &copyOfRaw
	}
	return command, nil
}

func decodeCustomerStage(writer http.ResponseWriter, request *http.Request) (*int64, error) {
	object, err := decodeCustomerMutationObject(writer, request)
	if err != nil {
		return nil, err
	}
	if len(object) != 1 {
		return nil, errCustomerMutationBodyMalformed
	}
	raw, ok := object["stage_id"]
	if !ok {
		return nil, errCustomerMutationBodyMalformed
	}
	return decodeNullableInteger(raw, 64)
}

func decodeCustomerMutationObject(
	writer http.ResponseWriter,
	request *http.Request,
) (map[string]json.RawMessage, error) {
	if request == nil || request.Body == nil {
		return nil, errCustomerMutationBodyMalformed
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxCustomerMutationBodyBytes)
	decoder := json.NewDecoder(request.Body)
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errCustomerMutationBodyMalformed
	}
	object := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, tokenErr := decoder.Token()
		key, ok := keyToken.(string)
		if tokenErr != nil || !ok {
			return nil, errCustomerMutationBodyMalformed
		}
		if _, duplicate := object[key]; duplicate {
			return nil, errCustomerMutationBodyMalformed
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, errCustomerMutationBodyMalformed
		}
		object[key] = append(json.RawMessage(nil), value...)
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errCustomerMutationBodyMalformed
	}
	var trailing json.RawMessage
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errCustomerMutationBodyMalformed
	}
	return object, nil
}

func decodeRequiredString(raw json.RawMessage) (string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", errCustomerMutationBodyMalformed
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errCustomerMutationBodyMalformed
	}
	return value, nil
}

func decodeNullableString(raw json.RawMessage) (*string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	value, err := decodeRequiredString(raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func decodeNullableInteger(raw json.RawMessage, bits int) (*int64, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errCustomerMutationBodyMalformed
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, errCustomerMutationBodyMalformed
	}
	parsed, err := strconv.ParseInt(number.String(), 10, bits)
	if err != nil {
		return nil, errCustomerMutationBodyInvalid
	}
	return &parsed, nil
}

func validateStrictJSONObject(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errCustomerMutationBodyMalformed
	}
	if err = consumeJSONObject(decoder); err != nil {
		return errCustomerMutationBodyMalformed
	}
	var trailing json.RawMessage
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errCustomerMutationBodyMalformed
	}
	return nil
}

func consumeJSONObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return errCustomerMutationBodyMalformed
		}
		if _, duplicate := seen[key]; duplicate {
			return errCustomerMutationBodyMalformed
		}
		seen[key] = struct{}{}
		if err = consumeJSONValue(decoder); err != nil {
			return err
		}
	}
	token, err := decoder.Token()
	if err != nil || token != json.Delim('}') {
		return errCustomerMutationBodyMalformed
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return errCustomerMutationBodyMalformed
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeJSONObject(decoder)
	case '[':
		for decoder.More() {
			if err = consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		token, err = decoder.Token()
		if err != nil || token != json.Delim(']') {
			return errCustomerMutationBodyMalformed
		}
		return nil
	default:
		return errCustomerMutationBodyMalformed
	}
}

func writeCustomerMutationError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, errCustomerMutationBodyMalformed):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, errCustomerMutationBodyInvalid), errors.Is(err, contactapp.ErrInvalidCustomerMutation):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, contactapp.ErrCustomerNotFound), errors.Is(err, contactapp.ErrCustomerTagNotFound),
		errors.Is(err, contactport.ErrStageNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, contactapp.ErrCustomerConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilCustomerMutationApplication(application customerMutationApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
