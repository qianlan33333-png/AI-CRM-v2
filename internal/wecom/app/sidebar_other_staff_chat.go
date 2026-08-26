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
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

const sidebarOtherStaffChatLimit int32 = 20

type OtherStaffChatRecord struct {
	StaffUserID, MessageType, ContentMasked string
	SentAt                                  time.Time
}

// OtherStaffChatStore reads the existing local archive projection only. Its
// rows intentionally contain no source message ID, participant, receipt, or
// provider fields.
type OtherStaffChatStore interface {
	ListOtherStaffChatRecords(context.Context, contactport.CustomerID, string, int32) ([]OtherStaffChatRecord, error)
}

// OtherStaffChatService binds an archive read to the Sidebar customer's owner.
// The active owner lookup and archive query share one UnitOfWork so a concurrent
// deactivation cannot make "other staff" classification ambiguous.
type OtherStaffChatService struct {
	uow    platformport.UnitOfWork
	store  OtherStaffChatStore
	owners contactport.ActiveStaffSenderReader
}

var _ wecomport.CustomerOtherStaffChatReader = (*OtherStaffChatService)(nil)

func NewOtherStaffChatService(uow platformport.UnitOfWork, store OtherStaffChatStore, owners contactport.ActiveStaffSenderReader) *OtherStaffChatService {
	return &OtherStaffChatService{uow: uow, store: store, owners: owners}
}

func (service *OtherStaffChatService) ListCustomerOtherStaffChats(ctx context.Context, query wecomport.CustomerOtherStaffChatQuery) (wecomport.CustomerOtherStaffChatPage, error) {
	if ctx == nil || query.CustomerID <= 0 || query.OwnerStaffID < 1 || !otherStaffChatReady(service) {
		return wecomport.CustomerOtherStaffChatPage{}, wecomport.ErrCustomerOtherStaffChatUnavailable
	}
	var ownerUserID string
	var records []OtherStaffChatRecord
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		ownerUserID, err = service.owners.LockActiveWeComUserID(tx, query.OwnerStaffID)
		if err != nil || !validOtherStaffUserID(ownerUserID) {
			return errors.Join(wecomport.ErrCustomerOtherStaffChatUnavailable, err)
		}
		records, err = service.store.ListOtherStaffChatRecords(tx, query.CustomerID, ownerUserID, sidebarOtherStaffChatLimit)
		return err
	})
	if err != nil || !validOtherStaffChatRecords(records, ownerUserID) {
		return wecomport.CustomerOtherStaffChatPage{}, errors.Join(wecomport.ErrCustomerOtherStaffChatUnavailable, err)
	}
	items := make([]wecomport.CustomerOtherStaffChat, len(records))
	for index, record := range records {
		items[index] = wecomport.CustomerOtherStaffChat{
			StaffUserID: record.StaffUserID, MessageType: record.MessageType, ContentMasked: record.ContentMasked, SentAt: record.SentAt.UTC(),
		}
	}
	return wecomport.CustomerOtherStaffChatPage{Items: items}, nil
}

func otherStaffChatReady(service *OtherStaffChatService) bool {
	return service != nil && service.uow != nil && !nilOtherStaffDependency(service.store) && !nilOtherStaffDependency(service.owners)
}

func validOtherStaffChatRecords(records []OtherStaffChatRecord, ownerUserID string) bool {
	if len(records) > int(sidebarOtherStaffChatLimit) || !validOtherStaffUserID(ownerUserID) {
		return false
	}
	for index, record := range records {
		if !validOtherStaffUserID(record.StaffUserID) || record.StaffUserID == ownerUserID ||
			(record.MessageType != "text" && record.MessageType != "image") || !validMaskedArchiveContent(record.ContentMasked) || record.SentAt.IsZero() ||
			(index > 0 && record.SentAt.After(records[index-1].SentAt)) {
			return false
		}
	}
	return true
}

func validOtherStaffUserID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validMaskedArchiveContent(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 10000 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func nilOtherStaffDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}
