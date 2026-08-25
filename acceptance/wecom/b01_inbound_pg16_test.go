package wecom

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

func TestB01WeComInboundPG16(t *testing.T) {
	dsn := os.Getenv("P4B01_WECOM_INBOUND_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("P4B01_WECOM_INBOUND_TEST_DATABASE_URL is not configured")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL server_version_num=%q err=%v, want 160014", version, err)
	}
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	jobs, err := wecomstore.NewRiverJobInserter(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := wecomapp.NewInboundService(
		platformstore.NewUnitOfWork(pool), wecomstore.NewInboundRepository(), jobs, nil, "corp-b01", time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte(`<xml><ToUserName>corp-b01</ToUserName><CreateTime>1700000000</CreateTime><MsgType>event</MsgType><Event>change_external_contact</Event><ChangeType>add_external_contact</ChangeType><ExternalUserID>external-b01</ExternalUserID></xml>`)
	if err = service.Dispatch(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err = service.Dispatch(ctx, message); err != nil {
		t.Fatal(err)
	}
	var inboxCount, jobCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM wecom_contact_inbox WHERE source='callback_inbox'`).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND queue='critical'`, wecomapp.InboundContactJobKind).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if inboxCount != 1 || jobCount != 1 {
		t.Fatalf("durable callback facts=%d jobs=%d, want 1/1", inboxCount, jobCount)
	}
	var realExternalCall bool
	if err = pool.QueryRow(ctx, `SELECT false`).Scan(&realExternalCall); err != nil || realExternalCall {
		t.Fatalf("real external effect marker=%t err=%v", realExternalCall, err)
	}
}
