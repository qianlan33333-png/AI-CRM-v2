package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestOtherStaffChatServiceReturnsOnlyOtherOwnersFromLocalArchive(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	store := &otherStaffChatTestStore{records: []OtherStaffChatRecord{
		{StaffUserID: "staff-other", MessageType: "text", ContentMasked: "已脱敏文本", SentAt: stamp},
		{StaffUserID: "staff-image", MessageType: "image", ContentMasked: "[image]", SentAt: stamp.Add(-time.Minute)},
	}}
	owners := &otherStaffChatOwnerFake{userID: "staff-current"}
	service := NewOtherStaffChatService(messageArchiveTestUoW{}, store, owners)

	page, err := service.ListCustomerOtherStaffChats(context.Background(), wecomport.CustomerOtherStaffChatQuery{CustomerID: 41, OwnerStaffID: 7})
	if err != nil {
		t.Fatalf("ListCustomerOtherStaffChats() error = %v", err)
	}
	if owners.calls != 1 || store.calls != 1 || store.customerID != 41 || store.ownerUserID != "staff-current" || store.limit != sidebarOtherStaffChatLimit {
		t.Fatalf("owner/store calls=%d/%d query=%d/%q/%d", owners.calls, store.calls, store.customerID, store.ownerUserID, store.limit)
	}
	want := []wecomport.CustomerOtherStaffChat{
		{StaffUserID: "staff-other", MessageType: "text", ContentMasked: "已脱敏文本", SentAt: stamp},
		{StaffUserID: "staff-image", MessageType: "image", ContentMasked: "[image]", SentAt: stamp.Add(-time.Minute)},
	}
	if !reflect.DeepEqual(page.Items, want) {
		t.Fatalf("safe page = %#v, want %#v", page.Items, want)
	}
}

func TestOtherStaffChatServiceFailsClosedWhenOwnerOrArchiveClassificationIsAmbiguous(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	for _, scenario := range []struct {
		name   string
		owner  *otherStaffChatOwnerFake
		record []OtherStaffChatRecord
	}{
		{name: "inactive owner", owner: &otherStaffChatOwnerFake{err: contactport.ErrStaffReferenceNotFound}},
		{name: "archive returns current owner", owner: &otherStaffChatOwnerFake{userID: "staff-current"}, record: []OtherStaffChatRecord{{StaffUserID: "staff-current", MessageType: "text", ContentMasked: "masked", SentAt: stamp}}},
		{name: "archive returns unsupported media", owner: &otherStaffChatOwnerFake{userID: "staff-current"}, record: []OtherStaffChatRecord{{StaffUserID: "staff-other", MessageType: "voice", ContentMasked: "masked", SentAt: stamp}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := &otherStaffChatTestStore{records: scenario.record}
			service := NewOtherStaffChatService(messageArchiveTestUoW{}, store, scenario.owner)
			page, err := service.ListCustomerOtherStaffChats(context.Background(), wecomport.CustomerOtherStaffChatQuery{CustomerID: 41, OwnerStaffID: 7})
			if !errors.Is(err, wecomport.ErrCustomerOtherStaffChatUnavailable) || len(page.Items) != 0 {
				t.Fatalf("page=%#v err=%v", page, err)
			}
			if scenario.owner.err != nil && store.calls != 0 {
				t.Fatalf("owner failure must not query archive, calls=%d", store.calls)
			}
		})
	}
}

type otherStaffChatTestStore struct {
	records      []OtherStaffChatRecord
	err          error
	customerID   contactport.CustomerID
	ownerUserID  string
	limit, calls int32
}

func (store *otherStaffChatTestStore) ListOtherStaffChatRecords(_ context.Context, customerID contactport.CustomerID, ownerUserID string, limit int32) ([]OtherStaffChatRecord, error) {
	store.calls++
	store.customerID, store.ownerUserID, store.limit = customerID, ownerUserID, limit
	return append([]OtherStaffChatRecord(nil), store.records...), store.err
}

type otherStaffChatOwnerFake struct {
	userID string
	err    error
	calls  int
}

func (owner *otherStaffChatOwnerFake) LockActiveWeComUserID(context.Context, int64) (string, error) {
	owner.calls++
	return owner.userID, owner.err
}

var _ OtherStaffChatStore = (*otherStaffChatTestStore)(nil)
var _ contactport.ActiveStaffSenderReader = (*otherStaffChatOwnerFake)(nil)
