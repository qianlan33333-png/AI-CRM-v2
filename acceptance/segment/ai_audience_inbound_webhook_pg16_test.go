package segment_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	legacyaudience "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
	legacyaudiencestore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience/store"
)

func TestAIAudienceInboundWebhookPG16(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated migrated PG16 test")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(dsn, acceptancefixtures.Audience101DatabaseName); err != nil {
		if errors.Is(err, acceptancefixtures.ErrUnsafeDatabaseURL) {
			t.Skip("Audience 101 PG16 tests require the isolated Audience 101 database")
		}
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("server_version_num=%q err=%v", version, err)
	}
	packageID := insertInboundWebhookPackage(t, ctx, pool)
	repository, err := legacyaudiencestore.NewInboundWebhookRepository(legacyaudiencestore.NewInboundWebhookQueryFactory())
	if err != nil {
		t.Fatal(err)
	}
	service, err := legacyaudience.NewInboundWebhookService(
		platformstore.NewUnitOfWork(pool), repository, inboundWebhookEventAppender{appender: eventstore.NewAppender()},
	)
	if err != nil {
		t.Fatal(err)
	}
	serviceInput := inboundWebhookPG16Input(packageID, "business-event-0001", "transport-event-0001", []byte(`{"payload":1}`))
	first, err := service.Accept(ctx, serviceInput)
	if err != nil || first.Replayed || first.Receipt.ID < 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := service.Accept(ctx, serviceInput)
	if err != nil || !replay.Replayed || replay.Receipt.ID != first.Receipt.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	changedPayload := serviceInput
	changedPayload.PayloadDigest = sha256.Sum256([]byte(`{"payload":2}`))
	if _, err = service.Accept(ctx, changedPayload); !errors.Is(err, legacyaudience.ErrIdempotencyConflict) {
		t.Fatalf("payload conflict err=%v", err)
	}
	transportConflict := inboundWebhookPG16Input(packageID, "business-event-0002", serviceInput.TransportEventID, []byte(`{"payload":3}`))
	if _, err = service.Accept(ctx, transportConflict); !errors.Is(err, legacyaudience.ErrIdempotencyConflict) {
		t.Fatalf("transport conflict err=%v", err)
	}
	unknown := inboundWebhookPG16Input(packageID+100000, "business-event-unknown", "transport-event-unknown", []byte(`{"payload":4}`))
	if _, err = service.Accept(ctx, unknown); !errors.Is(err, legacyaudience.ErrNotFound) {
		t.Fatalf("unknown package err=%v", err)
	}
	failing, err := legacyaudience.NewInboundWebhookService(
		platformstore.NewUnitOfWork(pool), repository, inboundWebhookFailingAppender{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = failing.Accept(ctx, inboundWebhookPG16Input(packageID, "business-event-rollback", "transport-event-rollback", []byte(`{"payload":5}`))); err == nil {
		t.Fatal("event append failure committed receipt")
	}
	var receipts, replays, events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_inbound_webhook_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_webhook_transport_replays`).Scan(&replays); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.event_log WHERE event_type = 'ai_audience.inbound_webhook.received'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || replays != 1 || events != 1 {
		t.Fatalf("receipt/replay/event=%d/%d/%d", receipts, replays, events)
	}
}

type inboundWebhookEventAppender struct{ appender *eventstore.Appender }

func (adapter inboundWebhookEventAppender) Append(ctx context.Context, event legacyaudience.LocalEvent) error {
	_, err := adapter.appender.Append(ctx, eventport.Event{Type: event.Type, Payload: event.Payload, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey})
	return err
}

type inboundWebhookFailingAppender struct{}

func (inboundWebhookFailingAppender) Append(context.Context, legacyaudience.LocalEvent) error {
	return errors.New("forced inbound webhook event failure")
}

func inboundWebhookPG16Input(packageID int64, externalEventID, transportEventID string, payload []byte) legacyaudience.InboundWebhookInput {
	return legacyaudience.InboundWebhookInput{
		PackageID: packageID, ClientID: legacyaudience.AIAudienceWebhookClientID, TransportEventID: transportEventID,
		ExternalEventID: externalEventID, Status: "received", Message: json.RawMessage(`{}`), Action: json.RawMessage(`{}`),
		PayloadDigest: sha256.Sum256(payload),
	}
}

func insertInboundWebhookPackage(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var packageID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO public.segments (name, definition, refresh_mode, member_count, refresh_status)
VALUES ('audience-inbound-webhook-pg16', '{}'::jsonb, 'manual', 0, 'idle')
RETURNING id`).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO public.ai_audience_package_metadata (segment_id, lifecycle, version, created_by, updated_by)
VALUES ($1, 'active', 1, 0, 0)`, packageID); err != nil {
		t.Fatal(err)
	}
	return packageID
}
