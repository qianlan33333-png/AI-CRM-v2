package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var outboundDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 outbound database")

func TestOutboundStorageCatalogWaterlineAndIdentity(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()

	var waterline int
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&waterline); err != nil || waterline != 14 {
		t.Fatalf("migration waterline=%d err=%v, want 14", waterline, err)
	}

	var identity, generation string
	if err := pool.QueryRow(ctx, `
SELECT is_identity, identity_generation
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'outbound_tasks' AND column_name = 'id'`).Scan(&identity, &generation); err != nil || identity != "YES" || generation != "ALWAYS" {
		t.Fatalf("outbound_tasks.id identity=%q generation=%q err=%v, want YES/ALWAYS", identity, generation, err)
	}

	for _, forbidden := range []string{"accepted_event_id", "river_job_id"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = 'public' AND table_name = 'outbound_tasks' AND column_name = $1::text
)`, forbidden).Scan(&exists); err != nil || exists {
			t.Fatalf("outbound_tasks forbidden column=%q exists=%t err=%v", forbidden, exists, err)
		}
	}
}

func TestAcceptOneCommitsDBGeneratedTaskAndAcceptedEvent(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()
	customerID := createOutboundCustomer(t, ctx, pool)
	sequenceStart := time.Now().UnixNano()
	if _, err := pool.Exec(ctx, `SELECT setval(pg_get_serial_sequence('outbound_tasks', 'id'), $1::bigint, true)`, sequenceStart); err != nil {
		t.Fatal(err)
	}

	repository := outboundstore.NewRepository()
	service := outboundapp.NewAcceptOneService(platformstore.NewUnitOfWork(pool), repository, eventstore.NewAppender())
	accepted, err := service.Accept(ctx, outboundapp.OneCommand{
		CustomerID:  customerID,
		TemplateKey: outboundapp.TemplateTextNoticeV1,
		Payload:     json.RawMessage(`{"text":"accepted only"}`),
	})
	if err != nil || accepted.TaskID != outboundapp.TaskID(sequenceStart+1) || accepted.EventID < 1 {
		t.Fatalf("Accept()=%+v err=%v, want database-generated task %d and event", accepted, err, sequenceStart+1)
	}

	var taskCustomerID int64
	var templateKey string
	var payload []byte
	if err = pool.QueryRow(ctx, `
SELECT customer_id, template_key, payload
FROM outbound_tasks
WHERE id = $1::bigint`, accepted.TaskID).Scan(&taskCustomerID, &templateKey, &payload); err != nil {
		t.Fatal(err)
	}
	if taskCustomerID != customerID || templateKey != outboundapp.TemplateTextNoticeV1 || !equalJSON(payload, []byte(`{"text":"accepted only"}`)) {
		t.Fatalf("task=%d/%q/%s, want persisted task", taskCustomerID, templateKey, payload)
	}

	var eventType, eventKey string
	var eventCustomerID *int64
	var eventPayload []byte
	if err = pool.QueryRow(ctx, `
SELECT event_type, customer_id, payload, idempotency_key
FROM event_log
WHERE id = $1::bigint`, accepted.EventID).Scan(&eventType, &eventCustomerID, &eventPayload, &eventKey); err != nil {
		t.Fatal(err)
	}
	wantPayload := fmt.Sprintf(`{"task_id":%d}`, accepted.TaskID)
	if eventType != eventport.EvOutboundAccepted || eventCustomerID == nil || *eventCustomerID != customerID || !equalJSON(eventPayload, []byte(wantPayload)) || eventKey != fmt.Sprintf("outbound.accepted:%d", accepted.TaskID) {
		t.Fatalf("event type=%q customer=%v payload=%s key=%q, want accepted event for task", eventType, eventCustomerID, eventPayload, eventKey)
	}
}

func TestAcceptOneRollsBackTaskWhenAcceptedEventCannotAppend(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()
	var eventsBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	service := outboundapp.NewAcceptOneService(platformstore.NewUnitOfWork(pool), outboundstore.NewRepository(), failingAppender{})
	_, err := service.Accept(ctx, outboundapp.OneCommand{
		CustomerID:  createOutboundCustomer(t, ctx, pool),
		TemplateKey: outboundapp.TemplateTextNoticeV1,
		Payload:     json.RawMessage(`{"text":"rollback"}`),
	})
	if !errors.Is(err, errAppendRejected) {
		t.Fatalf("Accept() error=%v, want %v", err, errAppendRejected)
	}

	var tasks, eventsAfter int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbound_tasks), (SELECT count(*) FROM event_log)`).Scan(&tasks, &eventsAfter); err != nil || tasks != 0 || eventsAfter != eventsBefore {
		t.Fatalf("rollback facts tasks/events=%d/%d err=%v, want 0/%d", tasks, eventsAfter, err, eventsBefore)
	}
}

var errAppendRejected = errors.New("accepted event append rejected")

type failingAppender struct{}

func (failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errAppendRejected
}

func openOutboundPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *outboundDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*outboundDatabaseURL); err != nil {
		t.Fatalf("unsafe outbound database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *outboundDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetOutboundFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE outbound_tasks RESTART IDENTITY`); err != nil {
		t.Fatalf("reset outbound fixture: %v", err)
	}
}

func createOutboundCustomer(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}
