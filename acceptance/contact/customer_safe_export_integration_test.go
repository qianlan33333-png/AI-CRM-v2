package contact_test

import (
	"context"
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

	failing := contactapp.NewCustomerSafeExportService(platformstore.NewUnitOfWork(pool), contactstore.NewCustomerSafeExportRepository(), failingCustomerSafeExportAppender{})
	failed := command
	failed.IdempotencyKey = "customer-safe-export-pg-key-0002"
	if _, err = failing.Create(ctx, failed); err == nil {
		t.Fatal("event failure unexpectedly committed")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.customer_safe_exports WHERE actor_id=$1 AND id <> $2`, facts.actor, first.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback snapshots=%d err=%v", count, err)
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
