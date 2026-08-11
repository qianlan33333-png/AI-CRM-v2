package contact_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16 acceptance database")

func TestCustomerEventsUseMonthlyPartitionsAndPrebuildThreeFutureMonths(t *testing.T) {
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe test database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer pool.Close()

	var partitioned bool
	if err = pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_partitioned_table
  WHERE partrelid = 'public.customer_events'::regclass
)`).Scan(&partitioned); err != nil || !partitioned {
		t.Fatalf("customer_events partitioned=%t err=%v", partitioned, err)
	}

	now := time.Now().UTC()
	for monthOffset := 0; monthOffset <= 3; monthOffset++ {
		month := now.AddDate(0, monthOffset, 0)
		assertPartitionHasIndexes(t, ctx, pool, fmt.Sprintf("customer_events_%04d_%02d", month.Year(), month.Month()))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var customerID int64
	if err = tx.QueryRow(ctx, `INSERT INTO customers (name) VALUES ('partition acceptance') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	currentMonth := monthStartUTC(now)
	currentEventAt := currentMonth.Add(time.Hour)
	for _, invalidInsert := range []struct {
		name      string
		statement string
	}{
		{
			name:      "blank event type",
			statement: `INSERT INTO customer_events (customer_id, event_type, actor, occurred_at) VALUES ($1, '   ', 'acceptance', $2)`,
		},
		{
			name:      "non-object payload",
			statement: `INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at) VALUES ($1, 'acceptance.invalid', '[]', 'acceptance', $2)`,
		},
		{
			name:      "blank actor",
			statement: `INSERT INTO customer_events (customer_id, event_type, actor, occurred_at) VALUES ($1, 'acceptance.invalid', '   ', $2)`,
		},
	} {
		t.Run(invalidInsert.name, func(t *testing.T) {
			assertInsertConstraintRejected(t, ctx, tx, customerID, currentEventAt, invalidInsert.statement)
		})
	}
	currentEventID := assertEventRoutesToPartition(t, ctx, tx, customerID, currentEventAt)
	assertMutationRejected(t, ctx, tx, currentEventAt, currentEventID, "UPDATE customer_events SET actor = 'changed' WHERE occurred_at = $1 AND id = $2")
	assertMutationRejected(t, ctx, tx, currentEventAt, currentEventID, "DELETE FROM customer_events WHERE occurred_at = $1 AND id = $2")
	for _, occurredAt := range []time.Time{currentMonth.AddDate(0, 3, 0).Add(time.Hour)} {
		assertEventRoutesToPartition(t, ctx, tx, customerID, occurredAt)
	}

	outOfHorizon := currentMonth.AddDate(0, 4, 0).Add(time.Hour)
	if _, err = tx.Exec(ctx, `SAVEPOINT before_unrouted_event`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
VALUES ($1, 'acceptance.unrouted', '{}', 'acceptance', $2)`, customerID, outOfHorizon)
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "23514" {
		t.Fatalf("out-of-horizon insert error = %v, want SQLSTATE 23514", err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_unrouted_event`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT public.aicrm_ensure_customer_event_partitions($1, 3)`, outOfHorizon); err != nil {
		t.Fatalf("extend event partitions: %v", err)
	}
	assertEventRoutesToPartition(t, ctx, tx, customerID, outOfHorizon)

	if _, err = tx.Exec(ctx, `SAVEPOINT before_invalid_horizon`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `SELECT public.aicrm_ensure_customer_event_partitions(now(), 37)`)
	if !errors.As(err, &pgError) || pgError.Code != "22023" {
		t.Fatalf("invalid horizon error = %v, want SQLSTATE 22023", err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_invalid_horizon`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT before_null_horizon`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `SELECT public.aicrm_ensure_customer_event_partitions(now(), NULL)`)
	if !errors.As(err, &pgError) || pgError.Code != "22023" {
		t.Fatalf("null horizon error = %v, want SQLSTATE 22023", err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_null_horizon`); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentPartitionMaintenanceIsIdempotent(t *testing.T) {
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe test database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer pool.Close()

	anchor := monthStartUTC(time.Now().UTC()).AddDate(1, 0, 0)
	errorsByCall := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range errorsByCall {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, errorsByCall[index] = pool.Exec(ctx, `SELECT public.aicrm_ensure_customer_event_partitions($1, 3)`, anchor)
		}()
	}
	close(start)
	wait.Wait()
	for index, callError := range errorsByCall {
		if callError != nil {
			t.Fatalf("concurrent maintenance call %d: %v", index, callError)
		}
	}
	for monthOffset := 0; monthOffset <= 3; monthOffset++ {
		month := anchor.AddDate(0, monthOffset, 0)
		assertPartitionHasIndexes(t, ctx, pool, fmt.Sprintf("customer_events_%04d_%02d", month.Year(), month.Month()))
	}
}

