package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestParseCallbackEnvelopeExternalContactIsStableAndScoped(t *testing.T) {
	message := []byte(`<xml><ToUserName><![CDATA[corp-a]]></ToUserName><CreateTime>1700000000</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[change_external_contact]]></Event><ChangeType><![CDATA[add_external_contact]]></ChangeType><ExternalUserID><![CDATA[external-1]]></ExternalUserID></xml>`)
	first, err := ParseCallbackEnvelope(message, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseCallbackEnvelope(message, "corp-a")
	if err != nil || first.SourceKey != second.SourceKey || first.EventType != "change_external_contact:add_external_contact" || first.ExternalUserID != "external-1" {
		t.Fatalf("parsed envelopes = %#v / %#v, err=%v", first, second, err)
	}
	if _, err = ParseCallbackEnvelope(message, "corp-b"); !errors.Is(err, ErrInvalidInboundMessage) {
		t.Fatalf("wrong corp error = %v", err)
	}
}

func TestInboundServiceDeduplicatesAndQueuesOnlyBusinessEvents(t *testing.T) {
	store := &memoryInboundStore{}
	jobs := &memoryInboundJobs{}
	service, err := NewInboundService(immediateUoW{}, store, jobs, nil, "corp-a", func() time.Time { return time.Unix(1700000100, 0) })
	if err != nil {
		t.Fatal(err)
	}
	message := []byte(`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ExternalUserID>external-1</ExternalUserID></xml>`)
	if err = service.Dispatch(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err = service.Dispatch(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(store.records) != 1 || len(jobs.args) != 1 || store.records[0].state != "pending" {
		t.Fatalf("records=%#v jobs=%#v", store.records, jobs.args)
	}
	enterAgent := []byte(`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000001</CreateTime><MsgType>event</MsgType><Event>enter_agent</Event></xml>`)
	if err = service.Dispatch(context.Background(), enterAgent); err != nil {
		t.Fatal(err)
	}
	if len(store.records) != 2 || len(jobs.args) != 1 || store.records[1].state != "skipped" {
		t.Fatalf("enter_agent records=%#v jobs=%#v", store.records, jobs.args)
	}
}

func TestInboundServiceProcessesAttributedPendingAndConflictLocally(t *testing.T) {
	for _, status := range []identityport.IngestStatus{identityport.IngestAttributed, identityport.IngestPending, identityport.IngestConflict} {
		t.Run(string(status), func(t *testing.T) {
			store := &memoryInboundStore{records: []memoryInboundRecord{{id: 7, source: InboundSourceCallback, sourceKey: "sha256:" + repeatedHex('a'), corpID: "corp-a", externalUserID: "external-1", occurredAt: time.Unix(1700000000, 0).UTC(), state: "pending"}}}
			jobs := &memoryInboundJobs{}
			ingestor := &memoryInboundIngestor{result: identityport.IngestResult{Status: status, CustomerID: 12, EventID: 4, PendingEventID: 8}}
			processor, err := NewIdentityContactProcessor(ingestor)
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewInboundService(immediateUoW{}, store, jobs, processor, "corp-a", func() time.Time { return time.Unix(1700000100, 0) })
			if err != nil {
				t.Fatal(err)
			}
			if err = service.Process(context.Background(), 7, "river:99"); err != nil {
				t.Fatal(err)
			}
			want := "processed"
			if status == identityport.IngestPending {
				want = "pending_identity"
			} else if status == identityport.IngestConflict {
				want = "conflict"
			}
			if store.records[0].state != want || len(ingestor.commands) != 1 {
				t.Fatalf("state=%q commands=%#v", store.records[0].state, ingestor.commands)
			}
		})
	}
}

type memoryInboundRecord struct {
	id, fence      int64
	source         InboundSource
	sourceKey      string
	corpID         string
	eventType      string
	externalUserID string
	rawPayload     []byte
	occurredAt     time.Time
	state          string
}

type memoryInboundStore struct{ records []memoryInboundRecord }

func (store *memoryInboundStore) ReserveInbound(_ context.Context, envelope InboundEnvelope) (InboundReservation, error) {
	for _, record := range store.records {
		if record.source == envelope.Source && record.sourceKey == envelope.SourceKey {
			return InboundReservation{ID: record.id, State: record.state}, nil
		}
	}
	state := envelope.InitialState
	if state == "" {
		state = "pending"
	}
	record := memoryInboundRecord{id: int64(len(store.records) + 1), source: envelope.Source, sourceKey: envelope.SourceKey, corpID: envelope.CorpID, eventType: envelope.EventType, externalUserID: envelope.ExternalUserID, rawPayload: append([]byte(nil), envelope.RawPayload...), occurredAt: envelope.OccurredAt, state: state}
	store.records = append(store.records, record)
	return InboundReservation{ID: record.id, Inserted: true, State: record.state}, nil
}

func (store *memoryInboundStore) MarkInboundQueued(context.Context, int64, int64) error { return nil }

func (store *memoryInboundStore) ClaimInbound(_ context.Context, id int64, _ string, _ time.Time) (InboundRecord, error) {
	for index := range store.records {
		if store.records[index].id == id {
			if store.records[index].state != "pending" && store.records[index].state != "failed" {
				return InboundRecord{}, ErrInboundAlreadyDone
			}
			store.records[index].state = "processing"
			store.records[index].fence++
			return InboundRecord{ID: id, Source: store.records[index].source, SourceKey: store.records[index].sourceKey, CorpID: store.records[index].corpID, ExternalUserID: store.records[index].externalUserID, OccurredAt: store.records[index].occurredAt, State: "processing", LeaseFence: store.records[index].fence}, nil
		}
	}
	return InboundRecord{}, ErrInboundAlreadyDone
}

func (store *memoryInboundStore) CompleteInbound(_ context.Context, id, fence int64, state string) error {
	for index := range store.records {
		if store.records[index].id == id && store.records[index].fence == fence {
			store.records[index].state = state
			return nil
		}
	}
	return errors.New("missing claim")
}

func (store *memoryInboundStore) FailInbound(context.Context, int64, int64, string) error { return nil }

type memoryInboundJobs struct{ args []InboundJobArgs }

func (jobs *memoryInboundJobs) Insert(_ context.Context, args InboundJobArgs) (int64, error) {
	jobs.args = append(jobs.args, args)
	return int64(len(jobs.args)), nil
}

type memoryInboundIngestor struct {
	commands []identityport.IngestCommand
	result   identityport.IngestResult
}

func (ingestor *memoryInboundIngestor) Ingest(_ context.Context, command identityport.IngestCommand) (identityport.IngestResult, error) {
	ingestor.commands = append(ingestor.commands, command)
	return ingestor.result, nil
}

func repeatedHex(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}

func _jsonPayload(value any) []byte {
	payload, _ := json.Marshal(value)
	return payload
}
