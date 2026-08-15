package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
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

func (store *messageArchiveTestStore) ListMessageArchive(_ context.Context, _ ArchiveQuery) ([]ArchiveMessage, int64, error) {
	return append([]ArchiveMessage(nil), store.records...), int64(len(store.records)), nil
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
