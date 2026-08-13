package wecom

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

func TestExternalContactCursorResumePersistsAndCompletesWithoutDuplicatePages(t *testing.T) {
	dsn := os.Getenv("WECOM_SYNC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WECOM_SYNC_TEST_DATABASE_URL is not configured")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	reader := &fixtureReader{pages: map[string]wecomclient.ExternalContactPage{
		"":       {ExternalUserIDs: []string{"wo-1", "wo-2"}, NextCursor: "page-2"},
		"page-2": {ExternalUserIDs: []string{"wo-3"}},
	}}
	staffUserID := fmt.Sprintf("w4-acceptance-%d", time.Now().UnixNano())
	repository := wecomstore.NewSyncStateRepository(pool)
	service := wecomapp.NewExternalContactSyncService(platformstore.NewUnitOfWork(pool), reader, repository)
	first, err := service.SyncNext(ctx, staffUserID)
	if err != nil || !reflect.DeepEqual(first.ExternalUserIDs, []string{"wo-1", "wo-2"}) {
		t.Fatalf("first SyncNext() = %#v, %v", first, err)
	}

	// Constructing a fresh service simulates process interruption and restart.
	restarted := wecomapp.NewExternalContactSyncService(platformstore.NewUnitOfWork(pool), reader, repository)
	second, err := restarted.SyncNext(ctx, staffUserID)
	if err != nil || !reflect.DeepEqual(second.ExternalUserIDs, []string{"wo-3"}) {
		t.Fatalf("second SyncNext() = %#v, %v", second, err)
	}
	if _, err = restarted.SyncNext(ctx, staffUserID); !errors.Is(err, wecomapp.ErrCursorSyncDone) {
		t.Fatalf("completed SyncNext() error = %v, want done", err)
	}
	if !reflect.DeepEqual(reader.cursors, []string{"", "page-2"}) {
		t.Fatalf("provider cursors = %#v, want no duplicated page", reader.cursors)
	}

	key := "external_contact_list:" + staffUserID
	var cursor string
	var completed bool
	if err = pool.QueryRow(ctx, `SELECT cursor, completed_at IS NOT NULL FROM wecom_sync_state WHERE sync_key = $1`, key).Scan(&cursor, &completed); err != nil {
		t.Fatal(err)
	}
	if cursor != "" || !completed {
		t.Fatalf("persisted state cursor=%q completed=%t, want terminal cursor", cursor, completed)
	}
}

type fixtureReader struct {
	pages   map[string]wecomclient.ExternalContactPage
	cursors []string
}

func (reader *fixtureReader) ListExternalContacts(_ context.Context, _ string, cursor string) (wecomclient.ExternalContactPage, error) {
	reader.cursors = append(reader.cursors, cursor)
	page, ok := reader.pages[cursor]
	if !ok {
		return wecomclient.ExternalContactPage{}, errors.New("unexpected fixture cursor")
	}
	return page, nil
}
