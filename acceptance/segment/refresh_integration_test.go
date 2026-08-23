package segment_acceptance

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

func TestRefreshOncePG16ReplacesMembersAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openRefreshPool(t, ctx)
	defer pool.Close()
	segmentID := insertRefreshSegment(t, ctx, pool,
		`{"and":[{"field":"stage_id","op":"in","value":[1,2]},{"field":"is_deleted","op":"eq","value":false}]}`,
	)
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	service := segmentapp.NewRefreshService(
		platformstore.NewUnitOfWork(pool), segmentstore.NewRefreshRepository(), eventstore.NewAppender(),
	)

	result, err := service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), reference)
	if err != nil {
		t.Fatalf("RefreshOnce() error = %v", err)
	}
	if result.MemberCount != 5 || result.RefreshedAt != reference {
		t.Fatalf("RefreshOnce() result = %#v, want five members at reference", result)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{1, 2, 3, 4, 5}, 5, reference, "idle")
	assertRefreshEvent(t, ctx, pool, segmentID, 5, reference)

	if _, err := pool.Exec(ctx, `UPDATE segments SET definition = '{"field":"stage_id","op":"eq","value":999}' WHERE id=$1`, segmentID); err != nil {
		t.Fatal(err)
	}
	result, err = service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), reference.Add(time.Minute))
	if err != nil || result.MemberCount != 0 {
		t.Fatalf("empty RefreshOnce() result/error = %#v/%v", result, err)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, nil, 0, reference.Add(time.Minute), "idle")
}

func TestRefreshOncePG16RollsBackPartialReplacementAndRejectsInjection(t *testing.T) {
	ctx := context.Background()
	pool := openRefreshPool(t, ctx)
	defer pool.Close()
	segmentID := insertRefreshSegment(t, ctx, pool,
		`{"field":"is_deleted","op":"eq","value":false}`,
	)
	originalAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	service := segmentapp.NewRefreshService(
		platformstore.NewUnitOfWork(pool), segmentstore.NewRefreshRepository(), eventstore.NewAppender(),
	)

	statements := []string{
		`CREATE FUNCTION reject_refresh_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rejection'; END $$`,
		`CREATE TRIGGER reject_refresh_insert BEFORE INSERT ON segment_members FOR EACH ROW EXECUTE FUNCTION reject_refresh_insert()`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	_, err := service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), originalAt.Add(time.Hour))
	if !errors.Is(err, segmentapp.ErrSegmentRefreshFailed) {
		t.Fatalf("RefreshOnce() error = %v, want replacement failure", err)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{8}, 1, originalAt, "failed")
	assertRefreshEventCount(t, ctx, pool, 0)
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_refresh_insert ON segment_members`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER reject_refresh_event BEFORE INSERT ON event_log FOR EACH ROW EXECUTE FUNCTION reject_refresh_insert()`); err != nil {
		t.Fatal(err)
	}
	_, err = service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), originalAt.Add(90*time.Minute))
	if !errors.Is(err, segmentapp.ErrSegmentRefreshFailed) {
		t.Fatalf("event append RefreshOnce() error = %v, want transaction failure", err)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{8}, 1, originalAt, "failed")
	assertRefreshEventCount(t, ctx, pool, 0)
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_refresh_event ON event_log`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE segments SET definition = '{"field":"stage_id) OR true --","op":"eq","value":1}' WHERE id=$1`, segmentID); err != nil {
		t.Fatal(err)
	}
	_, err = service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), originalAt.Add(2*time.Hour))
	if !errors.Is(err, segmentapp.ErrSegmentRefreshFailed) {
		t.Fatalf("SQL-shaped RefreshOnce() error = %v, want fail closed", err)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{8}, 1, originalAt, "failed")
	assertRefreshEventCount(t, ctx, pool, 0)
}

func TestRefreshOncePG16SerializesConcurrentSameSegmentCalls(t *testing.T) {
	ctx := context.Background()
	pool := openRefreshPool(t, ctx)
	defer pool.Close()
	segmentID := insertRefreshSegment(t, ctx, pool,
		`{"or":[{"field":"tag_id","op":"has_any","value":[1]},{"field":"channel_id","op":"eq","value":1}]}`,
	)
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	service := segmentapp.NewRefreshService(
		platformstore.NewUnitOfWork(pool), segmentstore.NewRefreshRepository(), eventstore.NewAppender(),
	)

	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			_, err := service.RefreshOnce(ctx, segmentport.SegmentID(segmentID), reference)
			errorsSeen <- err
		}()
	}
	ready.Wait()
	close(start)
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatalf("concurrent RefreshOnce() error = %v", err)
		}
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{1, 4, 5, 8}, 4, reference, "idle")
	assertRefreshEventCount(t, ctx, pool, 1)
}

func assertRefreshEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, segmentID, memberCount int64, occurredAt time.Time) {
	t.Helper()
	var eventType, key, payload string
	var gotAt time.Time
	if err := pool.QueryRow(ctx, `SELECT event_type, payload::text, occurred_at, idempotency_key FROM event_log`).Scan(&eventType, &payload, &gotAt, &key); err != nil {
		t.Fatal(err)
	}
	wantPayload := `{"segment_id": ` + strconv.FormatInt(segmentID, 10) + `, "member_count": ` + strconv.FormatInt(memberCount, 10) + `}`
	if eventType != "segment.refreshed" || payload != wantPayload || !gotAt.Equal(occurredAt) || key != "segment.refresh:"+strconv.FormatInt(segmentID, 10)+":"+occurredAt.Format(time.RFC3339Nano) {
		t.Fatalf("refresh event = (%s,%s,%v,%s), want stable committed fact", eventType, payload, gotAt, key)
	}
}

func assertRefreshEventCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("refresh event count = %d, want %d", got, want)
	}
}

func openRefreshPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SEGMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEGMENT_TEST_DATABASE_URL is required for the isolated PG16.14 test")
	}
	if dsn != segmentTestDSN {
		t.Fatal("SEGMENT_TEST_DATABASE_URL must select the isolated Segment slot")
	}
	setup, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	seedSegmentFixture(t, ctx, setup)
	for _, statement := range []string{
		`CREATE TABLE acceptance_fixtures.segments (
		 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		 definition JSONB NOT NULL,
		 member_count BIGINT NOT NULL DEFAULT 0,
		 refreshed_at TIMESTAMPTZ,
		 refresh_status TEXT NOT NULL DEFAULT 'idle',
		 lifecycle_status TEXT NOT NULL DEFAULT 'active',
		 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE acceptance_fixtures.segment_members (
		 segment_id BIGINT NOT NULL REFERENCES acceptance_fixtures.segments(id) ON DELETE CASCADE,
		 customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),
		 computed_at TIMESTAMPTZ NOT NULL,
		 PRIMARY KEY (segment_id, customer_id)
		)`,
		`CREATE TABLE acceptance_fixtures.event_log (
		 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		 event_type TEXT NOT NULL,
		 customer_id BIGINT,
		 payload JSONB NOT NULL,
		 occurred_at TIMESTAMPTZ NOT NULL,
		 idempotency_key TEXT NOT NULL UNIQUE
		)`,
	} {
		if _, err := setup.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := setup.Close(ctx); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET search_path TO acceptance_fixtures, pg_catalog")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	var version string
	if err := pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		pool.Close()
		t.Fatalf("PostgreSQL version = %q, err = %v", version, err)
	}
	return pool
}

func insertRefreshSegment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, definition string) int64 {
	t.Helper()
	originalAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	var segmentID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO segments (definition, member_count, refreshed_at, refresh_status, updated_at)
		VALUES ($1::jsonb, 1, $2, 'failed', $2)
		RETURNING id`, definition, originalAt).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO segment_members (segment_id, customer_id, computed_at) VALUES ($1, 8, $2)`, segmentID, originalAt); err != nil {
		t.Fatal(err)
	}
	return segmentID
}

func assertRefreshProjection(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	segmentID int64,
	wantIDs []int64,
	wantCount int64,
	wantAt time.Time,
	wantStatus string,
) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT customer_id, computed_at FROM segment_members WHERE segment_id=$1 ORDER BY customer_id`, segmentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var gotIDs []int64
	for rows.Next() {
		var customerID int64
		var computedAt time.Time
		if err := rows.Scan(&customerID, &computedAt); err != nil {
			t.Fatal(err)
		}
		if !computedAt.Equal(wantAt) {
			t.Fatalf("computed_at = %v, want %v", computedAt, wantAt)
		}
		gotIDs = append(gotIDs, customerID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("member IDs = %v, want %v", gotIDs, wantIDs)
	}
	var gotCount int64
	var gotAt time.Time
	var gotStatus string
	if err := pool.QueryRow(ctx, `SELECT member_count, refreshed_at, refresh_status FROM segments WHERE id=$1`, segmentID).Scan(&gotCount, &gotAt, &gotStatus); err != nil {
		t.Fatal(err)
	}
	if gotCount != wantCount || !gotAt.Equal(wantAt) || gotStatus != wantStatus {
		t.Fatalf("segment summary = (%d,%v,%s), want (%d,%v,%s)", gotCount, gotAt, gotStatus, wantCount, wantAt, wantStatus)
	}
}
