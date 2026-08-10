package p2s06_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestBusinessStateAndEventCommitOrRollbackTogether(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)

	uow := platformstore.NewUnitOfWork(fixture.Pool())
	appender := eventstore.NewAppender()
	occurredAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	event := eventport.Event{
		Type: "stage.changed", CustomerID: 42, Payload: []byte(`{"stage_id":7}`),
		OccurredAt: occurredAt, IdempotencyKey: "stage.changed:42:7",
	}

	var committedID eventport.EventID
	var expiredTxCtx context.Context
	if err := uow.Within(ctx, func(txCtx context.Context) error {
		expiredTxCtx = txCtx
		if err := useFixtureSearchPath(txCtx); err != nil {
			return err
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `INSERT INTO acceptance_fixtures.business_state (id, value) VALUES (1, 'committed')`); err != nil {
			return err
		}
		committedID, err = appender.Append(txCtx, event)
		return err
	}); err != nil {
		t.Fatalf("committed Within() error = %v", err)
	}
	if committedID <= 0 {
		t.Fatalf("committed event ID = %d, want positive", committedID)
	}
	if _, err := appender.Append(expiredTxCtx, event); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("Append() after callback error = %v, want ErrTransactionRequired", err)
	}

	if err := uow.Within(ctx, func(txCtx context.Context) error {
		if err := useFixtureSearchPath(txCtx); err != nil {
			return err
		}
		id, err := appender.Append(txCtx, event)
		if err != nil {
			return err
		}
		if id != committedID {
			t.Fatalf("idempotent event ID = %d, want %d", id, committedID)
		}
		return nil
	}); err != nil {
		t.Fatalf("idempotent Within() error = %v", err)
	}

	conflict := event
	conflict.Payload = []byte(`{"stage_id":8}`)
	if err := uow.Within(ctx, func(txCtx context.Context) error {
		if err := useFixtureSearchPath(txCtx); err != nil {
			return err
		}
		_, err := appender.Append(txCtx, conflict)
		return err
	}); !errors.Is(err, eventport.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Within() error = %v, want ErrIdempotencyConflict", err)
	}

	sentinel := errors.New("force state and event rollback")
	rolledBackEvent := event
	rolledBackEvent.IdempotencyKey = "stage.changed:42:8"
	if err := uow.Within(ctx, func(txCtx context.Context) error {
		if err := useFixtureSearchPath(txCtx); err != nil {
			return err
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `INSERT INTO acceptance_fixtures.business_state (id, value) VALUES (2, 'rolled back')`); err != nil {
			return err
		}
		if _, err = appender.Append(txCtx, rolledBackEvent); err != nil {
			return err
		}
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("rolled back Within() error = %v, want sentinel", err)
	}

	var stateCount, eventCount int
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.business_state`).Scan(&stateCount); err != nil {
		t.Fatalf("count business state: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.event_log`).Scan(&eventCount); err != nil {
		t.Fatalf("count event log: %v", err)
	}
	if stateCount != 1 || eventCount != 1 {
		t.Fatalf("persisted state/events = %d/%d, want 1/1", stateCount, eventCount)
	}
}

func openFixture(t *testing.T) (*acceptancefixtures.PostgreSQL, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgreSQL() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})
	return fixture, ctx
}

func createTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	ddl := `
CREATE TABLE acceptance_fixtures.business_state (
  id bigint PRIMARY KEY,
  value text NOT NULL
);
CREATE TABLE acceptance_fixtures.event_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type text NOT NULL,
  customer_id bigint,
  payload jsonb NOT NULL DEFAULT '{}',
  occurred_at timestamptz NOT NULL DEFAULT now(),
  idempotency_key text NOT NULL UNIQUE,
  dispatched boolean NOT NULL DEFAULT false
);`
	if _, err := fixture.Pool().Exec(ctx, ddl); err != nil {
		t.Fatalf("create acceptance tables: %v", err)
	}
}

func useFixtureSearchPath(ctx context.Context) error {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SET LOCAL search_path TO acceptance_fixtures, public`)
	return err
}