func assertPartitionHasIndexes(t *testing.T, ctx context.Context, pool *pgxpool.Pool, partition string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, partition).Scan(&exists); err != nil || !exists {
		t.Fatalf("partition %s exists=%t err=%v", partition, exists, err)
	}
	rows, err := pool.Query(ctx, `
SELECT index_definition.indisprimary,
       index_definition.access_method,
       array_agg(pg_get_indexdef(index_definition.indexrelid, key_position, true) ORDER BY key_position),
       array_agg((index_definition.indoption[key_position - 1] & 1) = 1 ORDER BY key_position),
       index_definition.reloptions
FROM (
  SELECT pg_index.indexrelid,
         pg_index.indisprimary,
         pg_index.indnkeyatts,
         pg_index.indoption,
         pg_am.amname AS access_method,
         COALESCE(index_relation.reloptions, '{}'::text[]) AS reloptions
  FROM pg_index
  JOIN pg_class AS index_relation ON index_relation.oid = pg_index.indexrelid
  JOIN pg_am ON pg_am.oid = index_relation.relam
  WHERE pg_index.indrelid = ('public.' || $1)::regclass
) AS index_definition
CROSS JOIN LATERAL generate_series(1, index_definition.indnkeyatts) AS key(key_position)
GROUP BY index_definition.indexrelid,
         index_definition.indisprimary,
         index_definition.access_method,
         index_definition.reloptions`, partition)
	if err != nil {
		t.Fatalf("query partition %s indexes: %v", partition, err)
	}
	defer rows.Close()
	var foundPrimary, foundTimeline, foundBRIN bool
	for rows.Next() {
		var primary bool
		var accessMethod string
		var keyExpressions, options []string
		var descending []bool
		if err = rows.Scan(&primary, &accessMethod, &keyExpressions, &descending, &options); err != nil {
			t.Fatalf("scan partition %s index: %v", partition, err)
		}
		switch {
		case primary && accessMethod == "btree" && slices.Equal(keyExpressions, []string{"occurred_at", "id"}) && slices.Equal(descending, []bool{false, false}):
			foundPrimary = true
		case !primary && accessMethod == "btree" && slices.Equal(keyExpressions, []string{"customer_id", "occurred_at", "id"}) && slices.Equal(descending, []bool{false, true, true}):
			foundTimeline = true
		case !primary && accessMethod == "brin" && slices.Equal(keyExpressions, []string{"occurred_at"}) && slices.Equal(descending, []bool{false}) && slices.Contains(options, "pages_per_range=32"):
			foundBRIN = true
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate partition %s indexes: %v", partition, err)
	}
	if !foundPrimary || !foundTimeline || !foundBRIN {
		t.Fatalf(
			"partition %s index shapes primary=%t timeline=%t brin=%t",
			partition,
			foundPrimary,
			foundTimeline,
			foundBRIN,
		)
	}
}

func assertEventRoutesToPartition(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	customerID int64,
	occurredAt time.Time,
) int64 {
	t.Helper()
	var partition string
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
VALUES ($1, 'acceptance.routed', '{"source":"p3c03"}', 'acceptance', $2)
RETURNING tableoid::regclass::text, id`, customerID, occurredAt).Scan(&partition, &eventID); err != nil {
		t.Fatalf("insert routed event at %s: %v", occurredAt, err)
	}
	want := fmt.Sprintf("customer_events_%04d_%02d", occurredAt.UTC().Year(), occurredAt.UTC().Month())
	if partition != want {
		t.Fatalf("event partition = %q, want %q", partition, want)
	}
	return eventID
}

func assertMutationRejected(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	occurredAt time.Time,
	eventID int64,
	statement string,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SAVEPOINT before_forbidden_mutation`); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, statement, occurredAt, eventID)
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "55000" {
		t.Fatalf("customer event mutation error = %v, want SQLSTATE 55000", err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_forbidden_mutation`); err != nil {
		t.Fatal(err)
	}
}

func assertInsertConstraintRejected(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	customerID int64,
	occurredAt time.Time,
	statement string,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SAVEPOINT before_invalid_event`); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, statement, customerID, occurredAt)
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "23514" {
		t.Fatalf("invalid customer event error = %v, want SQLSTATE 23514", err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_invalid_event`); err != nil {
		t.Fatal(err)
	}
}

func monthStartUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}
