package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestMessageArchiveSyncAcceptsOnceInOneUoWWithoutExternalDispatch(t *testing.T) {
	store := &messageArchiveTestStore{}
	events := &messageArchiveTestEvents{}
	service := NewMessageArchiveService(messageArchiveTestUoW{}, store, events)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC) }
	command := ArchiveSyncCommand{
		Actor: "admin:7", IdempotencyKey: "archive-sync-test-key-0001", StartTime: "2000-01-01 00:00:00",
		EndTime: "2099-12-31 23:59:59", Limit: 100, MaxPages: 1000,
	}

	first, err := service.RequestSync(context.Background(), command)
	if err != nil || first.State != ArchiveSyncAccepted || first.EventID != 17 || store.acceptCalls != 1 || len(events.events) != 1 {
		t.Fatalf("first RequestSync()=%#v err=%v accepts=%d events=%#v", first, err, store.acceptCalls, events.events)
	}
	if got := events.events[0]; got.Type != "wecom.message_archive_sync_accepted" || got.IdempotencyKey != "wecom.message_archive_sync.accepted:1" || !got.OccurredAt.Equal(service.now()) {
		t.Fatalf("event=%#v", got)
	}
	if string(events.events[0].Payload) != `{"receipt_id":1,"state":"accepted"}` {
		t.Fatalf("event payload=%s", events.events[0].Payload)
	}

	replay, err := service.RequestSync(context.Background(), command)
	if err != nil || replay != first || store.acceptCalls != 1 || len(events.events) != 1 {
		t.Fatalf("replay=%#v err=%v accepts=%d events=%d", replay, err, store.acceptCalls, len(events.events))
	}
	conflicting := command
	conflicting.StartTime = "2026-08-15 00:00:00"
	if _, err = service.RequestSync(context.Background(), conflicting); !errors.Is(err, ErrArchiveSyncConflict) || store.acceptCalls != 1 || len(events.events) != 1 {
		t.Fatalf("conflict error=%v accepts=%d events=%d", err, store.acceptCalls, len(events.events))
	}
}

func TestMessageArchiveSyncDoesNotAcceptWhenEventAppendFails(t *testing.T) {
	store := &messageArchiveTestStore{}
	service := NewMessageArchiveService(messageArchiveTestUoW{}, store, &messageArchiveTestEvents{err: errors.New("event log unavailable")})
	_, err := service.RequestSync(context.Background(), validArchiveSyncCommandForTest())
	if !errors.Is(err, ErrArchiveSyncFailed) || store.acceptCalls != 0 {
		t.Fatalf("RequestSync() error=%v accepts=%d", err, store.acceptCalls)
	}
}

func TestMessageArchiveListRejectsOutOfScopeAndInvalidProjection(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	store := &messageArchiveTestStore{records: []ArchiveMessage{{
		ID: "1", SourceMessageID: "msg-1", ExternalUserID: "ext-1", ChatType: "group", MessageType: "text", SentAt: now,
	}}}
	service := NewMessageArchiveService(messageArchiveTestUoW{}, store, &messageArchiveTestEvents{})
	if _, _, err := service.List(context.Background(), ArchiveQuery{CustomerID: contactport.CustomerID(1), ChatType: "private", StartedAt: &now, Limit: 20, External: true}); !errors.Is(err, ErrMessageArchiveUnavailable) {
		t.Fatalf("List() error=%v, want invalid projection unavailable", err)
	}
	if _, _, err := service.List(context.Background(), ArchiveQuery{CustomerID: contactport.CustomerID(1), ChatType: "private", Limit: 201}); !errors.Is(err, ErrInvalidMessageArchiveQuery) {
		t.Fatalf("List() invalid boundary error=%v", err)
	}
}

func validArchiveSyncCommandForTest() ArchiveSyncCommand {
	return ArchiveSyncCommand{Actor: "admin:7", IdempotencyKey: "archive-sync-test-key-0001", StartTime: "2000-01-01 00:00:00", EndTime: "2099-12-31 23:59:59", Limit: 100, MaxPages: 1000}
}

type messageArchiveTestUoW struct{}

