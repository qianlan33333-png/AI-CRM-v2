package contact_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eventfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/acceptancefixture"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerSafeExportLocalCorePG16(t *testing.T) {
	pool, ctx := openCustomerSafeExportPool(t)
	service := contactapp.NewCustomerSafeExportService(platformstore.NewUnitOfWork(pool), contactstore.NewCustomerSafeExportRepository(), eventstore.NewAppender())
	facts := seedCustomerSafeExportFacts(t, ctx, pool)
	command := contactapp.CustomerSafeExportCreate{ActorID: facts.actor, OwnerScopeStaffID: &facts.owner, IdempotencyKey: "customer-safe-export-pg-key-0001", Filter: contactapp.CustomerListInput{OwnerStaffID: &facts.owner}}

	first, err := service.Create(ctx, command)
	if err != nil || first.RecordCount != 1 {
		t.Fatalf("create=%+v err=%v", first, err)
	}
	replay, err := service.Create(ctx, command)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
	changed := command
	changed.Filter.Keyword = "different"
	if _, err = service.Create(ctx, changed); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("different payload error=%v", err)
	}
	assertCustomerSafeExportFacts(t, ctx, pool, facts.actor, first.ID, facts.customer)
	assertCustomerSafeExportConcurrentReplay(t, ctx, service, facts, command)
	assertCustomerSafeExportActorAndScope(t, ctx, service, facts, command)
	var before, after int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.customer_safe_exports WHERE actor_id=$1`, facts.actor).Scan(&before); err != nil {
		t.Fatal(err)
	}
	failing := contactapp.NewCustomerSafeExportService(platformstore.NewUnitOfWork(pool), contactstore.NewCustomerSafeExportRepository(), failingCustomerSafeExportAppender{})
	failed := command
	failed.IdempotencyKey = "customer-safe-export-pg-key-0002"
	if _, err = failing.Create(ctx, failed); err == nil {
		t.Fatal("event failure unexpectedly committed")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.customer_safe_exports WHERE actor_id=$1`, facts.actor).Scan(&after); err != nil || after != before {
		t.Fatalf("rollback snapshots before/after=%d/%d err=%v", before, after, err)
	}
	if _, err = service.Get(ctx, first.ID, facts.actor, &facts.owner); err != nil {
		t.Fatalf("sales get: %v", err)
	}
	if _, err = service.Get(ctx, first.ID, facts.actor, nil); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("global actor scope get error=%v", err)
	}
	if _, err = service.Get(ctx, first.ID, facts.actor, &facts.otherOwner); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("other sales scope get error=%v", err)
	}
	if _, _, err = service.Download(ctx, first.ID, facts.actor, nil); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("global actor scope download error=%v", err)
	}
	if _, _, err = service.Download(ctx, first.ID, facts.actor, &facts.otherOwner); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("other sales scope download error=%v", err)
	}
	assertCustomerSafeExportImmutable(t, ctx, pool, first.ID, facts.customer)
	assertCustomerSafeExportReservedReceiptDeferred(t, ctx, pool, facts)
	assertCustomerSafeExportCompletionGuards(t, ctx, pool, facts)
	assertCustomerSafeExportCompletionSerializesRows(t, ctx, pool, facts)
	assertCustomerSafeExportBounds(t, ctx, pool, service, facts)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM public.customers WHERE id=$1 FOR UPDATE`, facts.customer); err != nil {
		t.Fatal(err)
	}
	downloadDone := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		_, _, downloadErr := service.Download(context.Background(), first.ID, facts.actor, &facts.owner)
		downloadDone <- downloadErr
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err = tx.Exec(ctx, `UPDATE public.customers SET owner_staff_id=$2,updated_at=now() WHERE id=$1`, facts.customer, facts.otherOwner); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if err = <-downloadDone; !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("locked owner drift download error=%v", err)
	}

	if _, _, err = service.Download(ctx, first.ID, facts.actor, &facts.owner); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("owner drift download error=%v", err)
	}

}

func assertCustomerSafeExportConcurrentReplay(t *testing.T, ctx context.Context, service *contactapp.CustomerSafeExportService, facts customerSafeExportFacts, command contactapp.CustomerSafeExportCreate) {
	t.Helper()
	concurrent := command
	concurrent.IdempotencyKey = "customer-safe-export-pg-concurrent-key-0001"
	results := make([]contactapp.CustomerSafeExport, 2)
	errors := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errors[index] = service.Create(ctx, concurrent)
		}(index)
	}
	wait.Wait()
	if errors[0] != nil || errors[1] != nil || results[0].ID == "" || results[0].ID != results[1].ID {
		t.Fatalf("concurrent create results=%+v errors=%v", results, errors)
	}
}

func assertCustomerSafeExportActorAndScope(t *testing.T, ctx context.Context, service *contactapp.CustomerSafeExportService, facts customerSafeExportFacts, command contactapp.CustomerSafeExportCreate) {
	t.Helper()
	admin := command
	admin.OwnerScopeStaffID = nil
	admin.IdempotencyKey = "customer-safe-export-pg-admin-scope-key-0001"
	export, err := service.Create(ctx, admin)
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if _, err = service.Get(ctx, export.ID, facts.actor, &facts.owner); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("admin export sales get error=%v", err)
	}
	if _, _, err = service.Download(ctx, export.ID, facts.actor, &facts.owner); !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
		t.Fatalf("admin export sales download error=%v", err)
	}
	if _, err = service.Get(ctx, export.ID, facts.actor+1, nil); !errors.Is(err, contactapp.ErrCustomerSafeExportNotFound) {
		t.Fatalf("cross actor get error=%v", err)
	}
	if _, _, err = service.Download(ctx, export.ID, facts.actor+1, nil); !errors.Is(err, contactapp.ErrCustomerSafeExportNotFound) {
		t.Fatalf("cross actor download error=%v", err)
	}
}

func assertCustomerSafeExportBounds(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *contactapp.CustomerSafeExportService, facts customerSafeExportFacts) {
	t.Helper()
	for _, test := range []struct {
		rows    int
		wantErr bool
	}{{rows: 10000}, {rows: 10001, wantErr: true}} {
		marker := time.Now().UnixNano()
		var owner int64
		if err := pool.QueryRow(ctx, `INSERT INTO public.staff(wecom_userid,name,is_active) VALUES($1,$1,TRUE) RETURNING id`, fmt.Sprintf("export-bound-owner-%d", marker)).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO public.customers(name,owner_staff_id) SELECT 'export-bound-' || value,$1 FROM generate_series(1,$2) AS value`, owner, test.rows); err != nil {
			t.Fatal(err)
		}
		command := contactapp.CustomerSafeExportCreate{ActorID: facts.actor + marker, OwnerScopeStaffID: &owner, IdempotencyKey: fmt.Sprintf("customer-safe-export-bound-key-%d", marker), Filter: contactapp.CustomerListInput{OwnerStaffID: &owner}}
		export, err := service.Create(ctx, command)
		if test.wantErr {
			if !errors.Is(err, contactapp.ErrCustomerSafeExportConflict) {
				t.Fatalf("rows=%d error=%v", test.rows, err)
			}
			continue
		}
		if err != nil || export.RecordCount != test.rows {
			t.Fatalf("rows=%d export=%+v error=%v", test.rows, export, err)
		}
	}
}

