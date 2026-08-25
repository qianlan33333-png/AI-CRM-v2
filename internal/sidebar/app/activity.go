package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
)

const sidebarActivityLimit int32 = 20

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

func NewActivityService(timeline contactport.Customer360Reader, chats customer360port.CustomerChatActivityReader) (*ActivityService, error) {
	if nilActivityDependency(timeline) || nilActivityDependency(chats) {
		return nil, ErrUnavailable
	}
	return &ActivityService{timeline: timeline, chats: chats}, nil
}

func (service *ActivityService) Timeline(ctx context.Context, scope Scope) ([]TimelineActivity, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.OwnerStaffID < 1 || service == nil || nilActivityDependency(service.timeline) {
		return nil, ErrUnavailable
	}
	owner := scope.OwnerStaffID
	read, err := service.timeline.ReadCustomer360(ctx, contactport.Customer360ReadInput{
		CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: &owner, TimelineLimit: sidebarActivityLimit,
	})
	if err != nil {
		return nil, mapActivityError(err)
	}
	if int64(read.Customer.ID) != scope.CustomerID {
		return nil, ErrUnavailable
	}
	items := make([]TimelineActivity, len(read.Timeline))
	for index, item := range read.Timeline {
		if item.ID < 1 || item.EventType == "" || item.OccurredAt.IsZero() {
			return nil, ErrUnavailable
		}
		items[index] = TimelineActivity{ID: item.ID, EventType: item.EventType, OccurredAt: item.OccurredAt.UTC()}
	}
	return items, nil
}

func (service *ActivityService) Chat(ctx context.Context, scope Scope) ([]ChatActivity, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.OwnerStaffID < 1 || service == nil || nilActivityDependency(service.chats) {
		return nil, ErrUnavailable
	}
	owner := scope.OwnerStaffID
	page, err := service.chats.ListCustomerChatActivity(ctx, customer360port.CustomerChatActivityQuery{
		CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: &owner, Limit: sidebarActivityLimit,
	})
	if err != nil {
		return nil, mapActivityError(err)
	}
	if int64(page.CustomerID) != scope.CustomerID || len(page.Items) > int(sidebarActivityLimit) {
		return nil, ErrUnavailable
	}
	items := make([]ChatActivity, len(page.Items))
	for index, item := range page.Items {
		if (item.ChatType != "private" && item.ChatType != "group") || item.MessageType == "" || item.SentAt.IsZero() {
			return nil, ErrUnavailable
		}
		items[index] = ChatActivity{ChatType: item.ChatType, MessageType: item.MessageType, SentAt: item.SentAt.UTC()}
	}
	return items, nil
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