func (messageArchiveTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type messageArchiveTestEvents struct {
	events []eventport.Event
	err    error
}

func (events *messageArchiveTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.events = append(events.events, event)
	if events.err != nil {
		return 0, events.err
	}
	return 17, nil
}

type messageArchiveTestStore struct {
	digest      []byte
	accepted    bool
	acceptCalls int
	records     []ArchiveMessage
	listQueries []ArchiveQuery
}

func (store *messageArchiveTestStore) ReserveMessageArchiveSync(_ context.Context, _ ArchiveSyncCommand, digest []byte) (ArchiveSyncReceipt, []byte, error) {
	if store.digest == nil {
		store.digest = append([]byte(nil), digest...)
	}
	if store.accepted {
		return ArchiveSyncReceipt{ID: 1, State: ArchiveSyncAccepted, EventID: 17}, append([]byte(nil), store.digest...), nil
	}
	return ArchiveSyncReceipt{ID: 1, State: "reserved"}, append([]byte(nil), store.digest...), nil
}

func (store *messageArchiveTestStore) AcceptMessageArchiveSync(_ context.Context, receiptID int64, eventID eventport.EventID) (ArchiveSyncReceipt, []byte, error) {
	store.acceptCalls++
	store.accepted = true
	return ArchiveSyncReceipt{ID: receiptID, State: ArchiveSyncAccepted, EventID: eventID}, append([]byte(nil), store.digest...), nil
}

func (store *messageArchiveTestStore) MessageArchiveHealth(context.Context) (ArchiveHealth, error) {
	return ArchiveHealth{}, nil
}

func (store *messageArchiveTestStore) ListMessageArchive(_ context.Context, query ArchiveQuery) ([]ArchiveMessage, int64, error) {
	store.listQueries = append(store.listQueries, query)
	return append([]ArchiveMessage(nil), store.records...), int64(len(store.records)), nil
}

func TestMessageArchiveCustomerChatSummaryProjectsNoMessageBodyOrIdentity(t *testing.T) {
	sentAt := time.Date(2026, time.August, 20, 11, 0, 0, 0, time.UTC)
	store := &messageArchiveTestStore{records: []ArchiveMessage{{
		ID: "archive-local-1", SourceMessageID: "provider-message-id", ExternalUserID: "external-user-id", ChatType: "private",
		WithUserID: "staff-7", Sender: "external-user-id", Receiver: "staff-7", ChatID: "chat-id", RoomID: "room-id",
		GroupName: "group-name", MessageType: "text", Content: "sensitive body must not leak", SentAt: sentAt,
	}}}
	service := NewMessageArchiveService(messageArchiveTestUoW{}, store, nil)
	page, err := service.ListCustomerChatSummaries(context.Background(), wecomport.CustomerChatSummaryQuery{CustomerID: 41, Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListCustomerChatSummaries() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != (wecomport.CustomerChatSummary{ChatType: "private", MessageType: "text", SentAt: sentAt}) ||
		page.Total != 1 || page.Limit != 20 || page.Offset != 0 {
		t.Fatalf("safe page = %#v", page)
	}
	if len(store.listQueries) != 1 || store.listQueries[0] != (ArchiveQuery{CustomerID: 41, Limit: 20, Offset: 0}) {
		t.Fatalf("archive query = %#v", store.listQueries)
	}
	encoded, marshalErr := json.Marshal(page)
	if marshalErr != nil {
		t.Fatalf("marshal safe page: %v", marshalErr)
	}
	for _, forbidden := range []string{"sensitive body", "external-user-id", "provider-message-id", "chat-id", "room-id", "staff-7"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe page leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestMessageArchiveCustomerChatSummaryRejectsInvalidInputBeforeArchiveRead(t *testing.T) {
	store := &messageArchiveTestStore{}
	service := NewMessageArchiveService(messageArchiveTestUoW{}, store, nil)
	for _, query := range []wecomport.CustomerChatSummaryQuery{
		{CustomerID: 0, Limit: 20}, {CustomerID: 1, Limit: 0}, {CustomerID: 1, Limit: MessageArchiveMaximumLimit + 1}, {CustomerID: 1, Limit: 20, Offset: -1},
	} {
		page, err := service.ListCustomerChatSummaries(context.Background(), query)
		if !errors.Is(err, wecomport.ErrInvalidCustomerChatSummaryQuery) || !reflect.DeepEqual(page, wecomport.CustomerChatSummaryPage{}) {
			t.Fatalf("query=%#v page=%#v err=%v", query, page, err)
		}
	}
	if len(store.listQueries) != 0 {
		t.Fatalf("unexpected archive queries: %#v", store.listQueries)
	}
}

var _ MessageArchiveStore = (*messageArchiveTestStore)(nil)
var _ eventport.Appender = (*messageArchiveTestEvents)(nil)

func TestArchiveSyncDigestIncludesAcceptedCommandParameters(t *testing.T) {
	base := validArchiveSyncCommandForTest()
	for _, mutate := range []func(*ArchiveSyncCommand){
		func(command *ArchiveSyncCommand) { command.StartTime = "2001-01-01 00:00:00" },
		func(command *ArchiveSyncCommand) { command.EndTime = "2098-01-01 00:00:00" },
		func(command *ArchiveSyncCommand) { command.OwnerUserID = "staff-1" },
		func(command *ArchiveSyncCommand) { command.Cursor = "cursor-1" },
		func(command *ArchiveSyncCommand) { command.Limit = 101 },
		func(command *ArchiveSyncCommand) { command.MaxPages = 1001 },
	} {
		changed := base
		mutate(&changed)
		if reflect.DeepEqual(archiveSyncDigest(base), archiveSyncDigest(changed)) {
			t.Fatalf("digest omitted changed command: %#v", changed)
		}
	}
}
