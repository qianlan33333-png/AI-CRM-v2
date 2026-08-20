package app

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	event "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestManagerCompletesLocalLifecycleAndReplaysReceipts(t *testing.T) {
	store := &manageStoreStub{items: map[string]hxc.SenderConfig{}}
	events := &manageEventStub{}
	manager := NewManager(manageUOWStub{}, store, manageStaffStub{}, events)
	manager.now = func() time.Time { return time.Date(2026, 8, 20, 5, 0, 0, 0, time.UTC) }

	command := ManageCommand{ID: "cfg-1", SenderUserID: "alice", DisplayName: "Alice", Priority: 4, Active: true, Actor: "admin:7", IdempotencyKey: "hxc-save-key-00000001"}
	first, err := manager.Save(context.Background(), command)
	if err != nil || first.ID != "cfg-1" || first.SenderUserID != "alice" {
		t.Fatalf("first save = %#v, %v", first, err)
	}
	replay, err := manager.Save(context.Background(), command)
	if err != nil || replay != first || store.saveCalls != 1 || len(events.events) != 1 {
		t.Fatalf("save replay = %#v, %v calls=%d events=%d", replay, err, store.saveCalls, len(events.events))
	}

	if _, err = manager.Save(context.Background(), ManageCommand{ID: "cfg-2", SenderUserID: "bob", DisplayName: "Bob", Priority: 9, Active: false, Actor: "admin:7", IdempotencyKey: "hxc-save-key-00000002"}); err != nil {
		t.Fatal(err)
	}
	ordered, err := manager.Reorder(context.Background(), "admin:7", "hxc-reorder-key-0001", []string{"cfg-2", "cfg-1"})
	if err != nil || len(ordered) != 2 || ordered[0].ID != "cfg-2" || ordered[0].Priority != 0 || ordered[1].Priority != 1 {
		t.Fatalf("reorder = %#v, %v", ordered, err)
	}
	replayedOrder, err := manager.Reorder(context.Background(), "admin:7", "hxc-reorder-key-0001", []string{"cfg-2", "cfg-1"})
	if err != nil || len(replayedOrder) != 2 || replayedOrder[0].ID != "cfg-2" || len(events.events) != 3 {
		t.Fatalf("reorder replay = %#v, %v events=%d", replayedOrder, err, len(events.events))
	}
	if err = manager.Archive(context.Background(), ManageCommand{SenderUserID: "alice", Actor: "admin:7", IdempotencyKey: "hxc-archive-key-0001"}); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 1 || store.items["cfg-2"].SenderUserID != "bob" || len(events.events) != 4 {
		t.Fatalf("final items=%#v events=%d", store.items, len(events.events))
	}
}

func TestManagerRejectsReorderReceiptPayloadDriftWithoutSecondMutation(t *testing.T) {
	store := &manageStoreStub{items: map[string]hxc.SenderConfig{
		"cfg-1": {ID: "cfg-1", SenderUserID: "alice", Priority: 0},
		"cfg-2": {ID: "cfg-2", SenderUserID: "bob", Priority: 1},
	}}
	events := &manageEventStub{}
	manager := NewManager(manageUOWStub{}, store, manageStaffStub{}, events)

	first, err := manager.Reorder(context.Background(), "admin:7", "hxc-reorder-drift-01", []string{"cfg-2", "cfg-1"})
	if err != nil || len(first) != 2 || first[0].ID != "cfg-2" {
		t.Fatalf("first reorder = %#v, %v", first, err)
	}
	second, err := manager.Reorder(context.Background(), "admin:7", "hxc-reorder-drift-01", []string{"cfg-1", "cfg-2"})
	if err != ErrConfigConflict || second != nil || len(events.events) != 1 {
		t.Fatalf("payload drift = %#v, %v events=%d", second, err, len(events.events))
	}
	current, err := manager.List(context.Background())
	if err != nil || current[0].ID != "cfg-2" || current[1].ID != "cfg-1" {
		t.Fatalf("order changed after rejected drift = %#v, %v", current, err)
	}
}