func assertCustomerSafeExportImmutable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, exportID string, customerID int64) {
	t.Helper()
	for _, test := range []struct {
		statement string
		args      []any
	}{
		{statement: `UPDATE public.customer_safe_exports SET record_count=0 WHERE id=$1`, args: []any{exportID}},
		{statement: `DELETE FROM public.customer_safe_exports WHERE id=$1`, args: []any{exportID}},
		{statement: `UPDATE public.customer_safe_export_rows SET display_name='changed' WHERE export_id=$1`, args: []any{exportID}},
		{statement: `DELETE FROM public.customer_safe_export_rows WHERE export_id=$1`, args: []any{exportID}},
		{statement: `INSERT INTO public.customer_safe_export_rows(export_id,row_index,customer_id,display_name) VALUES($1,2,$2,'late')`, args: []any{exportID, customerID}},
	} {
		if _, err := pool.Exec(ctx, test.statement, test.args...); err == nil || pgErrorCode(err) != "55000" {
			t.Fatalf("immutable statement=%q error=%v", test.statement, err)
		}
	}
}

func assertCustomerSafeExportCompletionGuards(t *testing.T, ctx context.Context, pool *pgxpool.Pool, facts customerSafeExportFacts) {
	t.Helper()
	for _, test := range []struct {
		name          string
		recordCount   int
		initialRows   int
		includeEvent  bool
		headerActorID int64
		wrongDigest   bool
	}{
		{name: "row count", recordCount: 1, initialRows: 0},
		{name: "missing event", recordCount: 1, initialRows: 1},
		{name: "actor mismatch", recordCount: 1, initialRows: 1, includeEvent: true, headerActorID: facts.actor + 1},
		{name: "event payload", recordCount: 1, initialRows: 1, includeEvent: true, wrongDigest: true},
		{name: "result snapshot", recordCount: 1, initialRows: 1, includeEvent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending := seedPendingCustomerSafeExportCompletion(t, ctx, pool, facts, test.recordCount, test.initialRows, test.includeEvent, test.headerActorID, test.wrongDigest)
			snapshot := pending.resultSnapshot()
			if test.name == "result snapshot" {
				snapshot = customerSafeExportResultSnapshot(pending.exportID, pending.recordCount+1, pending.snapshotTime)
			}
			if err := completeCustomerSafeExportReceipt(ctx, pool, pending.receiptID, pending.exportID, snapshot); err == nil || pgErrorCode(err) != "55000" {
				t.Fatalf("malformed completion error=%v", err)
			}
		})
	}
}

func assertCustomerSafeExportCompletionSerializesRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, facts customerSafeExportFacts) {
	t.Helper()
	t.Run("completion before append", func(t *testing.T) {
		pending := seedPendingCustomerSafeExportCompletion(t, ctx, pool, facts, 1, 1, true, facts.actor, false)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, customerSafeExportCompletionSQL, pending.receiptID, pending.exportID, pending.resultSnapshot()); err != nil {
			t.Fatalf("complete before append: %v", err)
		}
		appendDone := make(chan error, 1)
		go func() {
			_, appendErr := pool.Exec(context.Background(), customerSafeExportAppendRowSQL, pending.exportID, 2, pending.appendCustomer, "append-after-complete")
			appendDone <- appendErr
		}()
		assertCustomerSafeExportBlocked(t, appendDone)
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-appendDone; err == nil || pgErrorCode(err) != "55000" {
			t.Fatalf("append after completed receipt error=%v", err)
		}
	})
	t.Run("append before completion", func(t *testing.T) {
		pending := seedPendingCustomerSafeExportCompletion(t, ctx, pool, facts, 2, 1, true, facts.actor, false)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, customerSafeExportAppendRowSQL, pending.exportID, 2, pending.appendCustomer, "append-before-complete"); err != nil {
			t.Fatalf("append before completion: %v", err)
		}
		completionDone := make(chan error, 1)
		go func() {
			completionDone <- completeCustomerSafeExportReceipt(context.Background(), pool, pending.receiptID, pending.exportID, pending.resultSnapshot())
		}()
		assertCustomerSafeExportBlocked(t, completionDone)
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-completionDone; err != nil {
			t.Fatalf("completion after append: %v", err)
		}
	})
}

const customerSafeExportCompletionSQL = `
UPDATE public.customer_safe_export_receipts
SET export_id=$2,state='completed',result_snapshot=$3::jsonb,completed_at=now()
WHERE id=$1 AND state='reserved'`

const customerSafeExportAppendRowSQL = `
INSERT INTO public.customer_safe_export_rows(export_id,row_index,customer_id,display_name)
VALUES($1,$2,$3,$4)`

type pendingCustomerSafeExportCompletion struct {
	exportID       string
	receiptID      int64
	appendCustomer int64
	recordCount    int
	snapshotTime   time.Time
}

