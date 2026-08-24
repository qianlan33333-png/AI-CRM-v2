package internaleventsacceptance_test

import (
	"context"
	"encoding/json"
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
	service := eventapp.NewInternalEventSafeExportService(platformstore.NewUnitOfWork(p), eventstore.NewInternalEventSafeExportRepository(), eventstore.NewAppender())
	t.Run("replay mismatch concurrency actor and side effects", func(t *testing.T) { testEE01Core(t, ctx, p, service) })
	t.Run("audit tamper fails closed", func(t *testing.T) { testEE01AuditTamper(t, ctx, p, service) })
	t.Run("completion invariant rolls back UoW", func(t *testing.T) { testEE01CompletionRollback(t, ctx, p) })
	t.Run("capacity boundary and overflow rollback", func(t *testing.T) { testEE01Capacity(t, ctx, p, service) })
}

type ee01MissingAuditAppender struct{}

func (ee01MissingAuditAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 999, nil
}

func testEE01CompletionRollback(t *testing.T, ctx context.Context, p *pgxpool.Pool) {
	before := readEE01MaterializedCounts(t, ctx, p)
	service := eventapp.NewInternalEventSafeExportService(platformstore.NewUnitOfWork(p), eventstore.NewInternalEventSafeExportRepository(), ee01MissingAuditAppender{})
	_, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: 92501, IdempotencyKey: fmt.Sprintf("ee01-missing-audit-%d", time.Now().UnixNano()), Filter: eventapp.InternalEventSafeExportFilter{EventType: "ee01.missing.audit"}})
	if !errors.Is(err, eventapp.ErrInternalEventSafeExportUnavailable) {
		t.Fatalf("missing audit err=%v", err)
	}
	after := readEE01MaterializedCounts(t, ctx, p)
	if after != before {
		t.Fatalf("failed completion left residue before=%+v after=%+v", before, after)
	}
}

func testEE01Core(t *testing.T, ctx context.Context, p *pgxpool.Pool, service *eventapp.InternalEventSafeExportService) {
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
	before := readEE01SideEffects(t, ctx, p)

	command := eventapp.InternalEventSafeExportCreate{ActorID: 91001, IdempotencyKey: marker + "-idempotency-key", Filter: eventapp.InternalEventSafeExportFilter{EventType: marker + ".empty"}}
	first, err := service.Create(ctx, command)
	if err != nil || first.RecordCount != 1 {
		t.Fatalf("create=%+v err=%v", first, err)
	}
	beforeReplay := readEE01MaterializedCounts(t, ctx, p)
	replay, err := service.Create(ctx, command)
	if err != nil || replay != first {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
	if afterReplay := readEE01MaterializedCounts(t, ctx, p); afterReplay != beforeReplay {
		t.Fatalf("exact replay wrote facts before=%+v after=%+v", beforeReplay, afterReplay)
	}
	if _, err := p.Exec(ctx, `INSERT INTO event_log(event_type,occurred_at,idempotency_key,dispatched)
VALUES ($1,now(),$2,FALSE)`, marker+".empty", marker+"-after-snapshot"); err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.Filter.Status = "pending"
	beforeMismatch := readEE01MaterializedCounts(t, ctx, p)
	if _, err := service.Create(ctx, changed); !errors.Is(err, eventapp.ErrInternalEventSafeExportConflict) {
		t.Fatalf("payload mismatch err=%v", err)
	}
	if afterMismatch := readEE01MaterializedCounts(t, ctx, p); afterMismatch != beforeMismatch {
		t.Fatalf("payload mismatch wrote facts before=%+v after=%+v", beforeMismatch, afterMismatch)
	}
	_, rows, err := service.Download(ctx, first.ID, command.ActorID)
	if err != nil || len(rows) != 1 || rows[0].EventID != eventport.EventID(eventID) || rows[0].Consumer != "" {
		t.Fatalf("download rows=%+v err=%v", rows, err)
	}
	if _, _, err := service.Download(ctx, first.ID, command.ActorID+1); !errors.Is(err, eventapp.ErrInternalEventSafeExportNotFound) {
		t.Fatalf("cross actor err=%v", err)
	}
	assertEE01FactCounts(t, ctx, p, first.ID, 1)

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
	assertEE01FactCounts(t, ctx, p, results[0].ID, results[0].RecordCount)

	empty, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: command.ActorID, IdempotencyKey: marker + "-empty-idempotency-key", Filter: eventapp.InternalEventSafeExportFilter{EventType: marker + ".missing"}})
	if err != nil || empty.RecordCount != 0 {
		t.Fatalf("empty export=%+v err=%v", empty, err)
	}
	assertEE01FactCounts(t, ctx, p, empty.ID, 0)
	after := readEE01SideEffects(t, ctx, p)
	if after.EventDeliveries != before.EventDeliveries || after.EventDeliveryJobs != before.EventDeliveryJobs || after.OutboundTasks != before.OutboundTasks || after.CustomerMerges != before.CustomerMerges || after.PendingEvents != before.PendingEvents {
		t.Fatalf("unexpected side effects before=%+v after=%+v", before, after)
	}
}

