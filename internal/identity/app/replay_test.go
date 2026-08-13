package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestPendingReplayCompletesUniqueAttributionWithOriginalFact(t *testing.T) {
	identity := NormalizedIdentity{
		Kind: identityport.KindPhone, Scope: "phone:e164", NormalizedValue: "+8613800138000", NormalizerVersion: 1,
	}
	ingestStore := newIngestTestStore()
	ingestStore.lookups[normalizedTestKey(identity.Kind, identity.Scope, identity.NormalizedValue)] = ResolveRecord{CustomerID: 41}
	contacts := &ingestTestContacts{eventID: 73}
	events := &ingestTestEvents{}
	pendingStore := &replayTestStore{pending: validPendingReplayForTest(identity), found: true}
	ingest := NewIngestService(ingestTestUoW{}, ingestStore, contacts, events, ingestTestReceiptKey)
	result, err := NewPendingReplayService(ingestTestUoW{}, pendingStore, ingest).ReplayOnce(context.Background())

	if err != nil || result.Status != PendingReplayCompleted || result.PendingEventID != 91 || result.CustomerID != 41 || result.EventID != 73 {
		t.Fatalf("ReplayOnce()=%+v err=%v", result, err)
	}
	if pendingStore.completedID != 91 || pendingStore.completedVersion != 1 || contacts.calls != 1 {
		t.Fatalf("completion id=%d version=%d contact calls=%d", pendingStore.completedID, pendingStore.completedVersion, contacts.calls)
	}
	if contacts.last.EventType != pendingStore.pending.EventType || string(contacts.last.Payload) != string(pendingStore.pending.Payload) ||
		contacts.last.Actor != "survey.callback" || contacts.last.IdempotencyKey != "survey:event:91" ||
		!contacts.last.OccurredAt.Equal(pendingStore.pending.OccurredAt) {
		t.Fatalf("timeline command=%+v", contacts.last)
	}
	if events.countType(pendingStore.pending.EventType) != 1 || events.events[0].IdempotencyKey != "identity.pending.replay:91" {
		t.Fatalf("events=%+v", events.events)
	}
}

func TestPendingReplayWithoutUniqueRootRemainsRetryable(t *testing.T) {
	identity := NormalizedIdentity{
		Kind: identityport.KindExtension, Scope: "ext:survey", NormalizedValue: "record-1", NormalizerVersion: 1,
	}
	pendingStore := &replayTestStore{pending: validPendingReplayForTest(identity), found: true}
	contacts := &ingestTestContacts{eventID: 73}
	events := &ingestTestEvents{}
	ingest := NewIngestService(ingestTestUoW{}, newIngestTestStore(), contacts, events, ingestTestReceiptKey)
	result, err := NewPendingReplayService(ingestTestUoW{}, pendingStore, ingest).ReplayOnce(context.Background())

	if err != nil || result.Status != PendingReplayRetryable || result.PendingEventID != 91 {
		t.Fatalf("ReplayOnce()=%+v err=%v", result, err)
	}
	if pendingStore.deferredID != 91 || pendingStore.deferredVersion != 1 || pendingStore.completedID != 0 || contacts.calls != 0 || len(events.events) != 0 {
		t.Fatalf("retryable defer=%d/%d completion=%d contacts=%d events=%+v", pendingStore.deferredID, pendingStore.deferredVersion, pendingStore.completedID, contacts.calls, events.events)
	}
}

func TestPendingReplayCompletionFailureFailsClosed(t *testing.T) {
	identity := NormalizedIdentity{
		Kind: identityport.KindUnionID, Scope: "wechat-open-platform:account-a", NormalizedValue: "union-a", NormalizerVersion: 1,
	}
	ingestStore := newIngestTestStore()
	ingestStore.lookups[normalizedTestKey(identity.Kind, identity.Scope, identity.NormalizedValue)] = ResolveRecord{CustomerID: 41}
	pendingStore := &replayTestStore{pending: validPendingReplayForTest(identity), found: true, completeErr: errors.New("version drift")}
	ingest := NewIngestService(ingestTestUoW{}, ingestStore, &ingestTestContacts{eventID: 73}, &ingestTestEvents{}, ingestTestReceiptKey)
	result, err := NewPendingReplayService(ingestTestUoW{}, pendingStore, ingest).ReplayOnce(context.Background())
	if !errors.Is(err, ErrPendingReplayFailed) || result != (PendingReplayResult{}) {
		t.Fatalf("ReplayOnce()=%+v err=%v", result, err)
	}
}

func TestPendingReplayIdleDoesNotInvokeIngest(t *testing.T) {
	contacts := &ingestTestContacts{eventID: 73}
	ingest := NewIngestService(ingestTestUoW{}, newIngestTestStore(), contacts, &ingestTestEvents{}, ingestTestReceiptKey)
	result, err := NewPendingReplayService(ingestTestUoW{}, &replayTestStore{}, ingest).ReplayOnce(context.Background())
	if err != nil || result.Status != PendingReplayIdle || contacts.calls != 0 {
		t.Fatalf("ReplayOnce()=%+v contacts=%d err=%v", result, contacts.calls, err)
	}
}

func validPendingReplayForTest(identity NormalizedIdentity) PendingReplay {
	return PendingReplay{
		ID: 91, Kind: "attribution", Identities: []PendingReplayIdentity{{ID: 17, Identity: identity}},
		EventType: "survey.submitted", Payload: json.RawMessage(`{"answer":42}`), Source: "survey.callback",
		IdempotencyKey: "survey:event:91", OccurredAt: time.Date(2026, 8, 13, 1, 2, 3, 456789000, time.UTC), Version: 1,
	}
}

type replayTestStore struct {
	pending          PendingReplay
	found            bool
	claimErr         error
	completeErr      error
	deferErr         error
	deferredID       int64
	deferredVersion  int64
	completedID      int64
	completedVersion int64
}

func (store *replayTestStore) ClaimPendingReplay(context.Context) (PendingReplay, bool, error) {
	return store.pending, store.found, store.claimErr
}

func (store *replayTestStore) DeferPendingReplay(_ context.Context, id, version int64) error {
	store.deferredID = id
	store.deferredVersion = version
	return store.deferErr
}

func (store *replayTestStore) CompletePendingReplay(_ context.Context, id, version int64) error {
	store.completedID = id
	store.completedVersion = version
	return store.completeErr
}
