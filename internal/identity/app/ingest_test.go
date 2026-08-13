package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

var ingestTestReceiptKey = []byte("identity-ingest-receipt-key-v1-32")

func TestIngestAttributesUniqueRootAndReplaysCanonicalCommand(t *testing.T) {
	store := newIngestTestStore()
	store.lookups[normalizedTestKey(identityport.KindPhone, "phone:e164", "+8613800138000")] = ResolveRecord{CustomerID: 41}
	contacts := &ingestTestContacts{eventID: 73}
	events := &ingestTestEvents{}
	service := NewIngestService(ingestTestUoW{}, store, contacts, events, ingestTestReceiptKey)
	command := validIngestCommandForTest()

	first, err := service.Ingest(context.Background(), command)
	want := identityport.IngestResult{Status: identityport.IngestAttributed, CustomerID: 41, EventID: 73}
	if err != nil || first != want {
		t.Fatalf("Ingest()=%+v err=%v, want %+v", first, err, want)
	}
	replayCommand := command
	replayCommand.Refs = []identityport.IDRef{command.Refs[1], {
		Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+86 138 0013 8000",
		Assurance: identityport.AssuranceVerified, Source: command.Source,
	}}
	replayCommand.Payload = json.RawMessage(`{"nested":{"n":9007199254740993.0},"a":1e0}`)
	replayCommand.OccurredAt = command.OccurredAt.In(time.FixedZone("CST", 8*60*60))
	replay, err := service.Ingest(context.Background(), replayCommand)
	if err != nil || replay != first {
		t.Fatalf("canonical replay=%+v err=%v, want %+v", replay, err, first)
	}
	if contacts.calls != 1 || store.pendingCalls != 0 || events.countType("survey.submitted") != 1 {
		t.Fatalf("side effects contacts=%d pending=%d events=%+v", contacts.calls, store.pendingCalls, events.events)
	}
}

func TestIngestPersistsPendingAndConflictWithoutAttribution(t *testing.T) {
	for _, test := range []struct {
		name    string
		lookups map[string]ResolveRecord
		want    identityport.IngestStatus
	}{
		{name: "zero root", lookups: map[string]ResolveRecord{}, want: identityport.IngestPending},
		{name: "multiple roots", lookups: map[string]ResolveRecord{
			normalizedTestKey(identityport.KindPhone, "phone:e164", "+8613800138000"):                {CustomerID: 41},
			normalizedTestKey(identityport.KindUnionID, "wechat-open-platform:account-a", "union-a"): {CustomerID: 42},
		}, want: identityport.IngestConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newIngestTestStore()
			store.lookups = test.lookups
			contacts := &ingestTestContacts{eventID: 73}
			events := &ingestTestEvents{}
			result, err := NewIngestService(ingestTestUoW{}, store, contacts, events, ingestTestReceiptKey).
				Ingest(context.Background(), validIngestCommandForTest())
			if err != nil || result.Status != test.want || result.PendingEventID <= 0 || result.CustomerID != 0 || result.EventID != 0 {
				t.Fatalf("Ingest()=%+v err=%v, want %s pending fact", result, err, test.want)
			}
			if contacts.calls != 0 || store.pendingCalls != 1 || store.lastPending.Status != test.want || len(store.lastPending.Payload) == 0 {
				t.Fatalf("side effects contacts=%d pending=%+v", contacts.calls, store.lastPending)
			}
			if events.countType("identity.ingest."+string(test.want)) != 1 {
				t.Fatalf("events=%+v", events.events)
			}
		})
	}
}

