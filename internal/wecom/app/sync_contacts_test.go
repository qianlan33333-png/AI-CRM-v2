package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestExternalContactSyncResumesCommittedCursorWithoutDuplicates(t *testing.T) {
	reader := &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{
		"":         {ExternalUserIDs: []string{"wo-1", "wo-2"}, NextCursor: "cursor-2"},
		"cursor-2": {ExternalUserIDs: []string{"wo-3"}},
	}}
	state := &memoryState{}
	service := NewExternalContactSyncService(immediateUoW{}, reader, state)
	first, err := service.SyncNext(context.Background(), "owner-1")
	if err != nil || !reflect.DeepEqual(first.ExternalUserIDs, []string{"wo-1", "wo-2"}) {
		t.Fatalf("first SyncNext() = %#v, %v", first, err)
	}
	// A new service represents an interrupted process restarting with only the
	// committed cursor state; it must not read or return the first page again.
	restarted := NewExternalContactSyncService(immediateUoW{}, reader, state)
	second, err := restarted.SyncNext(context.Background(), "owner-1")
	if err != nil || !reflect.DeepEqual(second.ExternalUserIDs, []string{"wo-3"}) {
		t.Fatalf("second SyncNext() = %#v, %v", second, err)
	}
	if !reflect.DeepEqual(reader.cursors, []string{"", "cursor-2"}) {
		t.Fatalf("reader cursors = %#v, want first then committed successor", reader.cursors)
	}
	if _, err = restarted.SyncNext(context.Background(), "owner-1"); !errors.Is(err, ErrCursorSyncDone) {
		t.Fatalf("completed SyncNext() error = %v, want completion", err)
	}
	if !reflect.DeepEqual(reader.cursors, []string{"", "cursor-2"}) {
		t.Fatalf("completed sync re-read a page: %#v", reader.cursors)
	}
}

func TestExternalContactSyncRetriesOnlyTheConcurrentCursorLoser(t *testing.T) {
	reader := &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{
		"":         {ExternalUserIDs: []string{"wo-1"}, NextCursor: "cursor-2"},
		"cursor-2": {ExternalUserIDs: []string{"wo-2"}},
	}}
	state := &memoryState{conflictOnce: true}
	service := NewExternalContactSyncService(immediateUoW{}, reader, state)
	page, err := service.SyncNext(context.Background(), "owner-1")
	if err != nil || !reflect.DeepEqual(page.ExternalUserIDs, []string{"wo-2"}) {
		t.Fatalf("SyncNext() = %#v, %v", page, err)
	}
	if !reflect.DeepEqual(reader.cursors, []string{"", "cursor-2"}) {
		t.Fatalf("reader cursors = %#v, want concurrent loser to continue at successor", reader.cursors)
	}
}

func TestExternalContactSyncFailsClosedBeforeCursorWrite(t *testing.T) {
	for name, page := range map[string]wecomclient.ExternalContactPage{
		"duplicate identifier": {ExternalUserIDs: []string{"wo-1", "wo-1"}},
		"non advancing cursor": {ExternalUserIDs: []string{"wo-1"}, NextCursor: "cursor-1"},
	} {
		t.Run(name, func(t *testing.T) {
			state := &memoryState{exists: name == "non advancing cursor"}
			cursor := ""
			if state.exists {
				cursor = "cursor-1"
				state.cursor = cursor
			}
			service := NewExternalContactSyncService(immediateUoW{}, &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{cursor: page}}, state)
			if _, err := service.SyncNext(context.Background(), "owner-1"); !errors.Is(err, ErrCursorSyncFailed) {
				t.Fatalf("SyncNext() error = %v, want fail closed", err)
			}
			if !state.exists && state.cursor != "" {
				t.Fatal("invalid provider page advanced durable cursor")
			}
		})
	}
}

type immediateUoW struct{}

func (immediateUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type fakePageReader struct {
	pages   map[string]wecomclient.ExternalContactPage
	cursors []string
}

func (reader *fakePageReader) ListExternalContacts(_ context.Context, _ string, cursor string) (wecomclient.ExternalContactPage, error) {
	reader.cursors = append(reader.cursors, cursor)
	page, exists := reader.pages[cursor]
	if !exists {
		return wecomclient.ExternalContactPage{}, errors.New("unexpected cursor")
	}
	return page, nil
}

type memoryState struct {
	exists       bool
	cursor       string
	completed    bool
	conflictOnce bool
}

func (state *memoryState) LoadCursor(context.Context, string) (CursorState, error) {
	if !state.exists {
		return CursorState{}, nil
	}
	return CursorState{Cursor: state.cursor, Completed: state.completed}, nil
}

func (state *memoryState) AdvanceCursor(_ context.Context, _ string, expected, next string, completed bool) error {
	if state.conflictOnce {
		state.conflictOnce = false
		state.exists, state.cursor, state.completed = true, "cursor-2", false
		return ErrCursorAdvanced
	}
	if state.exists && (state.cursor != expected || state.completed) {
		return ErrCursorAdvanced
	}
	state.exists, state.cursor, state.completed = true, next, completed
	return nil
}
