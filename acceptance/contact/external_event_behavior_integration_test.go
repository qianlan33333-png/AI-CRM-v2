package contact_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestExternalEventExactlyOnceConcurrencyConflictAndEffectiveRootReplay(t *testing.T) {
	pool := openExternalEventPool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)
	rootID := createLineageCustomer(t, uow, repository, "external-root")
	command := contactport.ExternalEventCommand{
		CustomerID:     rootID,
		EventType:      "extension.payment_succeeded",
		Payload:        json.RawMessage(`{"amount":99.00,"order_id":"order-1"}`),
		Actor:          "ext:payments",
		OccurredAt:     time.Now().UTC().Truncate(time.Microsecond),
		IdempotencyKey: fmt.Sprintf("p3-c07c-r3c-concurrent-%d", time.Now().UnixNano()),
	}
	if _, err := repository.AppendExternalEvent(context.Background(), command); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("AppendExternalEvent outside UoW error=%v", err)
	}

	const attempts = 10
	start := make(chan struct{})
	results := make([]contactport.EventID, attempts)
	errorsByAttempt := make([]error, attempts)
	var wait sync.WaitGroup
	for index := range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsByAttempt[index] = uow.Within(context.Background(), func(txCtx context.Context) error {
				var err error
				results[index], err = repository.AppendExternalEvent(txCtx, command)
				return err
			})
		}()
	}
	close(start)
	wait.Wait()
	wantEventID := results[0]
	if wantEventID <= 0 {
		t.Fatalf("first event id=%d error=%v", wantEventID, errorsByAttempt[0])
	}
	for index := range attempts {
		if errorsByAttempt[index] != nil || results[index] != wantEventID {
			t.Fatalf("attempt=%d event_id=%d error=%v want=%d", index, results[index], errorsByAttempt[index], wantEventID)
		}
	}
	assertExternalEventFacts(t, pool, command.IdempotencyKey, wantEventID, rootID, 1, 1)

	equivalent := command
	equivalent.Payload = json.RawMessage(`{"order_id":"order-1","amount":99}`)
	if eventID, err := appendExternalEvent(t, uow, repository, equivalent); err != nil || eventID != wantEventID {
		t.Fatalf("JSONB-equivalent replay event_id=%d error=%v", eventID, err)
	}
	otherRootID := createLineageCustomer(t, uow, repository, "external-other-root")
	conflicts := map[string]contactport.ExternalEventCommand{
		"different payload": func() contactport.ExternalEventCommand {
			changed := command
			changed.Payload = json.RawMessage(`{"amount":100,"order_id":"order-1"}`)
			return changed
		}(),
		"different event type": func() contactport.ExternalEventCommand {
			changed := command
			changed.EventType = "extension.payment_refunded"
			return changed
		}(),
		"different actor": func() contactport.ExternalEventCommand {
			changed := command
			changed.Actor = "ext:other"
			return changed
		}(),
		"different time": func() contactport.ExternalEventCommand {
			changed := command
			changed.OccurredAt = changed.OccurredAt.Add(time.Microsecond)
			return changed
		}(),
		"different root": func() contactport.ExternalEventCommand {
			changed := command
			changed.CustomerID = otherRootID
			return changed
		}(),
	}
	for name, conflict := range conflicts {
		t.Run(name, func(t *testing.T) {
			if _, err := appendExternalEvent(t, uow, repository, conflict); !errors.Is(err, contactport.ErrExternalEventConflict) {
				t.Fatalf("conflict error=%v", err)
			}
		})
	}
	assertExternalEventFacts(t, pool, command.IdempotencyKey, wantEventID, rootID, 1, 1)

	finalRootID := createLineageCustomer(t, uow, repository, "external-final-root")
	mergedID := createLineageCustomer(t, uow, repository, "external-before-merge")
	mergedCommand := command
	mergedCommand.CustomerID = mergedID
	mergedCommand.IdempotencyKey = fmt.Sprintf("p3-c07c-r3c-effective-root-%d", time.Now().UnixNano())
	mergedCommand.Payload = json.RawMessage(`{"source":"before-merge"}`)
	mergedEventID, err := appendExternalEvent(t, uow, repository, mergedCommand)
	if err != nil {
		t.Fatalf("append before merge: %v", err)
	}
	if err = uow.Within(context.Background(), func(txCtx context.Context) error {
		return repository.MergeCustomers(txCtx, contactport.MergeCustomersCommand{
			PrimaryID: finalRootID, MergedID: mergedID, Actor: "acceptance", Reason: "effective-root replay",
		})
	}); err != nil {
		t.Fatalf("merge after external event: %v", err)
	}
	for _, replayCustomerID := range []contactport.CustomerID{mergedID, finalRootID} {
		replay := mergedCommand
		replay.CustomerID = replayCustomerID
		if replayID, replayErr := appendExternalEvent(t, uow, repository, replay); replayErr != nil || replayID != mergedEventID {
			t.Fatalf("effective-root replay customer=%d event_id=%d error=%v want=%d", replayCustomerID, replayID, replayErr, mergedEventID)
		}
	}
	wrongRoot := mergedCommand
	wrongRoot.CustomerID = otherRootID
	if _, err = appendExternalEvent(t, uow, repository, wrongRoot); !errors.Is(err, contactport.ErrExternalEventConflict) {
		t.Fatalf("effective-root conflict error=%v", err)
	}
	assertExternalEventFacts(t, pool, mergedCommand.IdempotencyKey, mergedEventID, mergedID, 1, 1)
}

