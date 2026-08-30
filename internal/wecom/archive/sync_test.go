package archive

import (
	"context"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
)

type immediateUOW struct{}

func (immediateUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type providerFake struct {
	encrypted []EncryptedRecord
	payloads  []map[string]any
}

func (fake providerFake) FetchPage(context.Context, int64, int) ([]EncryptedRecord, error) {
	return fake.encrypted, nil
}

func (fake providerFake) Decrypt(context.Context, []EncryptedRecord) ([]map[string]any, error) {
	return fake.payloads, nil
}

type resolverFake struct{ result identityport.ResolveResult }

func (fake resolverFake) Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error) {
	return fake.result, nil
}

type storeFake struct {
	state    State
	records  []Record
	finished RunResult
	failure  string
}

func (fake *storeFake) State(context.Context) (State, error)      { return fake.state, nil }
func (*storeFake) StartRun(context.Context, int64) (int64, error) { return 41, nil }
func (fake *storeFake) SaveBatch(_ context.Context, records []Record, cursor int64, _ time.Time) (int64, int64, error) {
	fake.records = append(fake.records, records...)
	fake.state.LastSeq = cursor
	var unresolved int64
	for _, record := range records {
		if record.CustomerID == nil {
			unresolved++
		}
	}
	return int64(len(records)), unresolved, nil
}
func (fake *storeFake) FinishRun(_ context.Context, result RunResult, failure string, _ time.Time) error {
	fake.finished, fake.failure = result, failure
	return nil
}
func (*storeFake) ResolvePending(context.Context, string) (int64, error) { return 0, nil }

type appenderFake struct{ events []eventport.Event }

func (fake *appenderFake) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	fake.events = append(fake.events, event)
	return eventport.EventID(len(fake.events)), nil
}

func TestSyncPersistsUnresolvedTextAndAdvancesPastUnsupportedMessages(t *testing.T) {
	store := &storeFake{}
	events := &appenderFake{}
	provider := providerFake{
		encrypted: []EncryptedRecord{{Seq: 1}, {Seq: 2}, {Seq: 3}},
		payloads: []map[string]any{
			{"msgid": "msg-1", "msgtype": "text", "from": "wm_external", "tolist": []any{"staff-1"}, "msgtime": float64(1_700_000_000_000), "text": map[string]any{"content": "call 13800138000"}},
			{"msgid": "msg-2", "msgtype": "image", "from": "staff-1", "tolist": []any{"wm_external"}, "msgtime": float64(1_700_000_000_001)},
			{"msgid": "msg-3", "msgtype": "text", "from": "wm_group_external", "tolist": []any{}, "roomid": "wr_group", "msgtime": float64(1_700_000_000_002), "text": map[string]any{"content": "group update"}},
		},
	}
	service, err := NewService(immediateUOW{}, store, provider, resolverFake{result: identityport.ResolveResult{Status: identityport.ResolveNotFound}}, events, "ww-corp", "", 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(1_700_000_100, 0) }
	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID != 41 || result.CursorFrom != 0 || result.CursorTo != 3 || result.Fetched != 3 || result.Accepted != 2 || result.Inserted != 2 || result.Unresolved != 2 || result.PageCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.records) != 2 || store.records[0].Content != "call [masked-phone]" || store.records[0].CustomerID != nil || store.records[0].ProviderSeq != 1 || store.records[1].ExternalUserID != "wm_group_external" || store.records[1].OwnerUserID != "" || store.records[1].RoomID != "wr_group" {
		t.Fatalf("records = %+v", store.records)
	}
	if store.finished != result || store.failure != "" || len(events.events) != 1 || events.events[0].Type != "wecom.message_archive_batch_persisted" {
		t.Fatalf("finished=%+v failure=%q events=%+v", store.finished, store.failure, events.events)
	}
}

func TestRegisterWorkerBindsArchiveSyncToSyncQueue(t *testing.T) {
	registry := platformjobqueue.NewWorkerRegistry()
	if err := RegisterWorker(registry, &Service{}); err != nil {
		t.Fatal(err)
	}
	options, err := registry.ExplicitOptions(platformjobqueue.QueueSync, JobArgs{}, nil)
	if err != nil || options.Queue != string(platformjobqueue.QueueSync) {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}
