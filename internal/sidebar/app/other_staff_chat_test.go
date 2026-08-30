package app

import (
	"context"
	"errors"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestOtherStaffChatServiceBindsCustomerAndCurrentViewer(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	reader := &otherStaffChatReaderFake{page: wecomport.CustomerOtherStaffChatPage{Items: []wecomport.CustomerOtherStaffChat{{
		StaffUserID: "staff-other", MessageType: "text", ContentMasked: "已脱敏内容", SentAt: stamp,
	}}}}
	service, err := NewOtherStaffChatService(reader)
	if err != nil {
		t.Fatal(err)
	}
	viewerStaffID := int64(8)
	items, err := service.List(context.Background(), Scope{CustomerID: 41, OwnerStaffID: 7, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &viewerStaffID}})
	if err != nil || len(items) != 1 || items[0].StaffUserID != "staff-other" || items[0].ContentMasked != "已脱敏内容" || !items[0].SentAt.Equal(stamp) {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if reader.query != (wecomport.CustomerOtherStaffChatQuery{CustomerID: contactport.CustomerID(41), OwnerStaffID: viewerStaffID}) {
		t.Fatalf("query=%#v", reader.query)
	}
}

func TestOtherStaffChatServiceFailsClosedForReadFailureAndUnsafeProjection(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	viewerStaffID := int64(8)
	for _, reader := range []*otherStaffChatReaderFake{
		{err: errors.New("archive unavailable")},
		{page: wecomport.CustomerOtherStaffChatPage{Items: []wecomport.CustomerOtherStaffChat{{StaffUserID: "staff-other", MessageType: "voice", ContentMasked: "masked", SentAt: stamp}}}},
	} {
		service, err := NewOtherStaffChatService(reader)
		if err != nil {
			t.Fatal(err)
		}
		items, err := service.List(context.Background(), Scope{CustomerID: 41, OwnerStaffID: 7, Principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &viewerStaffID}})
		if !errors.Is(err, ErrUnavailable) || len(items) != 0 {
			t.Fatalf("items=%#v err=%v", items, err)
		}
	}
}

type otherStaffChatReaderFake struct {
	page  wecomport.CustomerOtherStaffChatPage
	err   error
	query wecomport.CustomerOtherStaffChatQuery
}

func (fake *otherStaffChatReaderFake) ListCustomerOtherStaffChats(_ context.Context, query wecomport.CustomerOtherStaffChatQuery) (wecomport.CustomerOtherStaffChatPage, error) {
	fake.query = query
	return fake.page, fake.err
}

var _ wecomport.CustomerOtherStaffChatReader = (*otherStaffChatReaderFake)(nil)
