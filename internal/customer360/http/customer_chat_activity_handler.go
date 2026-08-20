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

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360app "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/app"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerChatActivityApplication interface {
	ListCustomerChatActivity(context.Context, customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error)
}

type CustomerChatActivityHandler struct {
	application customerChatActivityApplication
}

type CustomerChatActivityQuery struct {
	ChatType       string
	Cursor         string
	CursorSupplied bool
	Limit          int32
	LimitSupplied  bool
}

type customerChatActivityEntryResponse struct {
	ChatType    string    `json:"chat_type"`
	MessageType string    `json:"message_type"`
	SentAt      time.Time `json:"sent_at"`
}

type customerChatActivityPageResponse struct {
	CustomerID               int64                               `json:"customer_id"`
	ChatType                 string                              `json:"chat_type"`
	Items                    []customerChatActivityEntryResponse `json:"items"`
	Total                    int64                               `json:"total"`
	NextCursor               *string                             `json:"next_cursor"`
	PreviousCursor           *string                             `json:"previous_cursor"`
	NonAtomicSnapshot        bool                                `json:"non_atomic_snapshot"`
	MessageContentIncluded   bool                                `json:"message_content_included"`
	IdentityValuesIncluded   bool                                `json:"identity_values_included"`
	ProviderReceiptsIncluded bool                                `json:"provider_receipts_included"`
	RealExternalCallExecuted bool                                `json:"real_external_call_executed"`
}

func NewCustomerChatActivityHandler(application customerChatActivityApplication) (*CustomerChatActivityHandler, error) {
	if nilCustomerChatActivityApplication(application) {
		return nil, customer360port.ErrCustomerChatActivityUnavailable
	}
	return &CustomerChatActivityHandler{application: application}, nil
}

func (handler *CustomerChatActivityHandler) GetCustomerChatActivity(
	writer http.ResponseWriter,
	request *http.Request,
	customerID contactport.CustomerID,
	query CustomerChatActivityQuery,
) {
	if handler == nil || nilCustomerChatActivityApplication(handler.application) || request == nil {
		if request != nil {
			writeCustomerContextError(writer, request, customer360port.ErrCustomerChatActivityUnavailable, false)
		}
		return
	}
	owner, err := customerContextOwner(request.Context())
	if err != nil {
		platformhttp.WriteError(writer, request, err)
		return
	}
	if customerID <= 0 || (query.ChatType != "" && query.ChatType != "private" && query.ChatType != "group") ||
		(query.CursorSupplied && query.Cursor == "") || len(query.Cursor) > 512 || (query.LimitSupplied && query.Limit < 1) ||
		query.Limit < 0 || query.Limit > customer360app.CustomerChatActivityMaximumLimit {
		writeCustomerChatActivityError(writer, request, customer360port.ErrInvalidCustomerChatActivity, query.CursorSupplied)
		return
	}
	page, err := handler.application.ListCustomerChatActivity(request.Context(), customer360port.CustomerChatActivityQuery{
		CustomerID: customerID, OwnerStaffID: cloneCustomerContextInt64(owner), ChatType: query.ChatType, Cursor: query.Cursor, Limit: query.Limit,
	})
	if err != nil {
		writeCustomerChatActivityError(writer, request, err, query.CursorSupplied)
		return
	}
	expectedLimit := query.Limit
	if expectedLimit == 0 {
		expectedLimit = customer360app.CustomerChatActivityDefaultLimit
	}
	response, err := customerChatActivityResponse(customerID, query.ChatType, expectedLimit, page)
	if err != nil {
		writeCustomerChatActivityError(writer, request, err, false)
		return
	}
	writeCustomerContextJSON(writer, http.StatusOK, response)
}

func customerChatActivityResponse(expected contactport.CustomerID, expectedChatType string, expectedLimit int32, page customer360port.CustomerChatActivityPage) (customerChatActivityPageResponse, error) {
	if expected <= 0 || page.CustomerID != expected || page.ChatType != expectedChatType || page.Total < int64(len(page.Items)) ||
		len(page.Items) > int(customer360app.CustomerChatActivityMaximumLimit) || invalidCustomerChatActivityCursor(page.NextCursor) ||
		invalidCustomerChatActivityCursor(page.PreviousCursor) || (page.NextCursor != nil && len(page.Items) != int(expectedLimit)) ||
		(page.PreviousCursor == nil && page.Total > int64(len(page.Items)) && page.NextCursor == nil) {
		return customerChatActivityPageResponse{}, customer360port.ErrCustomerChatActivityUnavailable
	}
	filter := page.ChatType
	if filter == "" {
		filter = "all"
	}
	response := customerChatActivityPageResponse{
		CustomerID: int64(expected), ChatType: filter, Items: make([]customerChatActivityEntryResponse, 0, len(page.Items)), Total: page.Total,
		NextCursor: cloneCustomerContextString(page.NextCursor), PreviousCursor: cloneCustomerContextString(page.PreviousCursor), NonAtomicSnapshot: true,
	}
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || (expectedChatType != "" && item.ChatType != expectedChatType) ||
			!safeCustomerChatActivityText(item.MessageType) || item.SentAt.IsZero() || (index > 0 && page.Items[index-1].SentAt.Before(item.SentAt)) {
			return customerChatActivityPageResponse{}, customer360port.ErrCustomerChatActivityUnavailable
		}
		response.Items = append(response.Items, customerChatActivityEntryResponse{ChatType: item.ChatType, MessageType: item.MessageType, SentAt: item.SentAt.UTC()})
	}
	return response, nil
}

func invalidCustomerChatActivityCursor(value *string) bool {
	return value != nil && (*value == "" || len(*value) > 512 || strings.TrimSpace(*value) != *value)
}

func safeCustomerChatActivityText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= 128 && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func writeCustomerChatActivityError(writer http.ResponseWriter, request *http.Request, err error, cursor bool) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, contactport.ErrCustomerReadNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, customer360port.ErrInvalidCustomerChatActivity):
		code = platformhttp.CodeMalformedRequest
		if cursor {
			code = platformhttp.CodeCursorInvalid
		}
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilCustomerChatActivityApplication(application customerChatActivityApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
