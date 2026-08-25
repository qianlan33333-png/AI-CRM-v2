package radarfixture

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
)

var trackingDatabaseURL = flag.String("radar-tracking-database-url", "", "isolated Radar tracking PostgreSQL 16 acceptance database")

func TestRadarLocalTrackingPostgreSQL16(t *testing.T) {
	if *trackingDatabaseURL == "" {
		t.Skip("-radar-tracking-database-url is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *trackingDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 16 {
		t.Fatalf("PostgreSQL version=%d err=%v", version, err)
	}
	const code = "rd_RRRRRRRRRRRRRRRRRRRRRR"
	var linkID int64
	if err = pool.QueryRow(ctx, `
INSERT INTO public.radar_links (
  public_code, name, title, destination_url, status, version,
  created_by, updated_by, created_at, updated_at
) VALUES ($1, 'Tracking acceptance', 'Tracking acceptance', 'https://example.com/radar', 'enabled', 1, 1, 1, now(), now())
RETURNING id`, code).Scan(&linkID); err != nil {
		t.Fatal(err)
	}

	repository := radarstore.NewPostgresRepository()
	service, err := radarapp.NewService(platformstore.NewUnitOfWork(pool), repository, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := service.ResolvePublicRedirect(ctx, code)
	if err != nil || redirect.DestinationURL != "https://example.com/radar" || !redirect.Receipt.LocalReceipt || redirect.Receipt.IdentityAttributed || redirect.Receipt.RealExternalCallExecuted {
		t.Fatalf("redirect=%#v err=%v", redirect, err)
	}

	page := int32(4)
	command := radarport.RecordEventCommand{PublicCode: code, Stage: radarport.EventStagePDFPageLoaded, Page: &page, Extra: map[string]any{"variant": "mobile"}, IdempotencyKey: "radar-pg16-event-key-001"}
	const workers = 8
	receipts := make(chan radarport.EventReceipt, workers)
	errorsSeen := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, recordErr := service.RecordPublicEvent(ctx, command)
			receipts <- receipt
			errorsSeen <- recordErr
		}()
	}
	wait.Wait()
	close(receipts)
	close(errorsSeen)
	for recordErr := range errorsSeen {
		if recordErr != nil {
			t.Fatalf("concurrent record error=%v", recordErr)
		}
	}
	var receiptID string
	for receipt := range receipts {
		if !receipt.LocalReceipt || receipt.IdentityAttributed || receipt.RealExternalCallExecuted {
			t.Fatalf("receipt=%#v", receipt)
		}
		if receiptID == "" {
			receiptID = receipt.ReceiptID
		} else if receipt.ReceiptID != receiptID {
			t.Fatalf("receipt IDs differ: first=%s current=%s", receiptID, receipt.ReceiptID)
		}
	}
	command.Extra["variant"] = "desktop"
	if _, err = service.RecordPublicEvent(ctx, command); !errors.Is(err, radarport.ErrIdempotencyConflict) {
		t.Fatalf("payload conflict error=%v", err)
	}

	stage := radarport.EventStagePDFPageLoaded
	start, end := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	events, err := service.ListEvents(ctx, radarport.EventListInput{LinkID: radarport.LinkID(linkID), Stage: &stage, Start: &start, End: &end, Limit: 10})
	if err != nil || events.Total != 1 || len(events.Items) != 1 || events.Items[0].ReceiptID != receiptID || events.IdentityAttributed || events.RealExternalCallExecuted {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	stats, err := service.EventStats(ctx, radarport.LinkID(linkID))
	if err != nil || stats.TotalEvents != 3 || stats.TotalLandings != 1 || stats.Redirects != 1 || stats.AuthorizedUsers != 0 || stats.IdentityAttributed || stats.RealExternalCallExecuted {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	sidebar, err := service.SidebarLinks(ctx, 10, 0, "https://crm.example.com")
	if err != nil || sidebar.Total != 1 || len(sidebar.Items) != 1 || sidebar.Items[0].URL != "https://crm.example.com/r/"+code || !sidebar.LocalProjection {
		t.Fatalf("sidebar=%#v err=%v", sidebar, err)
	}

	assertTrackingStorageIsPIIMinimal(t, ctx, pool, linkID, command.IdempotencyKey)
	if _, err = pool.Exec(ctx, `UPDATE public.radar_link_events SET created_at = created_at WHERE link_id = $1`, linkID); err == nil {
		t.Fatal("immutable receipt update unexpectedly succeeded")
	}
}

func assertTrackingStorageIsPIIMinimal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, linkID int64, rawKey string) {
	t.Helper()
	var forbiddenColumns int
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'radar_link_events'
  AND column_name IN ('openid','unionid','external_userid','customer_id','ip','ip_address','user_agent','referer','query_params_json','extra')`).Scan(&forbiddenColumns); err != nil {
		t.Fatal(err)
	}
	if forbiddenColumns != 0 {
		t.Fatalf("forbidden PII/raw columns=%d", forbiddenColumns)
	}
	keyDigest := sha256.Sum256([]byte(rawKey))
	var rows, rawMatches, digestMatches int
	if err := pool.QueryRow(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE key_digest = convert_to($2, 'UTF8')),
       count(*) FILTER (WHERE key_digest = $3)
FROM public.radar_link_events
WHERE link_id = $1 AND source = 'public_event'`, linkID, rawKey, keyDigest[:]).Scan(&rows, &rawMatches, &digestMatches); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || rawMatches != 0 || digestMatches != 1 {
		t.Fatalf("rows/raw/digest=%d/%d/%d", rows, rawMatches, digestMatches)
	}
}