func TestManagerRejectsIncompleteOrUnknownReorderSetsWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		ids  []string
	}{
		{name: "missing current config", ids: []string{"cfg-2"}},
		{name: "unknown extra config", ids: []string{"cfg-2", "cfg-1", "cfg-3"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &manageStoreStub{items: map[string]hxc.SenderConfig{
				"cfg-1": {ID: "cfg-1", SenderUserID: "alice", Priority: 0},
				"cfg-2": {ID: "cfg-2", SenderUserID: "bob", Priority: 1},
			}}
			manager := NewManager(manageUOWStub{}, store, manageStaffStub{}, nil)
			got, err := manager.Reorder(context.Background(), "admin:7", "hxc-reorder-set-0001", test.ids)
			if err != ErrConfigConflict || got != nil || store.reorderCalls != 0 {
				t.Fatalf("reorder = %#v, %v calls=%d", got, err, store.reorderCalls)
			}
			current, listErr := manager.List(context.Background())
			if listErr != nil || current[0].ID != "cfg-1" || current[0].Priority != 0 || current[1].ID != "cfg-2" || current[1].Priority != 1 {
				t.Fatalf("current = %#v, %v", current, listErr)
			}
		})
	}
}

func TestManagerRejectsIneligibleAndInvalidCommandsBeforeMutation(t *testing.T) {
	store := &manageStoreStub{items: map[string]hxc.SenderConfig{}}
	manager := NewManager(manageUOWStub{}, store, manageStaffStub{}, nil)
	_, err := manager.Save(context.Background(), ManageCommand{ID: "cfg", SenderUserID: "provider-only", Actor: "admin:7", IdempotencyKey: "hxc-save-key-00000003"})
	if err != ErrConfigConflict || store.saveCalls != 0 {
		t.Fatalf("ineligible save err=%v calls=%d", err, store.saveCalls)
	}
	_, err = manager.Reorder(context.Background(), "admin:7", "short", []string{"cfg", "cfg"})
	if err != ErrInvalidCommand {
		t.Fatalf("invalid reorder err=%v", err)
	}
}

type manageUOWStub struct{}

func (manageUOWStub) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type manageStaffStub struct{}

func (manageStaffStub) ListEligibleStaff(context.Context) ([]contact.StaffDirectoryEntry, error) {
	return []contact.StaffDirectoryEntry{{WeComUserID: "alice", DisplayName: "Alice"}, {WeComUserID: "bob", DisplayName: "Bob"}}, nil
}

type manageReceipt struct {
	payload [32]byte
	result  json.RawMessage
}

type manageStoreStub struct {
	items        map[string]hxc.SenderConfig
	receipts     map[string]manageReceipt
	saveCalls    int
	reorderCalls int
}

func (store *manageStoreStub) ListSenderConfigs(context.Context) ([]hxc.SenderConfig, error) {
	items := make([]hxc.SenderConfig, 0, len(store.items))
	for _, item := range store.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Priority < items[j].Priority })
	return items, nil
}

func (store *manageStoreStub) SaveSenderConfig(_ context.Context, item hxc.SenderConfig) (hxc.SenderConfig, error) {
	store.saveCalls++
	item.CreatedAt = time.Date(2026, 8, 20, 5, 0, 0, 0, time.UTC)
	item.UpdatedAt = item.CreatedAt
	store.items[item.ID] = item
	return item, nil
}

func (store *manageStoreStub) DeleteSenderConfig(_ context.Context, senderUserID string) error {
	for id, item := range store.items {
		if item.SenderUserID == senderUserID {
			delete(store.items, id)
			return nil
		}
	}
	return ErrConfigConflict
}

func (store *manageStoreStub) ReorderSenderConfigs(_ context.Context, ids []string) ([]hxc.SenderConfig, error) {
	store.reorderCalls++
	for priority, id := range ids {
		item, ok := store.items[id]
		if !ok {
			return nil, ErrConfigConflict
		}
		item.Priority = priority
		store.items[id] = item
	}
	return store.ListSenderConfigs(context.Background())
}

func (store *manageStoreStub) ReserveSenderReceipt(_ context.Context, operation, actor string, key, payload [32]byte, _ time.Time) (json.RawMessage, bool, error) {
	if store.receipts == nil {
		store.receipts = map[string]manageReceipt{}
	}
	receipt, found := store.receipts[operation+actor+string(key[:])]
	if found {
		if receipt.payload != payload {
			return nil, true, ErrConfigConflict
		}
		return receipt.result, true, nil
	}
	store.receipts[operation+actor+string(key[:])] = manageReceipt{payload: payload}
	return nil, false, nil
}

func (store *manageStoreStub) CompleteSenderReceipt(_ context.Context, operation, actor string, key [32]byte, result json.RawMessage, _ time.Time) error {
	receiptKey := operation + actor + string(key[:])
	receipt := store.receipts[receiptKey]
	receipt.result = append(json.RawMessage(nil), result...)
	store.receipts[receiptKey] = receipt
	return nil
}

type manageEventStub struct{ events []event.Event }

func (store *manageEventStub) Append(_ context.Context, value event.Event) (event.EventID, error) {
	store.events = append(store.events, value)
	return event.EventID(len(store.events)), nil
}
