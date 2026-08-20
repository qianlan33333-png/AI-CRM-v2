package wecom

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

var p4MessageArchiveDatabaseURL = flag.String("p4-message-archive-database-url", "", "isolated PostgreSQL 16.14 P4 message archive database")

func TestP4MessageArchiveNormalBoundaryOwnershipAndAcceptedUoW(t *testing.T) {
	pool, ctx := openMessageArchivePool(t)
	prefix := fmt.Sprintf("p4-message-archive-%d", time.Now().UnixNano())
	seed := time.Now().UnixNano() % 1_000_000_000
	firstCustomer, secondCustomer := int64(8_801_000_000+seed), int64(8_802_000_000+seed)
	now := time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)
	for index, customerID := range []int64{firstCustomer, secondCustomer} {
		_, err := pool.Exec(ctx, `INSERT INTO wecom_message_archive_records
      (source_message_id,customer_id,external_userid,chat_type,owner_userid,sender,receiver,message_type,content_masked,sent_at)
      VALUES ($1,$2,$3,'private','staff-archive','external-archive','staff-archive','text','phone [masked-phone]',$4)`,
			fmt.Sprintf("%s-%d", prefix, index), customerID, fmt.Sprintf("external-%d", index), now.Add(time.Duration(index)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := pool.Exec(ctx, `INSERT INTO wecom_message_archive_records
      (source_message_id,customer_id,external_userid,chat_type,owner_userid,sender,receiver,chat_id,roomid,group_name,message_type,content_masked,sent_at)
      VALUES ($1,$2,'external-group-hidden','group','','','','chat-local','room-local','local group','image','group body must stay hidden',$3)`,
		prefix+"-group", firstCustomer, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM wecom_message_archive_sync_receipts WHERE idempotency_scope='admin:8801' AND idempotency_key=$1`, prefix+"-accepted-command")
		_, _ = pool.Exec(context.Background(), `DELETE FROM wecom_message_archive_records WHERE source_message_id LIKE $1`, prefix+"%")
	})

	events := &archiveAcceptanceAppender{}
	service := wecomapp.NewMessageArchiveService(platformstore.NewUnitOfWork(pool), wecomstore.NewMessageArchiveRepository(), events)
	safeAll, err := service.ListCustomerChatSummaries(ctx, wecomport.CustomerChatSummaryQuery{CustomerID: contactport.CustomerID(firstCustomer), Limit: 1})
	if err != nil || safeAll.Total != 2 || len(safeAll.Items) != 1 || safeAll.Items[0].ChatType != "group" || safeAll.Items[0].MessageType != "image" {
		t.Fatalf("safe all page=%#v err=%v", safeAll, err)
	}
	safePrivate, err := service.ListCustomerChatSummaries(ctx, wecomport.CustomerChatSummaryQuery{CustomerID: contactport.CustomerID(firstCustomer), ChatType: "private", Limit: 20})
	if err != nil || safePrivate.Total != 1 || len(safePrivate.Items) != 1 || safePrivate.Items[0].ChatType != "private" {
		t.Fatalf("safe private page=%#v err=%v", safePrivate, err)
	}
	items, total, err := service.List(ctx, wecomapp.ArchiveQuery{CustomerID: contactport.CustomerID(firstCustomer), ChatType: "private", Keyword: "masked", Limit: 20})
	if err != nil || total != 1 || len(items) != 1 || items[0].ExternalUserID != "external-0" || items[0].Content != "phone [masked-phone]" {
		t.Fatalf("owned list items=%#v total=%d err=%v", items, total, err)
	}
	startedAt := now.Add(-time.Second)
	external, total, err := service.List(ctx, wecomapp.ArchiveQuery{CustomerID: contactport.CustomerID(firstCustomer), ChatType: "private", WithUserID: "staff-archive", StartedAt: &startedAt, Limit: 20, External: true})
	if err != nil || total != 1 || len(external) != 1 || external[0].ID != items[0].ID {
		t.Fatalf("external list items=%#v total=%d err=%v", external, total, err)
	}
	if _, _, err = service.List(ctx, wecomapp.ArchiveQuery{CustomerID: contactport.CustomerID(firstCustomer), ChatType: "private", Limit: 201}); !errors.Is(err, wecomapp.ErrInvalidMessageArchiveQuery) {
		t.Fatalf("limit boundary error=%v", err)
	}

	command := wecomapp.ArchiveSyncCommand{Actor: "admin:8801", IdempotencyKey: prefix + "-accepted-command", StartTime: "2000-01-01 00:00:00", EndTime: "2099-12-31 23:59:59", Limit: 100, MaxPages: 1000}
	accepted, err := service.RequestSync(ctx, command)
	if err != nil || accepted.State != wecomapp.ArchiveSyncAccepted || accepted.EventID <= 0 {
		t.Fatalf("accepted sync=%#v err=%v", accepted, err)
	}
	replayed, err := service.RequestSync(ctx, command)
	if err != nil || replayed != accepted {
		t.Fatalf("accepted replay=%#v err=%v", replayed, err)
	}
	changed := command
	changed.Cursor = "different-command"
	if _, err = service.RequestSync(ctx, changed); !errors.Is(err, wecomapp.ErrArchiveSyncConflict) {
		t.Fatalf("idempotency conflict error=%v", err)
	}
	var receipts, providers int
	err = pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM wecom_message_archive_sync_receipts WHERE id=$1 AND state='accepted' AND accepted_event_id=$2),
      0`, accepted.ID, accepted.EventID).Scan(&receipts, &providers)
	if err != nil || receipts != 1 || events.calls != 1 || events.event.Type != "wecom.message_archive_sync_accepted" || !events.transactionBound || providers != 0 {
		t.Fatalf("accepted UoW receipt/event-port/provider=%d/%d/%t/%d err=%v", receipts, events.calls, events.transactionBound, providers, err)
	}
}

type archiveAcceptanceAppender struct {
	calls            int
	event            eventport.Event
	transactionBound bool
}

func (appender *archiveAcceptanceAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	appender.calls++
	appender.event = event
	_, err := platformstore.TxFromContext(ctx)
	appender.transactionBound = err == nil
	if err != nil {
		return 0, err
	}
	return 8_801_003, nil
}

func TestP4MessageArchiveStorageContractHasNoTenantOrCrossDomainFK(t *testing.T) {
	pool, ctx := openMessageArchivePool(t)
	var waterline, constraints, invalidConstraints, indexes, invalidIndexes, crossDomainFK, tenantColumns int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass)),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND contype='f'),
      (SELECT count(*) FROM information_schema.columns WHERE table_name IN ('wecom_message_archive_records','wecom_message_archive_sync_receipts') AND column_name ILIKE '%tenant%')`).Scan(
		&waterline, &constraints, &invalidConstraints, &indexes, &invalidIndexes, &crossDomainFK, &tenantColumns,
	)
	if err != nil || waterline != 40 || constraints < 15 || invalidConstraints != 0 || indexes < 4 || invalidIndexes != 0 || crossDomainFK != 0 || tenantColumns != 0 {
		t.Fatalf("catalog waterline/constraints/invalid/indexes/invalid/fk/tenant/error=%d/%d/%d/%d/%d/%d/%d/%v", waterline, constraints, invalidConstraints, indexes, invalidIndexes, crossDomainFK, tenantColumns, err)
	}
}

func openMessageArchivePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *p4MessageArchiveDatabaseURL == "" {
		t.Skip("p4-message-archive-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*p4MessageArchiveDatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *p4MessageArchiveDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}