func TestExternalEventRollbackLeavesNoRegistryOrTimeline(t *testing.T) {
	pool := openExternalEventPool(t)
	repository := contactstore.NewMergePortRepository()
	uow := platformstore.NewUnitOfWork(pool)
	customerID := createLineageCustomer(t, uow, repository, "external-rollback")
	command := contactport.ExternalEventCommand{
		CustomerID:     customerID,
		EventType:      "extension.rollback_probe",
		Payload:        json.RawMessage(`{"must":"rollback"}`),
		Actor:          "ext:acceptance",
		OccurredAt:     time.Now().UTC().Truncate(time.Microsecond),
		IdempotencyKey: fmt.Sprintf("p3-c07c-r3c-rollback-%d", time.Now().UnixNano()),
	}
	rollbackMarker := errors.New("downstream identity failure")
	var rolledBackEventID contactport.EventID
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var appendErr error
		rolledBackEventID, appendErr = repository.AppendExternalEvent(txCtx, command)
		if appendErr != nil {
			return appendErr
		}
		return rollbackMarker
	})
	if !errors.Is(err, rollbackMarker) || rolledBackEventID <= 0 {
		t.Fatalf("rollback event_id=%d error=%v", rolledBackEventID, err)
	}
	assertExternalEventFacts(t, pool, command.IdempotencyKey, rolledBackEventID, customerID, 0, 0)

	missing := command
	missing.CustomerID = 9223372036854775000
	missing.IdempotencyKey += "-missing"
	if _, err = appendExternalEvent(t, uow, repository, missing); !errors.Is(err, contactport.ErrMergeCustomerNotFound) {
		t.Fatalf("missing customer error=%v", err)
	}
	assertExternalEventFacts(t, pool, missing.IdempotencyKey, 0, missing.CustomerID, 0, 0)
}

func openExternalEventPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe test database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(*databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 12
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL server_version_num=%q err=%v, want 160014", version, err)
	}
	return pool
}

func appendExternalEvent(
	t *testing.T,
	uow *platformstore.UnitOfWork,
	repository *contactstore.MergePortRepository,
	command contactport.ExternalEventCommand,
) (contactport.EventID, error) {
	t.Helper()
	var eventID contactport.EventID
	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		var appendErr error
		eventID, appendErr = repository.AppendExternalEvent(txCtx, command)
		return appendErr
	})
	return eventID, err
}

func assertExternalEventFacts(
	t *testing.T,
	pool *pgxpool.Pool,
	key string,
	eventID contactport.EventID,
	eventCustomerID contactport.CustomerID,
	wantRegistryCount, wantEventCount int,
) {
	t.Helper()
	var registryCount, eventCount int
	if err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM customer_event_idempotency WHERE idempotency_key=$1`, key).Scan(&registryCount); err != nil {
		t.Fatal(err)
	}
	if eventID > 0 {
		if err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM customer_events WHERE id=$1 AND customer_id=$2`, eventID, eventCustomerID).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
	}
	if registryCount != wantRegistryCount || eventCount != wantEventCount {
		t.Fatalf("key=%q event_id=%d registry=%d event=%d want registry=%d event=%d",
			key, eventID, registryCount, eventCount, wantRegistryCount, wantEventCount)
	}
}
