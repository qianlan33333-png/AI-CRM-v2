package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

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
		"control identifier":   {ExternalUserIDs: []string{"wo-\x01"}},
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

func TestExternalContactSyncPersistsEveryFactBeforeAdvancingCursor(t *testing.T) {
	reader := &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{
		"": {ExternalUserIDs: []string{"wo-1", "wo-2"}, NextCursor: "cursor-2"},
	}}
	state := &memoryState{}
	handoff := &memorySyncHandoff{}
	jobs := &memorySyncJobs{}
	service, err := NewExternalContactSyncServiceWithHandoff(
		immediateUoW{}, reader, state, handoff, jobs, "corp-a", func() time.Time { return time.Unix(1700000000, 0).UTC() },
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.SyncNext(context.Background(), "owner-1")
	if err != nil || !reflect.DeepEqual(page.ExternalUserIDs, []string{"wo-1", "wo-2"}) {
		t.Fatalf("SyncNext() = %#v, %v", page, err)
	}
	if len(handoff.facts) != 2 || len(jobs.args) != 2 || state.cursor != "cursor-2" {
		t.Fatalf("facts=%#v jobs=%#v cursor=%q", handoff.facts, jobs.args, state.cursor)
	}
}

func TestExternalContactSyncDoesNotAdvanceWhenHandoffJobFails(t *testing.T) {
	reader := &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{
		"": {ExternalUserIDs: []string{"wo-1"}, NextCursor: "cursor-2"},
	}}
	state := &memoryState{}
	handoff := &memorySyncHandoff{}
	jobs := &memorySyncJobs{err: errors.New("queue unavailable")}
	service, err := NewExternalContactSyncServiceWithHandoff(
		immediateUoW{}, reader, state, handoff, jobs, "corp-a", time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SyncNext(context.Background(), "owner-1"); !errors.Is(err, ErrCursorSyncFailed) || state.cursor != "" {
		t.Fatalf("SyncNext() = %v cursor=%q", err, state.cursor)
	}
}

func TestExternalContactSyncDoesNotQueueAnAlreadyReservedHandoffAgain(t *testing.T) {
	const staffUserID = "owner-1"
	key := "external_contact_list:" + staffUserID
	factID := syncFactID("corp-a", key, "", 0, "wo-1")
	handoff := &memorySyncHandoff{facts: []SyncHandoff{{FactID: factID}}}
	jobs := &memorySyncJobs{}
	state := &memoryState{}
	service, err := NewExternalContactSyncServiceWithHandoff(
		immediateUoW{}, &fakePageReader{pages: map[string]wecomclient.ExternalContactPage{
			"": {ExternalUserIDs: []string{"wo-1"}, NextCursor: "cursor-2"},
		}}, state, handoff, jobs, "corp-a", time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SyncNext(context.Background(), staffUserID); err != nil {
		t.Fatalf("SyncNext() error = %v", err)
	}
	if len(handoff.facts) != 1 || len(jobs.args) != 0 || state.cursor != "cursor-2" {
		t.Fatalf("facts=%#v jobs=%#v cursor=%q", handoff.facts, jobs.args, state.cursor)
	}
}

func TestExternalContactSyncDisabledReaderDoesNotAdvanceCursor(t *testing.T) {
	state := &memoryState{}
	service := NewExternalContactSyncService(immediateUoW{}, wecomclient.NewDisabledExternalContactReader(), state)
	if _, err := service.SyncNext(context.Background(), "owner-1"); !errors.Is(err, ErrCursorSyncDisabled) {
		t.Fatalf("SyncNext() error = %v, want disabled", err)
	}
	if state.exists || state.cursor != "" || state.completed {
		t.Fatalf("disabled sync changed state: %#v", state)
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

type memorySyncHandoff struct{ facts []SyncHandoff }

func (handoff *memorySyncHandoff) ReserveSyncFact(_ context.Context, fact SyncHandoff) (InboundReservation, error) {
	for index, existing := range handoff.facts {
		if existing.FactID == fact.FactID {
			return InboundReservation{ID: int64(index + 1), Inserted: false}, nil
		}
	}
	handoff.facts = append(handoff.facts, fact)
	return InboundReservation{ID: int64(len(handoff.facts)), Inserted: true}, nil
}

func (handoff *memorySyncHandoff) MarkInboundQueued(context.Context, int64, int64) error { return nil }

type memorySyncJobs struct {
	args []InboundJobArgs
	err  error
}

func (jobs *memorySyncJobs) Insert(_ context.Context, args InboundJobArgs) (int64, error) {
	if jobs.err != nil {
		return 0, jobs.err
	}
	jobs.args = append(jobs.args, args)
	return int64(len(jobs.args)), nil
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
