package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
)

type activityTimelineFake struct {
	read  contactport.Customer360Read
	err   error
	input contactport.Customer360ReadInput
	calls int
}

func (fake *activityTimelineFake) ReadCustomer360(_ context.Context, input contactport.Customer360ReadInput) (contactport.Customer360Read, error) {
	fake.calls++
	fake.input = input
	return fake.read, fake.err
}

type activityChatFake struct {
	page  customer360port.CustomerChatActivityPage
	err   error
	query customer360port.CustomerChatActivityQuery
	calls int
}

func (fake *activityChatFake) ListCustomerChatActivity(_ context.Context, query customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	fake.calls++
	fake.query = query
	return fake.page, fake.err
}

func TestActivityServiceProjectsOnlySafeFieldsAndTrustedScope(t *testing.T) {
	stamp := time.Date(2026, 8, 26, 10, 0, 0, 0, time.FixedZone("test", 8*60*60))
	timeline := &activityTimelineFake{read: contactport.Customer360Read{
		Customer: contactport.Customer360Customer{ID: 41},
		Timeline: []contactport.Customer360TimelineEntry{{ID: 7, EventType: "radar_opened", OccurredAt: stamp}},
	}}
	chat := &activityChatFake{page: customer360port.CustomerChatActivityPage{
		CustomerID: 41,
		Items:      []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text", SentAt: stamp}},
	}}
	service, err := NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{CustomerID: 41, OwnerStaffID: 7}
	nextTimeline := "timeline-opaque-next"
	timeline.read.TimelineNextCursor = &nextTimeline
	timelinePage, err := service.Timeline(context.Background(), scope, "timeline-opaque-current", 9)
	if err != nil || !reflect.DeepEqual(timelinePage, TimelineActivityPage{Items: []TimelineActivity{{ID: 7, EventType: "radar_opened", OccurredAt: stamp.UTC()}}, NextCursor: &nextTimeline}) ||
		timeline.calls != 1 || timeline.input.CustomerID != 41 || timeline.input.OwnerStaffID == nil || *timeline.input.OwnerStaffID != 7 || timeline.input.TimelineCursor != "timeline-opaque-current" || timeline.input.TimelineLimit != 9 {
		t.Fatalf("timeline=%+v err=%v calls/input=%d/%+v", timelinePage, err, timeline.calls, timeline.input)
	}
	nextChat, previousChat := "chat-opaque-next", "chat-opaque-previous"
	chat.page.ChatType, chat.page.NextCursor, chat.page.PreviousCursor = "private", &nextChat, &previousChat
	chatPage, err := service.Chat(context.Background(), scope, "private", "chat-opaque-current", 7)
	if err != nil || !reflect.DeepEqual(chatPage, ChatActivityPage{Items: []ChatActivity{{ChatType: "private", MessageType: "text", SentAt: stamp.UTC()}}, NextCursor: &nextChat, PreviousCursor: &previousChat}) ||
		chat.calls != 1 || chat.query.CustomerID != 41 || chat.query.OwnerStaffID == nil || *chat.query.OwnerStaffID != 7 || chat.query.ChatType != "private" || chat.query.Cursor != "chat-opaque-current" || chat.query.Limit != 7 {
		t.Fatalf("chat=%+v err=%v calls/query=%d/%+v", chatPage, err, chat.calls, chat.query)
	}
}

func TestActivityServiceFailsClosedBeforeReadsAndForUnsafeResults(t *testing.T) {
	timeline := &activityTimelineFake{}
	chat := &activityChatFake{}
	service, err := NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []Scope{{}, {CustomerID: 41}} {
		if _, err = service.Timeline(context.Background(), scope, "", 0); !errors.Is(err, ErrInvalidInput) || timeline.calls != 0 {
			t.Fatalf("timeline invalid scope=%+v err=%v calls=%d", scope, err, timeline.calls)
		}
		if _, err = service.Chat(context.Background(), scope, "", "", 0); !errors.Is(err, ErrInvalidInput) || chat.calls != 0 {
			t.Fatalf("chat invalid scope=%+v err=%v calls=%d", scope, err, chat.calls)
		}
	}
	timeline.read = contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, Timeline: []contactport.Customer360TimelineEntry{{ID: 1, EventType: "", OccurredAt: time.Now()}}}
	if _, err = service.Timeline(context.Background(), Scope{CustomerID: 41, OwnerStaffID: 7}, "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe timeline err=%v", err)
	}
	chat.page = customer360port.CustomerChatActivityPage{CustomerID: 41, Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text"}}}
	if _, err = service.Chat(context.Background(), Scope{CustomerID: 41, OwnerStaffID: 7}, "", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe chat err=%v", err)
	}
}

func TestActivityServiceMapsNotFoundAndUnavailable(t *testing.T) {
	timeline := &activityTimelineFake{err: contactport.ErrCustomerReadNotFound}
	chat := &activityChatFake{err: customer360port.ErrCustomerChatActivityUnavailable}
	service, err := NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{CustomerID: 41, OwnerStaffID: 7}
	if _, err = service.Timeline(context.Background(), scope, "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not found err=%v", err)
	}
	if _, err = service.Chat(context.Background(), scope, "", "", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable err=%v", err)
	}
}

func TestActivityServiceRejectsUnsafePageMetadataAndOrdering(t *testing.T) {
	stamp := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	timeline := &activityTimelineFake{read: contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, Timeline: []contactport.Customer360TimelineEntry{
		{ID: 7, EventType: "first", OccurredAt: stamp}, {ID: 7, EventType: "second", OccurredAt: stamp.Add(-time.Second)},
	}}}
	chat := &activityChatFake{page: customer360port.CustomerChatActivityPage{CustomerID: 41, ChatType: "group", Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: "text", SentAt: stamp}}}}
	service, err := NewActivityService(timeline, chat)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{CustomerID: 41, OwnerStaffID: 7}
	if _, err = service.Timeline(context.Background(), scope, "", 2); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("duplicate timeline err=%v", err)
	}
	if _, err = service.Chat(context.Background(), scope, "private", "", 2); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched chat metadata err=%v", err)
	}
	for _, input := range []struct {
		cursor string
		limit  int32
	}{{cursor: string(make([]byte, sidebarActivityMaximumCursor+1)), limit: 1}, {limit: sidebarActivityMaximumLimit + 1}} {
		if _, err = service.Timeline(context.Background(), scope, input.cursor, input.limit); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid timeline input=%+v err=%v", input, err)
		}
	}
	timeline.read = contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, Timeline: []contactport.Customer360TimelineEntry{{ID: 8, EventType: "bad\x00event", OccurredAt: stamp}}}
	if _, err = service.Timeline(context.Background(), scope, "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe timeline text err=%v", err)
	}
	emptyCursor := ""
	timeline.read = contactport.Customer360Read{Customer: contactport.Customer360Customer{ID: 41}, TimelineNextCursor: &emptyCursor}
	if _, err = service.Timeline(context.Background(), scope, "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe timeline cursor err=%v", err)
	}
	chat.page = customer360port.CustomerChatActivityPage{CustomerID: 41, ChatType: "private", Items: []customer360port.CustomerChatActivityEntry{{ChatType: "private", MessageType: strings.Repeat("a", 129), SentAt: stamp}}}
	if _, err = service.Chat(context.Background(), scope, "private", "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsafe chat text err=%v", err)
	}
}
