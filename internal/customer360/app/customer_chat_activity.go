package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const (
	CustomerChatActivityDefaultLimit = int32(50)
	CustomerChatActivityMaximumLimit = int32(100)
	customerChatActivityCursorLimit  = 512
)

type customerChatActivityCursor struct {
	Version    int    `json:"v"`
	Operation  string `json:"operation"`
	CustomerID int64  `json:"customer_id"`
	ChatType   string `json:"chat_type"`
	Offset     int32  `json:"offset"`
	Limit      int32  `json:"limit"`
}

func (service *CustomerContextService) ListCustomerChatActivity(
	ctx context.Context,
	query customer360port.CustomerChatActivityQuery,
) (customer360port.CustomerChatActivityPage, error) {
	if ctx == nil || query.CustomerID <= 0 || (query.OwnerStaffID != nil && *query.OwnerStaffID <= 0) ||
		(query.ChatType != "" && query.ChatType != "private" && query.ChatType != "group") || len(query.Cursor) > customerChatActivityCursorLimit {
		return customer360port.CustomerChatActivityPage{}, customer360port.ErrInvalidCustomerChatActivity
	}
	if service == nil || nilCustomerContextDependency(service.customers) || nilCustomerContextDependency(service.chats) {
		return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
	}
	if query.Limit == 0 {
		query.Limit = CustomerChatActivityDefaultLimit
	}
	if query.Limit < 1 || query.Limit > CustomerChatActivityMaximumLimit {
		return customer360port.CustomerChatActivityPage{}, customer360port.ErrInvalidCustomerChatActivity
	}
	offset, err := decodeCustomerChatActivityCursor(query.Cursor, query.CustomerID, query.ChatType, query.Limit)
	if err != nil {
		return customer360port.CustomerChatActivityPage{}, err
	}
	visible, err := service.customers.ReadCustomer360(ctx, contactport.Customer360ReadInput{
		CustomerID: query.CustomerID, OwnerStaffID: cloneCustomerContextInt64(query.OwnerStaffID), TimelineLimit: 1,
	})
	if err != nil {
		if errors.Is(err, contactport.ErrCustomerReadNotFound) {
			return customer360port.CustomerChatActivityPage{}, contactport.ErrCustomerReadNotFound
		}
		return customer360port.CustomerChatActivityPage{}, errors.Join(customer360port.ErrCustomerChatActivityUnavailable, err)
	}
	if visible.Customer.ID != query.CustomerID {
		return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
	}
	page, err := service.chats.ListCustomerChatSummaries(ctx, wecomport.CustomerChatSummaryQuery{
		CustomerID: query.CustomerID, ChatType: query.ChatType, Limit: query.Limit, Offset: offset,
	})
	if err != nil {
		return customer360port.CustomerChatActivityPage{}, errors.Join(customer360port.ErrCustomerChatActivityUnavailable, err)
	}
	if page.Limit != query.Limit || page.Offset != offset || len(page.Items) > int(query.Limit) || page.Total < 0 || page.Total > math.MaxInt32 ||
		(page.Total < int64(offset)+int64(len(page.Items))) || (offset < int32(page.Total) && len(page.Items) == 0) {
		return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
	}
	result := customer360port.CustomerChatActivityPage{
		CustomerID: query.CustomerID, ChatType: query.ChatType, Total: page.Total,
		Items: make([]customer360port.CustomerChatActivityEntry, len(page.Items)),
	}
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || (query.ChatType != "" && item.ChatType != query.ChatType) ||
			!validCustomerChatActivityText(item.MessageType, 128) || item.SentAt.IsZero() ||
			(index > 0 && page.Items[index-1].SentAt.Before(item.SentAt)) {
			return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
		}
		result.Items[index] = customer360port.CustomerChatActivityEntry{
			ChatType: item.ChatType, MessageType: item.MessageType, SentAt: item.SentAt.UTC(),
		}
	}
	consumed := int64(offset) + int64(len(page.Items))
	if consumed < page.Total {
		cursor, encodeErr := encodeCustomerChatActivityCursor(query.CustomerID, query.ChatType, int32(consumed), query.Limit)
		if encodeErr != nil {
			return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
		}
		result.NextCursor = &cursor
	}
	if offset > 0 {
		previous := offset - query.Limit
		if previous < 0 {
			previous = 0
		}
		cursor, encodeErr := encodeCustomerChatActivityCursor(query.CustomerID, query.ChatType, previous, query.Limit)
		if encodeErr != nil {
			return customer360port.CustomerChatActivityPage{}, customer360port.ErrCustomerChatActivityUnavailable
		}
		result.PreviousCursor = &cursor
	}
	return result, nil
}

func validCustomerChatActivityText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= maximum && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func encodeCustomerChatActivityCursor(customerID contactport.CustomerID, chatType string, offset, limit int32) (string, error) {
	if customerID <= 0 || offset < 0 || limit < 1 || limit > CustomerChatActivityMaximumLimit ||
		(chatType != "" && chatType != "private" && chatType != "group") {
		return "", customer360port.ErrInvalidCustomerChatActivity
	}
	value, err := json.Marshal(customerChatActivityCursor{1, "listCustomerChatActivity", int64(customerID), chatType, offset, limit})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func decodeCustomerChatActivityCursor(raw string, customerID contactport.CustomerID, chatType string, limit int32) (int32, error) {
	if raw == "" {
		return 0, nil
	}
	if len(raw) > customerChatActivityCursorLimit || strings.Contains(raw, "=") {
		return 0, customer360port.ErrInvalidCustomerChatActivity
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return 0, customer360port.ErrInvalidCustomerChatActivity
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor customerChatActivityCursor
	if err = decoder.Decode(&cursor); err != nil {
		return 0, customer360port.ErrInvalidCustomerChatActivity
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || cursor.Version != 1 || cursor.Operation != "listCustomerChatActivity" ||
		cursor.CustomerID != int64(customerID) || cursor.ChatType != chatType || cursor.Limit != limit || cursor.Offset < 0 {
		return 0, customer360port.ErrInvalidCustomerChatActivity
	}
	return cursor.Offset, nil
}

var _ customer360port.CustomerChatActivityReader = (*CustomerContextService)(nil)