func seedPendingCustomerSafeExportCompletion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, facts customerSafeExportFacts, recordCount, initialRows int, includeEvent bool, headerActorID int64, wrongDigest bool) pendingCustomerSafeExportCompletion {
	t.Helper()
	marker := time.Now().UnixNano()
	exportID := fmt.Sprintf("cse_%032x", marker)
	snapshotTime := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	filterDigest := sha256.Sum256([]byte(exportID + ":filter"))
	keyDigest := sha256.Sum256([]byte(exportID + ":key"))
	payloadDigest := sha256.Sum256([]byte(exportID + ":payload"))
	if headerActorID == 0 {
		headerActorID = facts.actor
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire malformed completion fixture connection: %v", err)
	}
	defer connection.Release()
	if _, err = connection.Exec(ctx, `SET session_replication_role = 'replica'`); err != nil {
		t.Fatalf("disable completion fixture triggers: %v", err)
	}
	defer func() {
		if _, restoreErr := connection.Exec(context.Background(), `SET session_replication_role = 'origin'`); restoreErr != nil {
			t.Errorf("restore completion fixture triggers: %v", restoreErr)
		}
	}()
	if _, err := connection.Exec(ctx, `
INSERT INTO public.customer_safe_exports(id,actor_id,owner_scope_staff_id,filter_digest,watermark,record_count,created_at)
VALUES($1,$2,$3,$4,$5,$6,$5)`, exportID, headerActorID, facts.owner, filterDigest[:], snapshotTime, recordCount); err != nil {
		t.Fatalf("seed export header: %v", err)
	}
	var receiptID int64
	if err := connection.QueryRow(ctx, `
INSERT INTO public.customer_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES($1,$2,$3,now()) RETURNING id`, facts.actor, keyDigest[:], payloadDigest[:]).Scan(&receiptID); err != nil {
		t.Fatalf("seed export receipt: %v", err)
	}
	customers := make([]int64, 0, initialRows+1)
	for index := 0; index < initialRows+1; index++ {
		var customerID int64
		if err := connection.QueryRow(ctx, `INSERT INTO public.customers(name,owner_staff_id) VALUES($1,$2) RETURNING id`, fmt.Sprintf("completion-customer-%d-%d", marker, index), facts.owner).Scan(&customerID); err != nil {
			t.Fatalf("seed completion customer: %v", err)
		}
		customers = append(customers, customerID)
	}
	for index := 0; index < initialRows; index++ {
		if _, err := connection.Exec(ctx, customerSafeExportAppendRowSQL, exportID, index+1, customers[index], fmt.Sprintf("completion-row-%d", index+1)); err != nil {
			t.Fatalf("seed completion row: %v", err)
		}
	}
	if includeEvent {
		eventDigest := filterDigest[:]
		if wrongDigest {
			wrong := sha256.Sum256([]byte(exportID + ":wrong"))
			eventDigest = wrong[:]
		}
		if err := eventfixture.AppendCustomerSafeExportCreated(ctx, pool, exportID, receiptID, recordCount, eventDigest); err != nil {
			t.Fatalf("seed completion event: %v", err)
		}
	}
	if _, err = connection.Exec(ctx, `SET session_replication_role = 'origin'`); err != nil {
		t.Fatalf("restore completion fixture triggers: %v", err)
	}
	return pendingCustomerSafeExportCompletion{exportID: exportID, receiptID: receiptID, appendCustomer: customers[initialRows], recordCount: recordCount, snapshotTime: snapshotTime}
}

func (pending pendingCustomerSafeExportCompletion) resultSnapshot() string {
	return customerSafeExportResultSnapshot(pending.exportID, pending.recordCount, pending.snapshotTime)
}

func completeCustomerSafeExportReceipt(ctx context.Context, pool *pgxpool.Pool, receiptID int64, exportID, snapshot string) error {
	_, err := pool.Exec(ctx, customerSafeExportCompletionSQL, receiptID, exportID, snapshot)
	return err
}

func customerSafeExportResultSnapshot(exportID string, recordCount int, snapshotTime time.Time) string {
	return fmt.Sprintf(`{"id":%q,"record_count":%d,"watermark":%q,"created_at":%q}`, exportID, recordCount, snapshotTime.UTC().Format(time.RFC3339Nano), snapshotTime.UTC().Format(time.RFC3339Nano))
}