func TestIngestSameKeyDifferentPayloadFailsClosed(t *testing.T) {
	store := newIngestTestStore()
	store.lookups[normalizedTestKey(identityport.KindPhone, "phone:e164", "+8613800138000")] = ResolveRecord{CustomerID: 41}
	contacts := &ingestTestContacts{eventID: 73}
	service := NewIngestService(ingestTestUoW{}, store, contacts, &ingestTestEvents{}, ingestTestReceiptKey)
	command := validIngestCommandForTest()
	if _, err := service.Ingest(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.Payload = json.RawMessage(`{"a":2,"nested":{"n":9007199254740993}}`)
	if _, err := service.Ingest(context.Background(), command); !errors.Is(err, ErrIdentityIngestIdempotencyConflict) {
		t.Fatalf("changed payload error=%v", err)
	}
	if contacts.calls != 1 || store.pendingCalls != 0 {
		t.Fatalf("changed payload added effects contacts=%d pending=%d", contacts.calls, store.pendingCalls)
	}
}

func TestIngestRejectsInvalidEvidenceAndPayloadBeforeUoW(t *testing.T) {
	valid := validIngestCommandForTest()
	tests := []identityport.IngestCommand{
		{},
		func() identityport.IngestCommand { value := valid; value.Payload = json.RawMessage(`[]`); return value }(),
		func() identityport.IngestCommand { value := valid; value.Refs[0].Source = "other"; return value }(),
		func() identityport.IngestCommand { value := valid; value.Refs[0].Assurance = "guessed"; return value }(),
	}
	for index, command := range tests {
		uow := &ingestCountingUoW{}
		if _, err := NewIngestService(uow, newIngestTestStore(), &ingestTestContacts{}, &ingestTestEvents{}, ingestTestReceiptKey).
			Ingest(context.Background(), command); err == nil {
			t.Fatalf("case %d accepted invalid command", index)
		}
		if uow.calls != 0 {
			t.Fatalf("case %d entered UoW %d times", index, uow.calls)
		}
	}
}

func validIngestCommandForTest() identityport.IngestCommand {
	return identityport.IngestCommand{
		Refs: []identityport.IDRef{
			{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 ", Assurance: identityport.AssuranceVerified, Source: "survey.callback"},
			{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:account-a", Value: " union-a ", Assurance: identityport.AssuranceDeclared, Source: "survey.callback"},
		},
		EventType: "survey.submitted", Payload: json.RawMessage(`{"a":1,"nested":{"n":9007199254740993}}`),
		Source: "survey.callback", OccurredAt: time.Date(2026, 8, 13, 3, 4, 5, 123456789, time.UTC), IdempotencyKey: "survey:event:1",
	}
}

type ingestTestUoW struct{}

func (ingestTestUoW) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type ingestCountingUoW struct{ calls int }

func (uow *ingestCountingUoW) Within(ctx context.Context, fn func(context.Context) error) error {
	uow.calls++
	return fn(ctx)
}

type ingestTestStore struct {
	mu          sync.Mutex
	nextID      int64
	identities  map[string]int64
	lookups     map[string]ResolveRecord
	receipts    map[string]IngestReceipt
	reservation map[int64]struct {
		key     string
		payload []byte
	}
	pendingCalls int
	lastPending  PendingIngest
}

func newIngestTestStore() *ingestTestStore {
	return &ingestTestStore{nextID: 10, identities: map[string]int64{}, lookups: map[string]ResolveRecord{}, receipts: map[string]IngestReceipt{}, reservation: map[int64]struct {
		key     string
		payload []byte
	}{}}
}

func (store *ingestTestStore) UpsertNormalized(_ context.Context, identity NormalizedIdentity) (int64, bool, error) {
	key := normalizedTestKey(identity.Kind, identity.Scope, identity.NormalizedValue)
	if id, found := store.identities[key]; found {
		return id, false, nil
	}
	store.nextID++
	store.identities[key] = store.nextID
	return store.nextID, true, nil
}

func (store *ingestTestStore) LookupNormalized(_ context.Context, identity NormalizedIdentity) (ResolveRecord, error) {
	return store.lookups[normalizedTestKey(identity.Kind, identity.Scope, identity.NormalizedValue)], nil
}

func (store *ingestTestStore) ReserveIngestReceipt(_ context.Context, keyDigest, payloadHMAC []byte) (IngestReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := hex.EncodeToString(keyDigest)
	if receipt, found := store.receipts[key]; found {
		return receipt, nil
	}
	store.nextID++
	store.reservation[store.nextID] = struct {
		key     string
		payload []byte
	}{key: key, payload: append([]byte(nil), payloadHMAC...)}
	return IngestReceipt{ID: store.nextID}, nil
}

func (store *ingestTestStore) InsertPendingIngest(_ context.Context, pending PendingIngest) (int64, error) {
	store.pendingCalls++
	store.lastPending = pending
	store.nextID++
	return store.nextID, nil
}

func (store *ingestTestStore) CompleteIngestReceipt(_ context.Context, receipt IngestReceipt, result identityport.IngestResult) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	reservation := store.reservation[receipt.ID]
	store.receipts[reservation.key] = IngestReceipt{Found: true, PayloadHMAC: reservation.payload, Result: result}
	delete(store.reservation, receipt.ID)
	return nil
}

type ingestTestContacts struct {
	eventID contactport.EventID
	calls   int
	last    contactport.ExternalEventCommand
}

func (*ingestTestContacts) CreateForIdentity(context.Context, contactport.CreateForIdentityCommand) (contactport.CustomerID, error) {
	return 0, errors.New("not implemented")
}
func (*ingestTestContacts) MergeCustomers(context.Context, contactport.MergeCustomersCommand) error {
	return errors.New("not implemented")
}
func (contacts *ingestTestContacts) AppendExternalEvent(_ context.Context, command contactport.ExternalEventCommand) (contactport.EventID, error) {
	contacts.calls++
	contacts.last = command
	return contacts.eventID, nil
}

type ingestTestEvents struct{ events []eventport.Event }

func (events *ingestTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	events.events = append(events.events, event)
	return eventport.EventID(len(events.events)), nil
}
func (events *ingestTestEvents) countType(eventType string) int {
	count := 0
	for _, event := range events.events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func normalizedTestKey(kind identityport.IDKind, scope, value string) string {
	return string(kind) + "\x00" + scope + "\x00" + value
}
