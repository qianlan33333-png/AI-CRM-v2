// Package http exposes the safe local Customer 360 read boundary.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"
	"unicode/utf8"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const customerContextMaximumLimit int32 = 200

type customerContextApplication interface {
	ReadCustomerContext(context.Context, customer360port.CustomerContextQuery) (customer360port.CustomerContext, error)
}

type CustomerContextHandler struct {
	application customerContextApplication
}

func NewCustomerContextHandler(application customerContextApplication) (*CustomerContextHandler, error) {
	if nilCustomerContextApplication(application) {
		return nil, errors.New("customer context application is required")
	}
	return &CustomerContextHandler{application: application}, nil
}

func (handler *CustomerContextHandler) GetCustomerContext(
	writer http.ResponseWriter,
	request *http.Request,
	customerID generated.CustomerID,
	params generated.GetCustomerContextParams,
) {
	if handler == nil || nilCustomerContextApplication(handler.application) || request == nil {
		if request == nil {
			request = &http.Request{}
		}
		writeCustomerContextError(writer, request, customer360port.ErrCustomerContextUnavailable, false)
		return
	}
	if customerID <= 0 {
		writeCustomerContextError(writer, request, contactport.ErrCustomerReadNotFound, false)
		return
	}
	ownerStaffID, err := customerContextOwner(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	input, cursorSupplied, err := customerContextInput(customerID, params, ownerStaffID)
	if err != nil {
		writeCustomerContextError(writer, request, err, cursorSupplied)
		return
	}
	result, err := handler.application.ReadCustomerContext(request.Context(), input)
	if err != nil {
		writeCustomerContextError(writer, request, err, cursorSupplied)
		return
	}
	response, err := customerContextResponse(customerID, result)
	if err != nil {
		writeCustomerContextError(writer, request, customer360port.ErrCustomerContextUnavailable, cursorSupplied)
		return
	}
	writeCustomerContextJSON(writer, http.StatusOK, response)
}

func customerContextOwner(ctx context.Context) (*int64, error) {
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
		if principal.Role != authport.RoleSales || principal.StaffID == nil || *principal.StaffID != authorization.OwnerStaffID || authorization.OwnerStaffID <= 0 {
			return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
		}
		owner := authorization.OwnerStaffID
		return &owner, nil
	default:
		return nil, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
}

func customerContextInput(
	customerID generated.CustomerID,
	params generated.GetCustomerContextParams,
	ownerStaffID *int64,
) (customer360port.CustomerContextQuery, bool, error) {
	input := customer360port.CustomerContextQuery{CustomerID: contactport.CustomerID(customerID), OwnerStaffID: cloneCustomerContextInt64(ownerStaffID)}
	if params.Cursor != nil {
		if *params.Cursor == "" || len(*params.Cursor) > 512 {
			return customer360port.CustomerContextQuery{}, true, customer360port.ErrInvalidCustomerContext
		}
		input.TimelineCursor = string(*params.Cursor)
	}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > int(customerContextMaximumLimit) {
			return customer360port.CustomerContextQuery{}, params.Cursor != nil, customer360port.ErrInvalidCustomerContext
		}
		input.TimelineLimit = int32(*params.Limit)
	}
	return input, params.Cursor != nil, nil
}

