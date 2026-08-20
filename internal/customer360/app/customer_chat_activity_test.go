package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestCustomerChatActivityPagesSafeMetadataWithOwnerScopeAndBoundCursor(t *testing.T) {
	owner := int64(71)
	local := &customerContextLocalFake{result: customerContextTestLocal(41)}
	chats := &customerContextChatFake{fn: func(query wecomport.CustomerChatSummaryQuery) (wecomport.CustomerChatSummaryPage, error) {
		items := []wecomport.CustomerChatSummary{
			{ChatType: "private", MessageType: "text", SentAt: customerContextTestTime(4)},
			{ChatType: "private", MessageType: "image", SentAt: customerContextTestTime(3)},
			{ChatType: "private", MessageType: "file", SentAt: customerContextTestTime(2)},
		}
		start := int(query.Offset)
		end := start + int(query.Limit)
		if end > len(items) {
			end = len(items)
		}
		return wecomport.CustomerChatSummaryPage{Items: items[start:end], Total: int64(len(items)), Limit: query.Limit, Offset: query.Offset}, nil
	}}
	service := NewCustomerContextService(local, chats)
	first, err := service.ListCustomerChatActivity(context.Background(), customer360port.CustomerChatActivityQuery{
		CustomerID: 41, OwnerStaffID: &owner, ChatType: "private", Limit: 2,
	})
	if err != nil || first.CustomerID != 41 || first.ChatType != "private" || first.Total != 3 || len(first.Items) != 2 ||
		first.NextCursor == nil || first.PreviousCursor != nil {
		t.Fatalf("first page=%#v err=%v", first, err)
	}
	second, err := service.ListCustomerChatActivity(context.Background(), customer360port.CustomerChatActivityQuery{
		CustomerID: 41, OwnerStaffID: &owner, ChatType: "private", Limit: 2, Cursor: *first.NextCursor,
	})
	if err != nil || len(second.Items) != 1 || second.Items[0].MessageType != "file" || second.NextCursor != nil || second.PreviousCursor == nil {
		t.Fatalf("second page=%#v err=%v", second, err)
	}
	if local.calls != 2 || len(chats.inputs) != 2 || chats.inputs[1] != (wecomport.CustomerChatSummaryQuery{CustomerID: 41, ChatType: "private", Limit: 2, Offset: 2}) ||
		local.inputs[0].OwnerStaffID == nil || *local.inputs[0].OwnerStaffID != owner || local.inputs[0].TimelineLimit != 1 {
		t.Fatalf("inputs local=%#v chats=%#v", local.inputs, chats.inputs)
	}
	for _, invalid := range []customer360port.CustomerChatActivityQuery{
		{CustomerID: 42, ChatType: "private", Limit: 2, Cursor: *first.NextCursor},
		{CustomerID: 41, ChatType: "group", Limit: 2, Cursor: *first.NextCursor},
		{CustomerID: 41, ChatType: "private", Limit: 3, Cursor: *first.NextCursor},
	} {
		if _, invalidErr := service.ListCustomerChatActivity(context.Background(), invalid); !errors.Is(invalidErr, customer360port.ErrInvalidCustomerChatActivity) {
			t.Fatalf("invalid=%#v err=%v", invalid, invalidErr)
		}
	}
}

func TestCustomerChatActivityFailsClosedBeforeUnsafeArchiveProjection(t *testing.T) {
	tests := []customer360port.CustomerChatActivityQuery{
		{}, {CustomerID: 1, ChatType: "room"}, {CustomerID: 1, Limit: -1},
		{CustomerID: 1, Limit: CustomerChatActivityMaximumLimit + 1}, {CustomerID: 1, Cursor: strings.Repeat("a", 513)},
	}
	for _, query := range tests {
		local, chats := &customerContextLocalFake{}, &customerContextChatFake{}
		page, err := NewCustomerContextService(local, chats).ListCustomerChatActivity(context.Background(), query)
		if !errors.Is(err, customer360port.ErrInvalidCustomerChatActivity) || !reflect.DeepEqual(page, customer360port.CustomerChatActivityPage{}) || local.calls != 0 || chats.calls != 0 {
			t.Fatalf("query=%#v page=%#v err=%v calls=%d/%d", query, page, err, local.calls, chats.calls)
		}
	}

	local := &customerContextLocalFake{result: customerContextTestLocal(41)}
	unsafe := &customerContextChatFake{result: wecomport.CustomerChatSummaryPage{
		Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: " text ", SentAt: customerContextTestTime(1)}},
		Total: 1, Limit: CustomerChatActivityDefaultLimit,
	}}
	page, err := NewCustomerContextService(local, unsafe).ListCustomerChatActivity(context.Background(), customer360port.CustomerChatActivityQuery{CustomerID: 41})
	if !errors.Is(err, customer360port.ErrCustomerChatActivityUnavailable) || !reflect.DeepEqual(page, customer360port.CustomerChatActivityPage{}) {
		t.Fatalf("unsafe page=%#v err=%v", page, err)
	}
}

func TestCustomerChatActivityPreservesVisibilityNotFoundAndDoesNotReadArchive(t *testing.T) {
	local := &customerContextLocalFake{err: contactport.ErrCustomerReadNotFound}
	chats := &customerContextChatFake{}
	page, err := NewCustomerContextService(local, chats).ListCustomerChatActivity(context.Background(), customer360port.CustomerChatActivityQuery{CustomerID: 41})
	if !errors.Is(err, contactport.ErrCustomerReadNotFound) || !reflect.DeepEqual(page, customer360port.CustomerChatActivityPage{}) || chats.calls != 0 {
		t.Fatalf("page=%#v err=%v chats=%d", page, err, chats.calls)
	}
}
