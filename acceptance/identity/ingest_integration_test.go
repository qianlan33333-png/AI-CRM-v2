package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var ingestReceiptKey = []byte("identity-ingest-acceptance-key-32b")

func TestIdentityIngestAttributesTimelineAndEventLogThenReplaysFact(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	customerID := createBindCustomer(t, pool)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ref := identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+86139" + suffix[len(suffix)-8:], Assurance: identityport.AssuranceVerified, Source: "acceptance.ingest"}
	seedBoundIngestIdentity(t, pool, customerID, ref)
	command := ingestCommand(ref, "identity.ingest.attributed."+suffix, "ingest-attributed-"+suffix)
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newIdentityIngestService(pool, recorder)

	first, err := service.Ingest(ctx, command)
	if err != nil || first.Status != identityport.IngestAttributed || first.CustomerID != contactport.CustomerID(customerID) || first.EventID <= 0 || first.PendingEventID != 0 {
		t.Fatalf("first Ingest()=%+v err=%v", first, err)
	}
	replayCommand := command
	replayCommand.Refs = append([]identityport.IDRef(nil), command.Refs...)
	replayCommand.Refs[0].Value = " +86 " + command.Refs[0].Value[3:6] + " " + command.Refs[0].Value[6:] + " "
	replayCommand.Payload = json.RawMessage(`{"nested":{"exact":9007199254740993.0},"answer":42e0}`)
	replay, err := service.Ingest(ctx, replayCommand)
	if err != nil || replay != first {
		t.Fatalf("replay Ingest()=%+v err=%v, want %+v", replay, err, first)
	}

	var timelineCount, receiptCount, pendingCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_events WHERE id=$1::bigint AND customer_id=$2::bigint AND event_type=$3::text`, first.EventID, customerID, command.EventType).Scan(&timelineCount); err != nil || timelineCount != 1 {
		t.Fatalf("timeline count=%d err=%v", timelineCount, err)
	}
	if domainEventCount := countIngestRecordedEvents(recorder.Events(), command.EventType); domainEventCount != 1 {
		t.Fatalf("recorded domain event count=%d", domainEventCount)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='ingest' AND state='completed' AND result_status='attributed' AND result_customer_id=$1::bigint AND result_event_id=$2::bigint`, customerID, first.EventID).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("receipt count=%d err=%v", receiptCount, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM pending_events WHERE event_type=$1::text`, command.EventType).Scan(&pendingCount); err != nil || pendingCount != 0 {
		t.Fatalf("pending count=%d err=%v", pendingCount, err)
	}

	changed := command
	changed.Payload = json.RawMessage(`{"answer":43,"nested":{"exact":9007199254740993}}`)
	if _, err = service.Ingest(ctx, changed); !errors.Is(err, identityapp.ErrIdentityIngestIdempotencyConflict) {
		t.Fatalf("changed payload error=%v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_events WHERE event_type=$1::text`, command.EventType).Scan(&timelineCount); err != nil || timelineCount != 1 {
		t.Fatalf("changed payload timeline count=%d err=%v", timelineCount, err)
	}
}

