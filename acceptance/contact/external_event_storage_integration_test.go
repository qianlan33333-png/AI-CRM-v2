package contact_test

import (
	"context"
	"errors"
	"flag"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var externalEventStorageDatabaseURL = flag.String("external-event-storage-database-url", "", "isolated PostgreSQL 16.14 Contact external-event storage database")

func TestExternalEventStorageCatalogAndConstraints(t *testing.T) {
	pool := openExternalEventStoragePool(t)
	resetExternalEventStorage(t, pool)
	for _, want := range []string{"customer_event_idempotency_actor", "customer_event_idempotency_event_fk", "customer_event_idempotency_event_unique", "customer_event_idempotency_key", "customer_event_idempotency_payload", "customer_event_idempotency_type"} {
		if !containsExternalEventStorageString(externalEventStorageStrings(t, pool, `SELECT constraint_name FROM information_schema.table_constraints WHERE table_schema = 'public' AND table_name = 'customer_event_idempotency'`), want) {
			t.Fatalf("registry constraint %q is missing", want)
		}
	}
	for _, want := range []string{"actor", "event_customer_id", "event_id", "event_occurred_at", "event_type", "idempotency_key", "payload"} {
		if !containsExternalEventStorageString(externalEventStorageStrings(t, pool, `SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'customer_event_idempotency'`), want) {
			t.Fatalf("registry column %q is missing", want)
		}
	}
}

func TestExternalEventStorageFactsAndFailures(t *testing.T) {
	pool := openExternalEventStoragePool(t)
	resetExternalEventStorage(t, pool)
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
	var occurredAt time.Time
	var eventID int64
	err = tx.QueryRow(ctx, `INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at) VALUES ($1, 'external.event', '{"source":"acceptance"}'::jsonb, 'acceptance', now()) RETURNING occurred_at, id`, customerID).Scan(&occurredAt, &eventID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO customer_event_idempotency (idempotency_key, event_occurred_at, event_id, event_customer_id, event_type, payload, actor) VALUES ('acceptance-key', $1, $2, $3, 'external.event', '{"source":"acceptance"}'::jsonb, 'acceptance')`, occurredAt, eventID, customerID)
	if err != nil {
		t.Fatal(err)
	}
	var secondOccurredAt time.Time
	var secondEventID int64
	err = tx.QueryRow(ctx, `INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at) VALUES ($1, 'external.event', '{"source":"acceptance","sequence":2}'::jsonb, 'acceptance', now()) RETURNING occurred_at, id`, customerID).Scan(&secondOccurredAt, &secondEventID)
	if err != nil {
		t.Fatal(err)
	}
	assertExternalEventStorageFailure(t, tx, `INSERT INTO customer_event_idempotency (idempotency_key, event_occurred_at, event_id, event_customer_id, event_type, payload, actor) VALUES ('duplicate-event', $1, $2, $3, 'external.event', '{}'::jsonb, 'acceptance')`, "23505", occurredAt, eventID, customerID)
	assertExternalEventStorageFailure(t, tx, `INSERT INTO customer_event_idempotency (idempotency_key, event_occurred_at, event_id, event_customer_id, event_type, payload, actor) VALUES ('bad-payload', $1, $2, $3, 'external.event', '[]'::jsonb, 'acceptance')`, "23514", occurredAt, eventID, customerID)
	assertExternalEventStorageFailure(t, tx, `INSERT INTO customer_event_idempotency (idempotency_key, event_occurred_at, event_id, event_customer_id, event_type, payload, actor) VALUES ('', $1, $2, $3, 'external.event', '{}'::jsonb, 'acceptance')`, "23514", secondOccurredAt, secondEventID, customerID)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func openExternalEventStoragePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *externalEventStorageDatabaseURL == "" {
		t.Skip("external-event-storage-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*externalEventStorageDatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *externalEventStorageDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetExternalEventStorage(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE customer_event_idempotency, customer_events, customers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func externalEventStorageStrings(t *testing.T, pool *pgxpool.Pool, statement string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), statement)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(values)
	return values
}

func containsExternalEventStorageString(values []string, want string) bool {
	index := sort.SearchStrings(values, want)
	return index < len(values) && values[index] == want
}

func assertExternalEventStorageFailure(t *testing.T, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, statement, code string, arguments ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), `SAVEPOINT external_event_storage_failure`); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(context.Background(), statement, arguments...)
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("PostgreSQL error=%v, want SQLSTATE %s", err, code)
	}
	if _, err = tx.Exec(context.Background(), `ROLLBACK TO SAVEPOINT external_event_storage_failure`); err != nil {
		t.Fatal(err)
	}
}