func customerContextResponse(
	expectedCustomerID generated.CustomerID,
	result customer360port.CustomerContext,
) (generated.CustomerContextResponse, error) {
	if expectedCustomerID <= 0 || result.Customer.ID != contactport.CustomerID(expectedCustomerID) || !utf8.ValidString(result.Customer.Name) {
		return generated.CustomerContextResponse{}, errors.New("invalid customer context customer")
	}
	if invalidCustomerContextID(result.Customer.StageID) || invalidCustomerContextID(result.Customer.OwnerStaffID) ||
		invalidCustomerContextID(result.Customer.ChannelID) || invalidCustomerContextTime(result.Customer.AddedAt) || invalidCustomerContextTime(result.Customer.LastInteractAt) ||
		(result.TimelineNextCursor != nil && (*result.TimelineNextCursor == "" || len(*result.TimelineNextCursor) > 512)) {
		return generated.CustomerContextResponse{}, errors.New("invalid customer context optional value")
	}
	response := generated.CustomerContextResponse{
		Customer: generated.CustomerContextCustomer{
			Id: int64(result.Customer.ID), Name: result.Customer.Name,
			StageId: cloneCustomerContextInt64(result.Customer.StageID), OwnerStaffId: cloneCustomerContextInt64(result.Customer.OwnerStaffID),
			ChannelId: cloneCustomerContextInt64(result.Customer.ChannelID), AddedAt: cloneCustomerContextTime(result.Customer.AddedAt),
			LastInteractAt: cloneCustomerContextTime(result.Customer.LastInteractAt),
		},
		Tags:                     make([]generated.CustomerContextTag, 0, len(result.Tags)),
		Timeline:                 make([]generated.CustomerContextTimelineEntry, 0, len(result.Timeline)),
		TimelineNextCursor:       cloneCustomerContextString(result.TimelineNextCursor),
		NonAtomicSnapshot:        generated.CustomerContextResponseNonAtomicSnapshotTrue,
		RealExternalCallExecuted: generated.CustomerContextResponseRealExternalCallExecutedFalse,
		Chat: generated.CustomerContextChatSummary{
			Items: make([]generated.CustomerContextChatEntry, 0, len(result.Chat.Items)), LocalArchiveAvailable: result.Chat.LocalArchiveAvailable, Total: result.Chat.Total,
		},
	}
	seenTags := make(map[int64]struct{}, len(result.Tags))
	for _, tag := range result.Tags {
		if tag.ID <= 0 || tag.Name == "" || utf8.RuneCountInString(tag.Name) > 200 || !utf8.ValidString(tag.Name) ||
			invalidCustomerContextID(tag.GroupID) || (tag.GroupID == nil) != (tag.GroupName == nil) ||
			(tag.GroupName != nil && !utf8.ValidString(*tag.GroupName)) {
			return generated.CustomerContextResponse{}, errors.New("invalid customer context tag")
		}
		if _, exists := seenTags[tag.ID]; exists {
			return generated.CustomerContextResponse{}, errors.New("duplicate customer context tag")
		}
		seenTags[tag.ID] = struct{}{}
		response.Tags = append(response.Tags, generated.CustomerContextTag{
			Id: tag.ID, GroupId: cloneCustomerContextInt64(tag.GroupID), GroupName: cloneCustomerContextString(tag.GroupName),
			GroupSortOrder: tag.GroupSortOrder, Name: tag.Name, SortOrder: tag.SortOrder,
		})
	}
	seenTimeline := make(map[int64]struct{}, len(result.Timeline))
	for _, event := range result.Timeline {
		if event.ID <= 0 || event.EventType == "" || utf8.RuneCountInString(event.EventType) > 200 || !utf8.ValidString(event.EventType) || event.OccurredAt.IsZero() {
			return generated.CustomerContextResponse{}, errors.New("invalid customer context timeline")
		}
		if _, exists := seenTimeline[event.ID]; exists {
			return generated.CustomerContextResponse{}, errors.New("duplicate customer context timeline")
		}
		seenTimeline[event.ID] = struct{}{}
		response.Timeline = append(response.Timeline, generated.CustomerContextTimelineEntry{Id: event.ID, EventType: event.EventType, OccurredAt: event.OccurredAt.UTC()})
	}
	if result.Chat.Total < int64(len(result.Chat.Items)) || len(result.Chat.Items) > 20 || (!result.Chat.LocalArchiveAvailable && (result.Chat.Total != 0 || len(result.Chat.Items) != 0)) {
		return generated.CustomerContextResponse{}, errors.New("invalid customer context chat summary")
	}
	for _, entry := range result.Chat.Items {
		if (entry.ChatType != "private" && entry.ChatType != "group") || entry.MessageType == "" || utf8.RuneCountInString(entry.MessageType) > 200 || !utf8.ValidString(entry.MessageType) || entry.SentAt.IsZero() {
			return generated.CustomerContextResponse{}, errors.New("invalid customer context chat entry")
		}
		response.Chat.Items = append(response.Chat.Items, generated.CustomerContextChatEntry{
			ChatType: generated.CustomerContextChatEntryChatType(entry.ChatType), MessageType: entry.MessageType, SentAt: entry.SentAt.UTC(),
		})
	}
	return response, nil
}

func writeCustomerContextError(writer http.ResponseWriter, request *http.Request, err error, cursorSupplied bool) {
	if request == nil {
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactport.ErrCustomerReadNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, customer360port.ErrInvalidCustomerContext):
		code = platformhttp.CodeMalformedRequest
		if cursorSupplied {
			code = platformhttp.CodeCursorInvalid
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeCustomerContextJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func invalidCustomerContextID(value *int64) bool       { return value != nil && *value <= 0 }
func invalidCustomerContextTime(value *time.Time) bool { return value != nil && value.IsZero() }

func cloneCustomerContextInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerContextString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerContextTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func nilCustomerContextApplication(application customerContextApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
