package segment_acceptance

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

func TestSegmentCRUDReceiptAndRuntimeFlow(t *testing.T) {
	dsn := os.Getenv("SEGMENT_CRUD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEGMENT_CRUD_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	fixtureTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var customerID int64
	customerID, err = contactfixture.CreateCustomer(ctx, fixtureTx)
	if err != nil {
		_ = fixtureTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = fixtureTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	service := segmentapp.NewCRUDService(platformstore.NewUnitOfWork(pool), segmentstore.NewCRUDRepository(), eventstore.NewAppender())
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	actor := segmentport.Actor("admin:" + suffix)
	create := segmentport.CreateCommand{
		Name: "高意向客户", Definition: segmentport.Definition(`{"field":"is_deleted","op":"eq","value":false}`),
		RefreshMode: segmentport.RefreshModeManual, Actor: actor, IdempotencyKey: "segment-create-" + suffix,
	}
	first, err := service.Create(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Create(ctx, create)
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("create replay = %#v, %v; want id %d", replayed, err, first.ID)
	}

	conflict := create
	conflict.Name = "不同载荷"
	if _, err = service.Create(ctx, conflict); !errors.Is(err, segmentapp.ErrSegmentCommandConflict) {
		t.Fatalf("create conflict error = %v", err)
	}

	name := "已更新人群"
	mode := segmentport.RefreshModeScheduled
	cron := "0 9 * * *"
	update := segmentapp.UpdateInput{SegmentID: first.ID, Name: &name, RefreshMode: &mode, RefreshCron: &cron, RefreshCronSet: true, Actor: actor, IdempotencyKey: "segment-update-" + suffix}
	updated, err := service.UpdateHTTP(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.RefreshCron == nil || *updated.RefreshCron != cron {
		t.Fatalf("updated segment = %#v", updated)
	}
	replayedUpdate, err := service.UpdateHTTP(ctx, update)
	if err != nil || replayedUpdate.ID != first.ID || replayedUpdate.Name != name || replayedUpdate.RefreshCron == nil || *replayedUpdate.RefreshCron != cron {
		t.Fatalf("update replay = %#v, %v", replayedUpdate, err)
	}

	var receiptCount, eventCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM segment_operation_receipts WHERE actor_scope = $1 AND state = 'completed'`, actor).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type IN ('segment.created', 'segment.updated') AND payload->>'segment_id' = $1`, strconv.FormatInt(int64(first.ID), 10)).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 2 || eventCount != 2 {
		t.Fatalf("receipt/event counts = %d/%d, want 2/2", receiptCount, eventCount)
	}

	if _, err = pool.Exec(ctx, `INSERT INTO segment_members (segment_id, customer_id) VALUES ($1, $2)`, first.ID, customerID); err != nil {
		t.Fatal(err)
	}
	members, err := service.ListMemberRecords(ctx, first.ID, "", 10)
	if err != nil || len(members.Items) != 1 || int64(members.Items[0].ID) != customerID {
		t.Fatalf("members = %#v, %v", members, err)
	}
	page, err := service.List(ctx, "", 10)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("segments = %#v, %v", page, err)
	}
	archive := segmentport.ArchiveCommand{
		SegmentID: first.ID, Actor: actor, IdempotencyKey: "segment-archive-" + suffix,
	}
	archived, err := service.Archive(ctx, archive)
	if err != nil || archived.ID != first.ID || archived.LifecycleStatus != segmentport.LifecycleStatusArchived {
		t.Fatalf("archived segment = %#v, %v", archived, err)
	}
	replayedArchive, err := service.Archive(ctx, archive)
	if err != nil || replayedArchive.ID != first.ID || replayedArchive.LifecycleStatus != segmentport.LifecycleStatusArchived {
		t.Fatalf("archive replay = %#v, %v", replayedArchive, err)
	}
	page, err = service.List(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == first.ID {
			t.Fatalf("archived segment %d remained in active list", first.ID)
		}
	}
	var archiveReceiptCount, archiveEventCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM segment_operation_receipts WHERE operation = 'archive' AND actor_scope = $1 AND state = 'completed'`, actor).Scan(&archiveReceiptCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type = 'segment.archived' AND payload->>'segment_id' = $1`, strconv.FormatInt(int64(first.ID), 10)).Scan(&archiveEventCount); err != nil {
		t.Fatal(err)
	}
	if archiveReceiptCount != 1 || archiveEventCount != 1 {
		t.Fatalf("archive receipt/event counts = %d/%d, want 1/1", archiveReceiptCount, archiveEventCount)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO segment_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
		VALUES ('create', 'admin:999', decode(repeat('ab', 32), 'hex'), decode(repeat('cd', 32), 'hex'), now())`)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("incomplete receipt commit succeeded")
	}
	_ = tx.Rollback(ctx)

	shadowTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shadowTx.Rollback(ctx) }()
	if _, err = shadowTx.Exec(ctx, `CREATE TEMP TABLE segment_operation_receipts (
		id BIGINT,
		state TEXT,
		result_segment_id BIGINT,
		completed_at TIMESTAMPTZ
	)`); err != nil {
		t.Fatal(err)
	}
	_, err = shadowTx.Exec(ctx, `INSERT INTO public.segment_operation_receipts (
		operation, actor_scope, key_digest, payload_digest, created_at
	) VALUES ('create', 'admin:1000', decode(repeat('ef', 32), 'hex'), decode(repeat('01', 32), 'hex'), now())`)
	if err != nil {
		t.Fatal(err)
	}
	if err = shadowTx.Commit(ctx); err == nil {
		t.Fatal("pg_temp shadow bypass allowed incomplete public receipt commit")
	}
}
