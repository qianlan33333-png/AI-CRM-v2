package internaleventsacceptance_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var ee01DatabaseURL = flag.String("ee01-database-url", "", "isolated EE01 PostgreSQL 16.14 acceptance database")

func TestInternalEventSafeExportPG16(t *testing.T) {
	p, ctx := openEE01Pool(t)
	marker := fmt.Sprintf("ee01-safe-export-%d", time.Now().UnixNano())
	stamp := time.Now().UTC().Add(-time.Minute)
	var eventID int64
	if err := p.QueryRow(ctx, `INSERT INTO event_log(event_type,occurred_at,idempotency_key,dispatched)
VALUES ($1,$2,$3,FALSE) RETURNING id`, marker+".empty", stamp, marker+"-empty").Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	var deliveredID int64
	if err := p.QueryRow(ctx, `INSERT INTO event_log(event_type,occurred_at,idempotency_key,dispatched)
VALUES ($1,$2,$3,TRUE) RETURNING id`, eventport.EvTagApplied, stamp, marker+"-delivery").Scan(&deliveredID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO event_deliveries(event_id,consumer,status,attempt_count)
VALUES ($1,$2,'pending',0)`, deliveredID, eventport.ConsumerAutomationTagTrigger); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(context.Background(), `DELETE FROM event_deliveries WHERE event_id IN ($1,$2)`, eventID, deliveredID)
		_, _ = p.Exec(context.Background(), `DELETE FROM event_log WHERE id IN ($1,$2)`, eventID, deliveredID)
	})

	service := eventapp.NewInternalEventSafeExportService(platformstore.NewUnitOfWork(p), eventstore.NewInternalEventSafeExportRepository(), eventstore.NewAppender())
	command := eventapp.InternalEventSafeExportCreate{ActorID: 91001, IdempotencyKey: marker + "-idempotency-key", Filter: eventapp.InternalEventSafeExportFilter{EventType: marker + ".empty"}}
	first, err := service.Create(ctx, command)
	if err != nil || first.RecordCount != 1 {
		t.Fatalf("create=%+v err=%v", first, err)
	}
	replay, err := service.Create(ctx, command)
	if err != nil || replay != first {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
	changed := command
	changed.Filter.Status = "pending"
	if _, err := service.Create(ctx, changed); !errors.Is(err, eventapp.ErrInternalEventSafeExportConflict) {
		t.Fatalf("payload mismatch err=%v", err)
	}
	_, rows, err := service.Download(ctx, first.ID, command.ActorID)
	if err != nil || len(rows) != 1 || rows[0].EventID != eventport.EventID(eventID) || rows[0].Consumer != "" {
		t.Fatalf("download rows=%+v err=%v", rows, err)
	}
	if _, _, err := service.Download(ctx, first.ID, command.ActorID+1); !errors.Is(err, eventapp.ErrInternalEventSafeExportNotFound) {
		t.Fatalf("cross actor err=%v", err)
	}

	concurrent := command
	concurrent.IdempotencyKey = marker + "-concurrent-idempotency-key"
	results := make([]eventapp.InternalEventSafeExport, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for i := range results {
		wait.Add(1)
		go func(i int) { defer wait.Done(); results[i], errs[i] = service.Create(context.Background(), concurrent) }(i)
	}
	wait.Wait()
	if errs[0] != nil || errs[1] != nil || results[0].ID == "" || results[0].ID != results[1].ID {
		t.Fatalf("concurrent results=%+v errors=%v", results, errs)
	}

	if _, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: command.ActorID, IdempotencyKey: marker + "-empty-idempotency-key", Filter: eventapp.InternalEventSafeExportFilter{EventType: marker + ".missing"}}); err != nil {
		t.Fatalf("empty export err=%v", err)
	}
}

func openEE01Pool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *ee01DatabaseURL == "" {
		t.Skip("ee01-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*ee01DatabaseURL, acceptancefixtures.EE01DatabaseName); err != nil {
		t.Fatalf("unsafe EE01 database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	p, err := pgxpool.New(ctx, *ee01DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p, ctx
}