func testEE01AuditTamper(t *testing.T, ctx context.Context, p *pgxpool.Pool, service *eventapp.InternalEventSafeExportService) {
	marker := fmt.Sprintf("ee01-tamper-%d", time.Now().UnixNano())
	create := func(suffix string) eventapp.InternalEventSafeExport {
		result, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: 92001, IdempotencyKey: marker + suffix + "-idempotency", Filter: eventapp.InternalEventSafeExportFilter{EventType: marker + ".missing"}})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	updated := create("-update")
	var eventID int64
	var original json.RawMessage
	if err := p.QueryRow(ctx, `SELECT event.id,event.payload
FROM event_log event JOIN internal_event_safe_export_receipts receipt
  ON event.idempotency_key='internal-event-safe-export:' || receipt.id::text
WHERE receipt.export_id=$1`, updated.ID).Scan(&eventID, &original); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE event_log SET payload='{}'::jsonb WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Download(ctx, updated.ID, 92001); !errors.Is(err, eventapp.ErrInternalEventSafeExportUnavailable) {
		t.Fatalf("updated audit err=%v", err)
	}
	if _, err := p.Exec(ctx, `UPDATE event_log SET payload=$2::jsonb WHERE id=$1`, eventID, original); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Download(ctx, updated.ID, 92001); err != nil {
		t.Fatalf("restored audit err=%v", err)
	}

	deleted := create("-delete")
	if _, err := p.Exec(ctx, `DELETE FROM event_log event USING internal_event_safe_export_receipts receipt
WHERE receipt.export_id=$1 AND event.idempotency_key='internal-event-safe-export:' || receipt.id::text`, deleted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, deleted.ID, 92001); !errors.Is(err, eventapp.ErrInternalEventSafeExportUnavailable) {
		t.Fatalf("deleted audit err=%v", err)
	}
}

