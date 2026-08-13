package segment_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

func TestRefreshRequestRepositoryPersistsAcceptedReceiptAndHeavyJob(t *testing.T) {
	dsn := os.Getenv("SEGMENT_REFRESH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEGMENT_REFRESH_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	var segmentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO segments (name, definition) VALUES ('S-5A acceptance', '{}') RETURNING id`).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	repository, err := segmentstore.NewRefreshRequestRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := segmentapp.NewRefreshRequestService(platformstore.NewUnitOfWork(pool), repository, repository)
	command := segmentport.RefreshCommand{SegmentID: segmentport.SegmentID(segmentID), Actor: "admin:42", IdempotencyKey: "s5a-refresh-command"}
	for attempt := 0; attempt < 2; attempt++ {
		accepted, err := service.RequestRefresh(ctx, command)
		if err != nil || accepted.ID != command.SegmentID {
			t.Fatalf("attempt %d RequestRefresh() = %#v, %v", attempt, accepted, err)
		}
	}
	var receiptID, receiptSegmentID, jobID int64
	var state string
	if err := pool.QueryRow(ctx, `SELECT id, segment_id, state, river_job_id FROM segment_refresh_receipts WHERE idempotency_scope = $1 AND idempotency_key = $2`, command.Actor, command.IdempotencyKey).Scan(&receiptID, &receiptSegmentID, &state, &jobID); err != nil {
		t.Fatal(err)
	}
	if receiptID <= 0 || receiptSegmentID != segmentID || state != "accepted" || jobID <= 0 {
		t.Fatalf("receipt = id=%d segment=%d state=%q job=%d, want accepted original fact", receiptID, receiptSegmentID, state, jobID)
	}
	var queue, kind string
	var encodedArgs []byte
	if err := pool.QueryRow(ctx, `SELECT queue, kind, args FROM river_job WHERE id = $1`, jobID).Scan(&queue, &kind, &encodedArgs); err != nil {
		t.Fatal(err)
	}
	var args segmentapp.RefreshRequestArgs
	if err := json.Unmarshal(encodedArgs, &args); err != nil {
		t.Fatal(err)
	}
	if queue != "heavy" || kind != segmentapp.RefreshRequestJobKind || args.SegmentID != command.SegmentID || args.ReceiptID != receiptID {
		t.Fatalf("River job = queue=%q kind=%q args=%#v, want heavy refresh command for receipt", queue, kind, args)
	}
	var conflictingSegmentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO segments (name, definition) VALUES ('S-5A conflict', '{}') RETURNING id`).Scan(&conflictingSegmentID); err != nil {
		t.Fatal(err)
	}
	_, err = service.RequestRefresh(ctx, segmentport.RefreshCommand{SegmentID: segmentport.SegmentID(conflictingSegmentID), Actor: command.Actor, IdempotencyKey: command.IdempotencyKey})
	if !errors.Is(err, segmentapp.ErrRefreshCommandConflict) {
		t.Fatalf("conflicting RequestRefresh() error = %v, want conflict", err)
	}
	var receiptCount, jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM segment_refresh_receipts WHERE idempotency_scope = $1 AND idempotency_key = $2`, command.Actor, command.IdempotencyKey).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE id = $1 AND kind = $2`, jobID, segmentapp.RefreshRequestJobKind).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 || jobCount != 1 {
		t.Fatalf("receipt/job counts = %d/%d, want exactly one persisted command", receiptCount, jobCount)
	}
}
