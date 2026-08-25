package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
)

const sidebarActivityLimit int32 = 20

const sidebarActivityMaximumLimit int32 = 100
const sidebarActivityMaximumCursor = 512
const sidebarActivityChatDefaultLimit int32 = 50

// ActivityService projects the two existing local read models into the
// sidebar's deliberately narrow activity contract. It does not read any
// provider, receipt, identity, actor, or message-body data.
type ActivityService struct {
	timeline contactport.Customer360Reader
	chats    customer360port.CustomerChatActivityReader
}

type TimelineActivity struct {
	ID         int64     `json:"id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
}

type ChatActivity struct {
	ChatType    string    `json:"chat_type"`
	MessageType string    `json:"message_type"`
	SentAt      time.Time `json:"sent_at"`
}

type TimelineActivityPage struct {
	Items      []TimelineActivity `json:"items"`
	NextCursor *string            `json:"next_cursor,omitempty"`
}

type ChatActivityPage struct {
	Items          []ChatActivity `json:"items"`
	NextCursor     *string        `json:"next_cursor,omitempty"`
	PreviousCursor *string        `json:"previous_cursor,omitempty"`
}

func NewActivityService(timeline contactport.Customer360Reader, chats customer360port.CustomerChatActivityReader) (*ActivityService, error) {
	if nilActivityDependency(timeline) || nilActivityDependency(chats) {
		return nil, ErrUnavailable
	}
	return &ActivityService{timeline: timeline, chats: chats}, nil
}

func (service *ActivityService) Timeline(ctx context.Context, scope Scope, cursor string, limit int32) (TimelineActivityPage, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.OwnerStaffID < 1 || !validActivityPageInput(cursor, limit) {
		return TimelineActivityPage{}, ErrInvalidInput
	}
	if service == nil || nilActivityDependency(service.timeline) {
		return TimelineActivityPage{}, ErrUnavailable
	}
	owner := scope.OwnerStaffID
	read, err := service.timeline.ReadCustomer360(ctx, contactport.Customer360ReadInput{
		CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: &owner, TimelineCursor: cursor, TimelineLimit: limit,
	})
	if err != nil {
		return TimelineActivityPage{}, mapActivityError(err)
	}
	if int64(read.Customer.ID) != scope.CustomerID || len(read.Timeline) > activityTimelineResultLimit(limit) {
		return TimelineActivityPage{}, ErrUnavailable
	}
	items := make([]TimelineActivity, len(read.Timeline))
	seen := make(map[int64]struct{}, len(read.Timeline))
	for index, item := range read.Timeline {
		if item.ID < 1 || !validActivityText(item.EventType) || item.OccurredAt.IsZero() {
			return TimelineActivityPage{}, ErrUnavailable
		}
		if _, duplicate := seen[item.ID]; duplicate || (index > 0 && (item.OccurredAt.After(read.Timeline[index-1].OccurredAt) ||
			item.OccurredAt.Equal(read.Timeline[index-1].OccurredAt) && item.ID >= read.Timeline[index-1].ID)) {
			return TimelineActivityPage{}, ErrUnavailable
		}
		seen[item.ID] = struct{}{}
		items[index] = TimelineActivity{ID: item.ID, EventType: item.EventType, OccurredAt: item.OccurredAt.UTC()}
	}
	if !validActivityCursor(read.TimelineNextCursor) {
		return TimelineActivityPage{}, ErrUnavailable
	}
	return TimelineActivityPage{Items: items, NextCursor: cloneActivityCursor(read.TimelineNextCursor)}, nil
}

func (service *ActivityService) Chat(ctx context.Context, scope Scope, chatType, cursor string, limit int32) (ChatActivityPage, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.OwnerStaffID < 1 || !validActivityPageInput(cursor, limit) ||
		(chatType != "" && chatType != "private" && chatType != "group") {
		return ChatActivityPage{}, ErrInvalidInput
	}
	if service == nil || nilActivityDependency(service.chats) {
		return ChatActivityPage{}, ErrUnavailable
	}
	owner := scope.OwnerStaffID
	page, err := service.chats.ListCustomerChatActivity(ctx, customer360port.CustomerChatActivityQuery{
		CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: &owner, ChatType: chatType, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		return ChatActivityPage{}, mapActivityError(err)
	}
	if int64(page.CustomerID) != scope.CustomerID || page.ChatType != chatType || len(page.Items) > activityChatResultLimit(limit) || !validActivityCursor(page.NextCursor) || !validActivityCursor(page.PreviousCursor) {
		return ChatActivityPage{}, ErrUnavailable
	}
	items := make([]ChatActivity, len(page.Items))
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || (chatType != "" && item.ChatType != chatType) || !validActivityText(item.MessageType) || item.SentAt.IsZero() ||
			(index > 0 && item.SentAt.After(page.Items[index-1].SentAt)) {
			return ChatActivityPage{}, ErrUnavailable
		}
		items[index] = ChatActivity{ChatType: item.ChatType, MessageType: item.MessageType, SentAt: item.SentAt.UTC()}
	}
	return ChatActivityPage{Items: items, NextCursor: cloneActivityCursor(page.NextCursor), PreviousCursor: cloneActivityCursor(page.PreviousCursor)}, nil
}

func validActivityPageInput(cursor string, limit int32) bool {
	return len(cursor) <= sidebarActivityMaximumCursor && limit >= 0 && limit <= sidebarActivityMaximumLimit
}

func activityTimelineResultLimit(limit int32) int {
	if limit == 0 {
		return int(sidebarActivityLimit)
	}
	return int(limit)
}

func activityChatResultLimit(limit int32) int {
	if limit == 0 {
		return int(sidebarActivityChatDefaultLimit)
	}
	return int(limit)
}

func validActivityText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validActivityCursor(cursor *string) bool {
	return cursor == nil || *cursor != "" && len(*cursor) <= sidebarActivityMaximumCursor
}

func cloneActivityCursor(cursor *string) *string {
	if cursor == nil {
		return nil
	}
	cloned := *cursor
	return &cloned
}

func mapActivityError(err error) error {
	if errors.Is(err, contactport.ErrCustomerReadNotFound) {
		return ErrNotFound
	}
	return ErrUnavailable
}

func nilActivityDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