func testEE01Capacity(t *testing.T, ctx context.Context, p *pgxpool.Pool, service *eventapp.InternalEventSafeExportService) {
	marker := fmt.Sprintf("ee01-capacity-%d", time.Now().UnixNano())
	insertEvents := func(eventType string, count int) {
		if _, err := p.Exec(ctx, `INSERT INTO event_log(event_type,occurred_at,idempotency_key,dispatched)
SELECT $1,now()-interval '1 minute',$2 || '-' || value::text,FALSE FROM generate_series(1,$3) value`, eventType, marker+"-"+eventType, count); err != nil {
			t.Fatal(err)
		}
	}
	exactType := marker + ".exact"
	insertEvents(exactType, eventapp.InternalEventSafeExportMaximumRows)
	exact, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: 93001, IdempotencyKey: marker + "-exact-idempotency", Filter: eventapp.InternalEventSafeExportFilter{EventType: exactType}})
	if err != nil || exact.RecordCount != eventapp.InternalEventSafeExportMaximumRows {
		t.Fatalf("exact=%+v err=%v", exact, err)
	}
	assertEE01FactCounts(t, ctx, p, exact.ID, eventapp.InternalEventSafeExportMaximumRows)
	if verified, err := service.Get(ctx, exact.ID, 93001); err != nil || verified != exact {
		t.Fatalf("verified exact=%+v err=%v", verified, err)
	}

	overflowType := marker + ".overflow"
	insertEvents(overflowType, eventapp.InternalEventSafeExportMaximumRows+1)
	before := readEE01MaterializedCounts(t, ctx, p)
	if _, err := service.Create(ctx, eventapp.InternalEventSafeExportCreate{ActorID: 93001, IdempotencyKey: marker + "-overflow-idempotency", Filter: eventapp.InternalEventSafeExportFilter{EventType: overflowType}}); !errors.Is(err, eventapp.ErrInternalEventSafeExportConflict) {
		t.Fatalf("overflow err=%v", err)
	}
	after := readEE01MaterializedCounts(t, ctx, p)
	if after != before {
		t.Fatalf("overflow left residue before=%+v after=%+v", before, after)
	}
}

type ee01SideEffects struct {
	EventDeliveries   int64
	EventDeliveryJobs int64
	OutboundTasks     int64
	CustomerMerges    int64
	PendingEvents     int64
}

func readEE01SideEffects(t *testing.T, ctx context.Context, p *pgxpool.Pool) ee01SideEffects {
	t.Helper()
	var result ee01SideEffects
	if err := p.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM event_deliveries),
  (SELECT count(*) FROM outbound_tasks),
  (SELECT count(*) FROM customer_merges),
  (SELECT count(*) FROM pending_events)`).Scan(&result.EventDeliveries, &result.OutboundTasks, &result.CustomerMerges, &result.PendingEvents); err != nil {
		t.Fatal(err)
	}
	var riverExists bool
	if err := p.QueryRow(ctx, `SELECT to_regclass('public.river_job') IS NOT NULL`).Scan(&riverExists); err != nil {
		t.Fatal(err)
	}
	if riverExists {
		if err := p.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='events_deliver'`).Scan(&result.EventDeliveryJobs); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

type ee01MaterializedCounts struct{ Headers, Rows, Receipts, Audits int64 }

func readEE01MaterializedCounts(t *testing.T, ctx context.Context, p *pgxpool.Pool) ee01MaterializedCounts {
	t.Helper()
	var result ee01MaterializedCounts
	if err := p.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM internal_event_safe_exports),
  (SELECT count(*) FROM internal_event_safe_export_rows),
  (SELECT count(*) FROM internal_event_safe_export_receipts),
  (SELECT count(*) FROM event_log WHERE event_type='events.safe_export_created')`).Scan(&result.Headers, &result.Rows, &result.Receipts, &result.Audits); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertEE01FactCounts(t *testing.T, ctx context.Context, p *pgxpool.Pool, exportID string, rows int) {
	t.Helper()
	var headers, snapshotRows, receipts, audits int
	if err := p.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM internal_event_safe_exports WHERE id=$1),
  (SELECT count(*) FROM internal_event_safe_export_rows WHERE export_id=$1),
  (SELECT count(*) FROM internal_event_safe_export_receipts WHERE export_id=$1 AND state='completed'),
  (SELECT count(*) FROM event_log event JOIN internal_event_safe_export_receipts receipt
     ON event.idempotency_key='internal-event-safe-export:' || receipt.id::text
   WHERE receipt.export_id=$1 AND event.event_type='events.safe_export_created')`, exportID).Scan(&headers, &snapshotRows, &receipts, &audits); err != nil {
		t.Fatal(err)
	}
	if headers != 1 || snapshotRows != rows || receipts != 1 || audits != 1 {
		t.Fatalf("export=%s headers=%d rows=%d receipts=%d audits=%d", exportID, headers, snapshotRows, receipts, audits)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	p, err := pgxpool.New(ctx, *ee01DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p, ctx
}
