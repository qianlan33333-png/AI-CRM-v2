package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestParseCallbackEnvelopeExternalContactIsStableAndScoped(t *testing.T) {
	message := []byte(`<xml><ToUserName><![CDATA[corp-a]]></ToUserName><CreateTime>1700000000</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[change_external_contact]]></Event><ChangeType><![CDATA[add_external_contact]]></ChangeType><UserID><![CDATA[staff-1]]></UserID><ExternalUserID><![CDATA[external-1]]></ExternalUserID></xml>`)
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

func TestParseExternalContactCallbackFactRetainsEntrantFieldsWithoutWelcomeCode(t *testing.T) {
	message := []byte(`<xml><ToUserName><![CDATA[corp-a]]></ToUserName><CreateTime>1700000000</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[change_external_contact]]></Event><ChangeType><![CDATA[add_external_contact]]></ChangeType><UserID><![CDATA[staff-1]]></UserID><ExternalUserID><![CDATA[external-1]]></ExternalUserID><State><![CDATA[ch02-state-1]]></State><WelcomeCode><![CDATA[welcome-code-secret]]></WelcomeCode><Source><![CDATA[link-source-secret]]></Source><FailReason><![CDATA[reason-secret]]></FailReason></xml>`)
	fact, err := ParseExternalContactCallbackFact(message, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	if !fact.IsEntrant() || fact.ChangeType != addExternalContact || fact.CorpID != "corp-a" || fact.UserID != "staff-1" || fact.ExternalUserID != "external-1" || fact.State != "ch02-state-1" || !fact.OccurredAt.Equal(time.Unix(1700000000, 0).UTC()) {
		t.Fatalf("fact = %#v", fact)
	}
	if !fact.WelcomeCodePresent || fact.WelcomeCodeDigest != callbackValueDigest("welcome-code-secret") || fact.SourceDigest != callbackValueDigest("link-source-secret") || fact.FailReasonDigest != callbackValueDigest("reason-secret") {
		t.Fatalf("unsafe digest fact = %#v", fact)
	}
	envelope, err := ParseCallbackEnvelope(message, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ExternalContact == nil || envelope.InitialState != "pending" || envelope.EventType != "change_external_contact:add_external_contact" {
		t.Fatalf("envelope = %#v", envelope)
	}
	stored := string(envelope.RawPayload)
	if strings.Contains(stored, "welcome-code-secret") || strings.Contains(stored, "link-source-secret") || strings.Contains(stored, "reason-secret") || !strings.Contains(stored, "[redacted]") || !strings.Contains(stored, "ch02-state-1") {
		t.Fatalf("redacted raw payload = %q", stored)
	}
}

func TestParseExternalContactCallbackFactHalfContactIsEntrant(t *testing.T) {
	message := []byte(`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_half_external_contact</ChangeType><UserID>staff-1</UserID><ExternalUserId>external-1</ExternalUserId></xml>`)
	fact, err := ParseExternalContactCallbackFact(message, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	if !fact.IsEntrant() || fact.ExternalUserID != "external-1" || fact.UserID != "staff-1" || fact.WelcomeCodePresent {
		t.Fatalf("fact = %#v", fact)
	}
}

func TestParseCallbackEnvelopeExternalContactLifecycleDoesNotBecomeEntrant(t *testing.T) {
	for _, changeType := range []string{"edit_external_contact", "del_external_contact", "del_follow_user", "transfer_success", "transfer_fail", "future_change"} {
		t.Run(changeType, func(t *testing.T) {
			message := []byte(`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>` + changeType + `</ChangeType><ExternalUserID>external-1</ExternalUserID><State>state-1</State><Source>source-secret</Source><FailReason>reason-secret</FailReason></xml>`)
			envelope, err := ParseCallbackEnvelope(message, "corp-a")
			if err != nil {
				t.Fatal(err)
			}
			if envelope.InitialState != "pending" || envelope.ExternalContact == nil || envelope.ExternalContact.IsEntrant() || envelope.ExternalContact.SourceDigest != callbackValueDigest("source-secret") || envelope.ExternalContact.FailReasonDigest != callbackValueDigest("reason-secret") {
				t.Fatalf("envelope = %#v", envelope)
			}
		})
	}
}

func TestParseExternalContactCallbackFactRejectsMalformedOrUnsafeEntrants(t *testing.T) {
	valid := `<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType><UserID>staff-1</UserID><ExternalUserID>external-1</ExternalUserID></xml>`
	for _, message := range []string{
		`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType><ExternalUserID>external-1</ExternalUserID></xml>`,
		strings.Replace(valid, "corp-a", "corp-b", 1),
		strings.Replace(valid, "</xml>", "</xml>trailing", 1),
		strings.Replace(valid, "</ExternalUserID>", "</ExternalUserID><ExternalUserId>external-2</ExternalUserId>", 1),
		`<xml><ToUserName>corp-a</ToUserName><CreateTime>never</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType><UserID>staff-1</UserID><ExternalUserID>external-1</ExternalUserID></xml>`,
	} {
		if _, err := ParseExternalContactCallbackFact([]byte(message), "corp-a"); !errors.Is(err, ErrInvalidInboundMessage) {
			t.Fatalf("ParseExternalContactCallbackFact(%q) error = %v", message, err)
		}
	}
}

func TestInboundServiceDeduplicatesAndQueuesOnlyBusinessEvents(t *testing.T) {
	store := &memoryInboundStore{}
	jobs := &memoryInboundJobs{}
	service, err := NewInboundService(immediateUoW{}, store, jobs, nil, "corp-a", func() time.Time { return time.Unix(1700000100, 0) })
	if err != nil {
		t.Fatal(err)
	}
	message := []byte(`<xml><ToUserName>corp-a</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType><UserID>staff-1</UserID><ExternalUserID>external-1</ExternalUserID></xml>`)
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
	for _, source := range []InboundSource{InboundSourceCallback, InboundSourceSync} {
		for _, status := range []identityport.IngestStatus{identityport.IngestAttributed, identityport.IngestPending, identityport.IngestConflict} {
			t.Run(string(source)+"/"+string(status), func(t *testing.T) {
				store := &memoryInboundStore{records: []memoryInboundRecord{{id: 7, source: source, sourceKey: "sha256:" + repeatedHex('a'), corpID: "corp-a", externalUserID: "external-1", occurredAt: time.Unix(1700000000, 0).UTC(), state: "pending"}}}
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
				if source == InboundSourceSync && ingestor.commands[0].Source != "wecom.sync" {
					t.Fatalf("sync source=%q, want wecom.sync", ingestor.commands[0].Source)
				}
			})
		}
	}
}

func TestInboundServiceUsesPersistedTypedEntrantWithoutRawXMLReparse(t *testing.T) {
	entrants, correlation, identities, receipts := entrantServiceFixture(t)
	correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: entrantMatch(41, 7, contactport.AcquisitionAssetQRCode, 3)}
	identities.result = identityport.AcquisitionEntrantIdentityResolution{Status: identityport.AcquisitionEntrantIdentityFound, CustomerID: 22}
	fact := entrantInput().Fact
	store := &memoryInboundStore{records: []memoryInboundRecord{{id: 7, source: InboundSourceCallback, sourceKey: "sha256:" + repeatedHex('a'), corpID: fact.CorpID, eventType: fact.EventType(), externalUserID: fact.ExternalUserID, externalContact: &fact, rawPayload: []byte("not XML"), occurredAt: fact.OccurredAt, state: "pending"}}}
	service, err := NewInboundServiceWithEntrants(immediateUoW{}, store, &memoryInboundJobs{}, nil, entrants, "corp-a", func() time.Time { return time.Unix(1700000100, 0) })
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Process(context.Background(), 7, "river:99"); err != nil {
		t.Fatal(err)
	}
	if store.records[0].state != "processed" || correlation.calls != 1 || identities.calls != 1 || receipts.events != 1 {
		t.Fatalf("state=%q correlation=%d identities=%d events=%d", store.records[0].state, correlation.calls, identities.calls, receipts.events)
	}
}

func TestInboundServiceAttributesPendingEntrantAfterIdentityIngest(t *testing.T) {
	entrants, correlation, identities, receipts := entrantServiceFixture(t)
	correlation.result = contactport.AcquisitionAssetCorrelationResolution{Cardinality: contactport.AcquisitionAssetCorrelationOne, Match: entrantMatch(41, 7, contactport.AcquisitionAssetQRCode, 3)}
	identities.results = []identityport.AcquisitionEntrantIdentityResolution{
		{Status: identityport.AcquisitionEntrantIdentityNotFound},
		{Status: identityport.AcquisitionEntrantIdentityFound, CustomerID: 22},
	}
	ingestor := &memoryInboundIngestor{result: identityport.IngestResult{Status: identityport.IngestAttributed, CustomerID: 22, EventID: 4}}
	processor, err := NewIdentityContactProcessor(ingestor)
	if err != nil {
		t.Fatal(err)
	}
	fact := entrantInput().Fact
	store := &memoryInboundStore{records: []memoryInboundRecord{{id: 7, source: InboundSourceCallback, sourceKey: "sha256:" + repeatedHex('a'), corpID: fact.CorpID, eventType: fact.EventType(), externalUserID: fact.ExternalUserID, externalContact: &fact, occurredAt: fact.OccurredAt, state: "pending"}}}
	service, err := NewInboundServiceWithEntrants(immediateUoW{}, store, &memoryInboundJobs{}, processor, entrants, "corp-a", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Process(context.Background(), 7, "river:99"); err != nil {
		t.Fatal(err)
	}
	if store.records[0].state != "processed" || identities.calls != 2 || receipts.events != 1 || receipts.byInbox[7].Status != contactport.ChannelAcquisitionEntrantAttributed {
		t.Fatalf("state=%q identity_calls=%d events=%d receipt=%#v", store.records[0].state, identities.calls, receipts.events, receipts.byInbox[7])
	}
}

type memoryInboundRecord struct {
	id, fence       int64
	source          InboundSource
	sourceKey       string
	corpID          string
	eventType       string
	externalUserID  string
	rawPayload      []byte
	externalContact *ExternalContactCallbackFact
	occurredAt      time.Time
	state           string
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
	record := memoryInboundRecord{id: int64(len(store.records) + 1), source: envelope.Source, sourceKey: envelope.SourceKey, corpID: envelope.CorpID, eventType: envelope.EventType, externalUserID: envelope.ExternalUserID, externalContact: envelope.ExternalContact, rawPayload: append([]byte(nil), envelope.RawPayload...), occurredAt: envelope.OccurredAt, state: state}
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
			return InboundRecord{ID: id, Source: store.records[index].source, SourceKey: store.records[index].sourceKey, CorpID: store.records[index].corpID, EventType: store.records[index].eventType, ExternalUserID: store.records[index].externalUserID, ExternalContact: store.records[index].externalContact, RawPayload: append([]byte(nil), store.records[index].rawPayload...), OccurredAt: store.records[index].occurredAt, State: "processing", LeaseFence: store.records[index].fence}, nil
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