func TestIdentityIngestDurablyPreservesPendingAndConflictPayloads(t *testing.T) {
	for _, test := range []struct {
		name   string
		status identityport.IngestStatus
		roots  int
	}{
		{name: "zero match pending", status: identityport.IngestPending},
		{name: "multiple roots conflict", status: identityport.IngestConflict, roots: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := openIdentityPool(t)
			resetIdentityUpsert(t, pool)
			ctx := context.Background()
			suffix := fmt.Sprintf("%d", time.Now().UnixNano())
			refs := []identityport.IDRef{
				{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:ingest-" + suffix, Value: "union-a", Assurance: identityport.AssuranceVerified, Source: "acceptance.ingest"},
			}
			if test.roots == 2 {
				refs = append(refs, identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:ingest-" + suffix, Value: "record-b", Assurance: identityport.AssuranceVerified, Source: "acceptance.ingest"})
				seedBoundIngestIdentity(t, pool, createBindCustomer(t, pool), refs[0])
				seedBoundIngestIdentity(t, pool, createBindCustomer(t, pool), refs[1])
			}
			command := identityport.IngestCommand{
				Refs: refs, EventType: "identity.ingest." + string(test.status) + "." + suffix,
				Payload: json.RawMessage(`{"answer":42,"nested":{"exact":9007199254740993}}`), Source: "acceptance.ingest",
				OccurredAt: time.Date(2026, 8, 13, 6, 7, 8, 987654321, time.FixedZone("CST", 8*60*60)), IdempotencyKey: "ingest-" + string(test.status) + "-" + suffix,
			}
			recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
			service := newIdentityIngestService(pool, recorder)
			first, err := service.Ingest(ctx, command)
			if err != nil || first.Status != test.status || first.PendingEventID <= 0 || first.CustomerID != 0 || first.EventID != 0 {
				t.Fatalf("Ingest()=%+v err=%v", first, err)
			}
			replay := command
			replay.Refs = append([]identityport.IDRef(nil), command.Refs...)
			for left, right := 0, len(replay.Refs)-1; left < right; left, right = left+1, right-1 {
				replay.Refs[left], replay.Refs[right] = replay.Refs[right], replay.Refs[left]
			}
			replay.Payload = json.RawMessage(`{"nested":{"exact":9007199254740993.0},"answer":42.0}`)
			replayed, err := service.Ingest(ctx, replay)
			if err != nil || replayed != first {
				t.Fatalf("replay=%+v err=%v, want %+v", replayed, err, first)
			}

			var kind, state, eventType, source, idempotencyKey string
			var payload json.RawMessage
			var occurredAt time.Time
			var identityIDs, candidates []int64
			var payloadMatches bool
			err = pool.QueryRow(ctx, `
SELECT kind,state,identity_ids,candidate_customer_ids,event_type,payload,source,idempotency_key,occurred_at,payload=$2::jsonb
FROM pending_events WHERE id=$1::bigint`, first.PendingEventID, command.Payload).
				Scan(&kind, &state, &identityIDs, &candidates, &eventType, &payload, &source, &idempotencyKey, &occurredAt, &payloadMatches)
			wantKind := "attribution"
			if test.status == identityport.IngestConflict {
				wantKind = "conflict"
			}
			if err != nil || kind != wantKind || state != "pending" || len(identityIDs) != len(refs) || len(candidates) != 0 || eventType != command.EventType || source != command.Source || idempotencyKey != command.IdempotencyKey || !occurredAt.Equal(command.OccurredAt.UTC().Truncate(time.Microsecond)) || !payloadMatches {
				t.Fatalf("pending fact kind=%q state=%q identities=%v candidates=%v event=%q payload=%s source=%q key=%q occurred=%s err=%v", kind, state, identityIDs, candidates, eventType, payload, source, idempotencyKey, occurredAt, err)
			}
			var receiptCount int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='ingest' AND result_status=$1::text AND result_pending_event_id=$2::bigint`, string(test.status), first.PendingEventID).Scan(&receiptCount); err != nil || receiptCount != 1 {
				t.Fatalf("receipt count=%d err=%v", receiptCount, err)
			}
			if pendingEventCount := countIngestRecordedEvents(recorder.Events(), "identity.ingest."+string(test.status)); pendingEventCount != 1 {
				t.Fatalf("recorded pending event count=%d", pendingEventCount)
			}
		})
	}
}

func TestIdentityIngestAttributedRollbackLeavesNoTimelineReceiptOrEvent(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	customerID := createBindCustomer(t, pool)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ref := identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:rollback-" + suffix, Value: "record", Assurance: identityport.AssuranceVerified, Source: "acceptance.ingest"}
	seedBoundIngestIdentity(t, pool, customerID, ref)
	command := ingestCommand(ref, "identity.ingest.rollback."+suffix, "ingest-rollback-"+suffix)

	if _, err := newIdentityIngestService(pool, failingEventAppender{}).Ingest(ctx, command); err == nil {
		t.Fatal("Ingest succeeded while event_log append failed")
	}
	for table, query := range map[string]string{
		"timeline": `SELECT count(*) FROM customer_events WHERE event_type=$1::text`,
		"receipt":  `SELECT count(*) FROM identity_operation_receipts WHERE operation='ingest' AND $1::text <> ''`,
		"pending":  `SELECT count(*) FROM pending_events WHERE event_type=$1::text`,
	} {
		var count int
		if err := pool.QueryRow(ctx, query, command.EventType).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rollback count=%d err=%v", table, count, err)
		}
	}
}

func TestIdentityIngestConcurrentSameKeyReturnsOneAttributedFact(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	customerID := createBindCustomer(t, pool)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ref := identityport.IDRef{Kind: identityport.KindOAOpenID, Scope: "wechat-app:ingest-" + suffix, Value: "openid", Assurance: identityport.AssuranceVerified, Source: "acceptance.ingest"}
	seedBoundIngestIdentity(t, pool, customerID, ref)
	command := ingestCommand(ref, "identity.ingest.concurrent."+suffix, "ingest-concurrent-"+suffix)
	secondPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	recorders := []*recordingEventAppender{{delegate: eventstore.NewAppender()}, {delegate: eventstore.NewAppender()}}
	services := []*identityapp.IngestService{newIdentityIngestService(pool, recorders[0]), newIdentityIngestService(secondPool, recorders[1])}
	results := make([]identityport.IngestResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range services {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errs[index] = services[index].Ingest(ctx, command)
		}(index)
	}
	close(start)
	wait.Wait()
	if errs[0] != nil || errs[1] != nil || results[0] != results[1] || results[0].Status != identityport.IngestAttributed || results[0].EventID <= 0 {
		t.Fatalf("concurrent results=%+v/%+v errors=%v/%v", results[0], results[1], errs[0], errs[1])
	}
	var timelines, receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_events WHERE event_type=$1::text`, command.EventType).Scan(&timelines); err != nil || timelines != 1 {
		t.Fatalf("timeline count=%d err=%v", timelines, err)
	}
	if events := countIngestRecordedEvents(recorders[0].Events(), command.EventType) + countIngestRecordedEvents(recorders[1].Events(), command.EventType); events != 1 {
		t.Fatalf("recorded event count=%d", events)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation='ingest' AND result_event_id=$1::bigint`, results[0].EventID).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("receipt count=%d err=%v", receipts, err)
	}
}

func newIdentityIngestService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.IngestService {
	return identityapp.NewIngestService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), contactstore.NewMergePortRepository(), events, ingestReceiptKey)
}

func seedBoundIngestIdentity(t *testing.T, pool *pgxpool.Pool, customerID int64, ref identityport.IDRef) {
	t.Helper()
	normalized, err := identityapp.Normalize(ref)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `
INSERT INTO identities (customer_id,kind,scope,normalized_value,normalizer_version,assurance,source,review_fingerprint,fingerprint_key_version,bound_at)
VALUES ($1::bigint,$2::text,$3::text,$4::text,$5::smallint,$6::text,$7::text,decode('00112233445566778899aabbccddeeff','hex'),1,now())`,
		customerID, string(normalized.Kind), normalized.Scope, normalized.NormalizedValue, normalized.NormalizerVersion, string(ref.Assurance), ref.Source)
	if err != nil {
		t.Fatal(err)
	}
}

func ingestCommand(ref identityport.IDRef, eventType, key string) identityport.IngestCommand {
	return identityport.IngestCommand{
		Refs: []identityport.IDRef{ref}, EventType: eventType,
		Payload: json.RawMessage(`{"answer":42,"nested":{"exact":9007199254740993}}`), Source: ref.Source,
		OccurredAt: time.Date(2026, 8, 13, 1, 2, 3, 456789123, time.UTC), IdempotencyKey: key,
	}
}

func countIngestRecordedEvents(events []eventport.Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
