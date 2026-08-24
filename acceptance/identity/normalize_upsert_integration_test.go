package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestIdentityNormalizeUpsertScopeValueAndCreatedEvent(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newIdentityUpsertService(pool, recorder)

	first, err := service.Upsert(context.Background(), identityport.IDRef{
		Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 ",
	})
	if err != nil || !first.Created || first.IdentityID < 1 {
		t.Fatalf("first upsert=%+v err=%v", first, err)
	}
	second, err := service.Upsert(context.Background(), identityport.IDRef{
		Kind: identityport.KindPhone, Scope: "phone:e164", Value: "+86 138 0013 8000",
	})
	if err != nil || second.Created || second.IdentityID != first.IdentityID {
		t.Fatalf("second upsert=%+v err=%v; want existing identity %d", second, err, first.IdentityID)
	}

	var kind, scope, normalized string
	var version int16
	var customerID *int64
	if err = pool.QueryRow(context.Background(), `
SELECT kind, scope, normalized_value, normalizer_version, customer_id
FROM identities WHERE id = $1::bigint`, first.IdentityID).Scan(&kind, &scope, &normalized, &version, &customerID); err != nil {
		t.Fatal(err)
	}
	if kind != "phone" || scope != "phone:e164" || normalized != "+8613800138000" || version != identityapp.NormalizerVersion || customerID != nil {
		t.Fatalf("stored identity kind=%q scope=%q value=%q version=%d customer=%v", kind, scope, normalized, version, customerID)
	}

	events := recorder.Events()
	if len(events) != 1 || events[0].Type != "identity.created" || events[0].CustomerID != 0 || events[0].IdempotencyKey != "identity.created:"+itoa(first.IdentityID) {
		t.Fatalf("identity created facts=%+v", events)
	}
	var payload struct {
		IdentityID        int64  `json:"identity_id"`
		Kind              string `json:"kind"`
		Scope             string `json:"scope"`
		NormalizerVersion int16  `json:"normalizer_version"`
	}
	if err = json.Unmarshal(events[0].Payload, &payload); err != nil || payload.IdentityID != first.IdentityID || payload.Kind != kind || payload.Scope != scope || payload.NormalizerVersion != version {
		t.Fatalf("identity created payload=%s decoded=%+v err=%v", events[0].Payload, payload, err)
	}
}

func TestIdentityNormalizeUpsertConcurrentSameValueReturnsOneIdentity(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	secondPool, err := pgxpool.New(context.Background(), *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	firstRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	secondRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	services := []*identityapp.UpsertService{newIdentityUpsertService(pool, firstRecorder), newIdentityUpsertService(secondPool, secondRecorder)}
	ref := identityport.IDRef{Kind: identityport.KindUnionID, Scope: "wechat-open-platform:account-a", Value: " union-race "}
	start := make(chan struct{})
	results := make([]identityapp.UpsertResult, len(services))
	errs := make([]error, len(services))
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *identityapp.UpsertService) {
			defer wait.Done()
			<-start
			results[index], errs[index] = service.Upsert(context.Background(), ref)
		}(index, service)
	}
	close(start)
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].IdentityID < 1 || results[0].IdentityID != results[1].IdentityID || results[0].Created == results[1].Created {
		t.Fatalf("concurrent results=%+v %+v", results[0], results[1])
	}
	var identities int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM identities WHERE kind = 'unionid' AND scope = 'wechat-open-platform:account-a' AND normalized_value = 'union-race'`).Scan(&identities); err != nil || identities != 1 {
		t.Fatalf("identity count=%d err=%v", identities, err)
	}
	if len(firstRecorder.Events())+len(secondRecorder.Events()) != 1 {
		t.Fatalf("created event count=%d", len(firstRecorder.Events())+len(secondRecorder.Events()))
	}
}

func TestIdentityNormalizeUpsertRollsBackWhenCreatedEventCannotAppend(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	service := identityapp.NewUpsertService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), failingEventAppender{})
	_, err := service.Upsert(context.Background(), identityport.IDRef{Kind: identityport.KindExtension, Scope: "ext:partner-a", Value: "record-a"})
	if err == nil {
		t.Fatal("upsert succeeded while event append failed")
	}
	var identities int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identities`).Scan(&identities); err != nil || identities != 0 {
		t.Fatalf("identity rollback count=%d err=%v", identities, err)
	}
}

func newIdentityUpsertService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.UpsertService {
	return identityapp.NewUpsertService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), events)
}

func resetIdentityUpsert(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE identity_operation_receipts, pending_events, customer_merges, identities CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `SELECT setval('identities_id_seq', $1::bigint, false)`, time.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}
}

type failingEventAppender struct{}

func (failingEventAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("event append failed")
}

type recordingEventAppender struct {
	delegate eventport.Appender
	mu       sync.Mutex
	events   []eventport.Event
}

func (appender *recordingEventAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	id, err := appender.delegate.Append(ctx, event)
	if err != nil {
		return 0, err
	}
	appender.mu.Lock()
	defer appender.mu.Unlock()
	appender.events = append(appender.events, event)
	return id, nil
}

func (appender *recordingEventAppender) Events() []eventport.Event {
	appender.mu.Lock()
	defer appender.mu.Unlock()
	return append([]eventport.Event(nil), appender.events...)
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