func assertCustomerSafeExportReservedReceiptDeferred(t *testing.T, ctx context.Context, pool *pgxpool.Pool, facts customerSafeExportFacts) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(fmt.Sprintf("deferred-receipt-%d", time.Now().UnixNano())))
	payloadDigest := sha256.Sum256([]byte("deferred-receipt-payload"))
	if _, err := pool.Exec(ctx, `
INSERT INTO public.customer_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES($1,$2,$3,now())`, facts.actor, keyDigest[:], payloadDigest[:]); err == nil || pgErrorCode(err) != "55000" {
		t.Fatalf("reserved receipt commit error=%v", err)
	}
}

func assertCustomerSafeExportBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("concurrent operation did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func pgErrorCode(err error) string {
	var databaseErr *pgconn.PgError
	if errors.As(err, &databaseErr) {
		return databaseErr.Code
	}
	return ""
}

type customerSafeExportFacts struct{ actor, owner, otherOwner, customer int64 }

func openCustomerSafeExportPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("P4CUSTOMER_SAFE_EXPORT_ACCEPTANCE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("P4CUSTOMER_SAFE_EXPORT_ACCEPTANCE_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func seedCustomerSafeExportFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) customerSafeExportFacts {
	t.Helper()
	marker := time.Now().UnixNano()
	var facts customerSafeExportFacts
	err := pool.QueryRow(ctx, `
WITH actor AS (
 SELECT $1::bigint AS id
), owners AS (
 INSERT INTO public.staff(wecom_userid,name,is_active) VALUES($2,$2,TRUE),($3,$3,TRUE) RETURNING id,wecom_userid
), customer AS (
 INSERT INTO public.customers(name,owner_staff_id) SELECT $4,id FROM owners WHERE wecom_userid=$2 RETURNING id
)
SELECT (SELECT id FROM actor),(SELECT id FROM owners WHERE wecom_userid=$2),(SELECT id FROM owners WHERE wecom_userid=$3),(SELECT id FROM customer)`,
		marker, fmt.Sprintf("export-owner-%d", marker), fmt.Sprintf("export-other-%d", marker), "=formula").Scan(&facts.actor, &facts.owner, &facts.otherOwner, &facts.customer)
	if err != nil {
		t.Fatalf("seed customer export facts: %v", err)
	}
	return facts
}

func assertCustomerSafeExportFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, actor int64, exportID string, customer int64) {
	t.Helper()
	var rows, receipts, events int
	var completed bool
	err := pool.QueryRow(ctx, `
SELECT (SELECT count(*) FROM public.customer_safe_export_rows WHERE export_id=$1),
       (SELECT count(*) FROM public.customer_safe_export_receipts WHERE actor_id=$2 AND export_id=$1 AND state='completed'),
       (SELECT count(*) FROM public.event_log WHERE event_type=$3),
       (SELECT bool_and(state='completed') FROM public.customer_safe_export_receipts WHERE actor_id=$2 AND export_id=$1)`, exportID, actor, eventport.EvCustomerSafeExportCreated).Scan(&rows, &receipts, &events, &completed)
	if err != nil || rows != 1 || receipts != 1 || events != 1 || !completed {
		t.Fatalf("rows=%d receipts=%d events=%d completed=%t err=%v", rows, receipts, events, completed, err)
	}
	var recorded int64
	if err = pool.QueryRow(ctx, `SELECT customer_id FROM public.customer_safe_export_rows WHERE export_id=$1`, exportID).Scan(&recorded); err != nil || recorded != customer {
		t.Fatalf("customer=%d want=%d err=%v", recorded, customer, err)
	}
	var exportDigest, eventDigest string
	err = pool.QueryRow(ctx, `
SELECT encode(e.filter_digest,'hex'),l.payload->>'filter_digest'
FROM public.customer_safe_exports e
JOIN public.event_log l ON l.event_type=$2 AND l.payload->>'export_id'=e.id
WHERE e.id=$1`, exportID, eventport.EvCustomerSafeExportCreated).Scan(&exportDigest, &eventDigest)
	if err != nil || exportDigest != eventDigest {
		t.Fatalf("export digest=%q event digest=%q err=%v", exportDigest, eventDigest, err)
	}
}

type failingCustomerSafeExportAppender struct{}

func (failingCustomerSafeExportAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("injected event failure")
}
