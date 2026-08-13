package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var bindReceiptKey = []byte("identity-bind-receipt-key-v1-32b")

func TestIdentityBindPersistsReplayAndDivertsOtherCustomer(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	firstCustomerID, secondCustomerID := createBindCustomer(t, pool), createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newIdentityBindService(pool, recorder)
	command := bindCommand(firstCustomerID, "bind-replay-key")

	first, err := service.Bind(ctx, command)
	if err != nil || first != (identityport.BindResult{Status: identityport.BindBound, CustomerID: contactport.CustomerID(firstCustomerID)}) {
		t.Fatalf("first Bind()=%+v err=%v", first, err)
	}
	replay, err := service.Bind(ctx, command)
	if err != nil || replay != first {
		t.Fatalf("replay Bind()=%+v err=%v, want %+v", replay, err, first)
	}
	changed := command
	changed.CustomerID = contactport.CustomerID(secondCustomerID)
	if _, err = service.Bind(ctx, changed); !errors.Is(err, identityapp.ErrIdentityBindIdempotencyConflict) {
		t.Fatalf("same key changed payload error=%v", err)
	}
	sameCustomerDifferentKey := command
	sameCustomerDifferentKey.IdempotencyKey = "bind-state-replay-key"
	alreadyBound, err := service.Bind(ctx, sameCustomerDifferentKey)
	if err != nil || alreadyBound != (identityport.BindResult{Status: identityport.BindAlreadyBound, CustomerID: contactport.CustomerID(firstCustomerID)}) {
		t.Fatalf("same customer Bind()=%+v err=%v", alreadyBound, err)
	}
	otherCustomer := changed
	otherCustomer.IdempotencyKey = "bind-other-customer-key"
	rejected, err := service.Bind(ctx, otherCustomer)
	if err != nil || rejected != (identityport.BindResult{Status: identityport.BindRejected}) {
		t.Fatalf("other customer Bind()=%+v err=%v", rejected, err)
	}

	var boundCustomerID int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM identities WHERE kind = 'phone' AND scope = 'phone:e164'`).Scan(&boundCustomerID); err != nil || boundCustomerID != firstCustomerID {
		t.Fatalf("identity customer_id=%d err=%v, want %d", boundCustomerID, err, firstCustomerID)
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind' AND state = 'completed'`).Scan(&receipts); err != nil || receipts != 3 {
		t.Fatalf("completed bind receipts=%d err=%v, want 3", receipts, err)
	}
	events := recorder.Events()
	if len(events) != 1 || events[0].Type != "identity.bound" || events[0].CustomerID != eventport.CustomerID(firstCustomerID) || events[0].IdempotencyKey == "" {
		t.Fatalf("bound events=%+v", events)
	}
}

func TestIdentityBindRollsBackReceiptAndEdgeWhenEventFails(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	customerID := createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	command := bindCommand(customerID, "bind-event-failure-key")

	_, err := newIdentityBindService(pool, failingEventAppender{}).Bind(context.Background(), command)
	if err == nil {
		t.Fatal("Bind succeeded while event append failed")
	}
	var customerIDAfter *int64
	if err = pool.QueryRow(context.Background(), `SELECT customer_id FROM identities WHERE kind = 'phone' AND scope = 'phone:e164'`).Scan(&customerIDAfter); err != nil || customerIDAfter != nil {
		t.Fatalf("identity after failed Bind customer=%v err=%v", customerIDAfter, err)
	}
	var receipts int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("receipts after failed Bind=%d err=%v", receipts, err)
	}
}

func TestIdentityBindConcurrentSameKeyReturnsOriginalBoundFactOnce(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	customerID := createBindCustomer(t, pool)
	upsertIdentityForBind(t, pool, bindRef())
	secondPool, err := pgxpool.New(context.Background(), *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	firstRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	secondRecorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	services := []*identityapp.BindService{newIdentityBindService(pool, firstRecorder), newIdentityBindService(secondPool, secondRecorder)}
	command := bindCommand(customerID, "bind-concurrent-key")
	start := make(chan struct{})
	results := make([]identityport.BindResult, len(services))
	errs := make([]error, len(services))
	var wait sync.WaitGroup
	for index, service := range services {
		wait.Add(1)
		go func(index int, service *identityapp.BindService) {
			defer wait.Done()
			<-start
			results[index], errs[index] = service.Bind(context.Background(), command)
		}(index, service)
	}
	close(start)
	wait.Wait()
	want := identityport.BindResult{Status: identityport.BindBound, CustomerID: contactport.CustomerID(customerID)}
	for index := range results {
		if errs[index] != nil || results[index] != want {
			t.Fatalf("concurrent Bind[%d]=%+v err=%v, want %+v", index, results[index], errs[index], want)
		}
	}
	if len(firstRecorder.Events())+len(secondRecorder.Events()) != 1 {
		t.Fatalf("bound events=%d, want 1", len(firstRecorder.Events())+len(secondRecorder.Events()))
	}
	var receipts, boundEdges int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity_operation_receipts WHERE operation = 'bind'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("bind receipts=%d err=%v", receipts, err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM identities WHERE customer_id = $1::bigint`, customerID).Scan(&boundEdges); err != nil || boundEdges != 1 {
		t.Fatalf("bound edges=%d err=%v", boundEdges, err)
	}
}

func newIdentityBindService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.BindService {
	return identityapp.NewBindService(platformstore.NewUnitOfWork(pool), identitystore.NewRepository(), events, bindReceiptKey)
}

func bindRef() identityport.IDRef {
	return identityport.IDRef{Kind: identityport.KindPhone, Scope: "phone:e164", Value: " +86 (138) 0013-8000 ", Assurance: identityport.AssuranceDeclared, Source: "admin"}
}

func bindCommand(customerID int64, idempotencyKey string) identityport.BindCommand {
	return identityport.BindCommand{CustomerID: contactport.CustomerID(customerID), Ref: bindRef(), Actor: "acceptance:identity-bind", IdempotencyKey: idempotencyKey}
}

func upsertIdentityForBind(t *testing.T, pool *pgxpool.Pool, ref identityport.IDRef) {
	t.Helper()
	result, err := newIdentityUpsertService(pool, eventstore.NewAppender()).Upsert(context.Background(), ref)
	if err != nil || !result.Created || result.IdentityID <= 0 {
		t.Fatalf("upsert identity=%+v err=%v", result, err)
	}
}

func createBindCustomer(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID
}
