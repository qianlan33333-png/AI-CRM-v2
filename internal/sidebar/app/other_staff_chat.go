package app

import (
	"context"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const sidebarOtherStaffChatMaximumItems = 20

// OtherStaffChatService passes the Sidebar's verified customer/owner scope to
// the local WeCom archive projection. It does not call a provider or infer a
// staff identity when the active owner lookup is unavailable.
type OtherStaffChatService struct {
	chats wecomport.CustomerOtherStaffChatReader
}

type OtherStaffChat struct {
	StaffUserID   string    `json:"staff_userid"`
	MessageType   string    `json:"message_type"`
	ContentMasked string    `json:"content_masked"`
	SentAt        time.Time `json:"sent_at"`
}

func NewOtherStaffChatService(chats wecomport.CustomerOtherStaffChatReader) (*OtherStaffChatService, error) {
	if nilOtherStaffChatDependency(chats) {
		return nil, ErrUnavailable
	}
	return &OtherStaffChatService{chats: chats}, nil
}

func (service *OtherStaffChatService) List(ctx context.Context, scope Scope) ([]OtherStaffChat, error) {
	if ctx == nil || scope.CustomerID < 1 || scope.OwnerStaffID < 1 {
		return nil, ErrInvalidInput
	}
	if service == nil || nilOtherStaffChatDependency(service.chats) {
		return nil, ErrUnavailable
	}
	page, err := service.chats.ListCustomerOtherStaffChats(ctx, wecomport.CustomerOtherStaffChatQuery{
		CustomerID: contactport.CustomerID(scope.CustomerID), OwnerStaffID: scope.OwnerStaffID,
	})
	if err != nil || len(page.Items) > sidebarOtherStaffChatMaximumItems {
		return nil, ErrUnavailable
	}
	items := make([]OtherStaffChat, len(page.Items))
	for index, item := range page.Items {
		if !validSidebarOtherStaffUserID(item.StaffUserID) || (item.MessageType != "text" && item.MessageType != "image") ||
			!validSidebarMaskedContent(item.ContentMasked) || item.SentAt.IsZero() ||
			(index > 0 && item.SentAt.After(page.Items[index-1].SentAt)) {
			return nil, ErrUnavailable
		}
		items[index] = OtherStaffChat{
			StaffUserID: item.StaffUserID, MessageType: item.MessageType, ContentMasked: item.ContentMasked, SentAt: item.SentAt.UTC(),
		}
	}
	return items, nil
}

func validSidebarOtherStaffUserID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validSidebarMaskedContent(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 10000 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func nilOtherStaffChatDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
