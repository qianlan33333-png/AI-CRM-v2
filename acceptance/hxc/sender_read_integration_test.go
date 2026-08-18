package hxc_acceptance

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcstore "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store"
)

var hxcDatabaseURL = flag.String("database-url", "", "PostgreSQL URL for HXC sender acceptance")
var hxcSequence atomic.Int64

func TestHXCSenderReadPostgreSQLMergeAndUnavailable(t *testing.T) {
	pool, ctx := openPool(t)
	marker := fmt.Sprintf("hxc-acceptance-%d-%d", time.Now().UnixNano(), hxcSequence.Add(1))
	activeUserID := marker + "-active"
	fallbackUserID := marker + "-fallback"
	inactiveUserID := marker + "-inactive"
	orphanUserID := marker + "-orphan"
	now := time.Now().UTC().Truncate(time.Microsecond)
	whitespaceUserID := strings.Repeat(" ", 64+int(now.UnixNano()%31))

	fixtureTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, staff := range []struct {
		userID    string
		name      string
		active    bool
		updatedAt time.Time
	}{
		{fallbackUserID, "", true, now.Add(-time.Second)},
		{activeUserID, "staff name", true, now},
		{inactiveUserID, "excluded", false, now.Add(time.Second)},
		{whitespaceUserID, "excluded whitespace", true, now.Add(2 * time.Second)},
	} {
		if _, err := contactfixture.CreateStaffWithDetails(ctx, fixtureTx, staff.userID, staff.name, staff.active, staff.updatedAt); err != nil {
			_ = fixtureTx.Rollback(ctx)
			t.Fatal(err)
		}
	}
	if err := fixtureTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, config := range []struct {
		id       string
		userID   string
		name     string
		priority int
		active   bool
	}{
		{marker + "-one", activeUserID, "configured name", 8, true},
		{marker + "-two", fallbackUserID, "", 4, false},
		{marker + "-three", orphanUserID, "orphan config", 2, true},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO hxc_sender_configs (id,sender_userid,display_name,priority,is_active,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$6)`, config.id, config.userID, config.name, config.priority, config.active, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []struct {
		id     string
		userID string
	}{
		{" " + marker + "-padded-id", marker + "-padded-id"},
		{marker + "-padded-user", " " + marker + "-padded-user"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO hxc_sender_configs (id,sender_userid,created_at,updated_at) VALUES ($1,$2,$3,$3)`, invalid.id, invalid.userID, now); err == nil {
			t.Fatalf("padded HXC sender config was accepted: %+v", invalid)
		}
	}

	var beforeEffects, afterEffects struct {
		events, deliveries, jobLinks, providerEffects, providerState int
	}
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM event_log),
		(SELECT count(*) FROM event_deliveries),
		(SELECT count(*) FROM outbound_task_job_links),
		(SELECT count(*) FROM order_external_effects),
		(SELECT count(*) FROM wecom_sync_state)`).
		Scan(&beforeEffects.events, &beforeEffects.deliveries, &beforeEffects.jobLinks, &beforeEffects.providerEffects, &beforeEffects.providerState); err != nil {
		t.Fatal(err)
	}
	reader := hxcapp.Reader{Staff: contactstore.NewStaffDirectoryRepository(pool), Configs: hxcstore.NewSenderConfigRepository(pool)}
	projection, err := reader.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, activeOK := hxcCandidate(projection, activeUserID)
	fallback, fallbackOK := hxcCandidate(projection, fallbackUserID)
	orphan, orphanOK := hxcCandidate(projection, orphanUserID)
	if len(projection.SendConfigs) != 3 || projection.SendConfigs[0].SenderUserID != orphanUserID ||
		!activeOK || active.DisplayName != "configured name" || !active.IsSender || !active.IsActive ||
		!fallbackOK || fallback.DisplayName != fallbackUserID || !fallback.IsSender || fallback.IsActive ||
		!orphanOK || orphan.DisplayName != "orphan config" || !orphan.IsSender || orphan.IsActive ||
		projectionHasHXCCandidate(projection, whitespaceUserID) ||
		projection.ActiveSenderCount != 1 || projection.LastSyncedAt.Before(now) {
		t.Fatalf("projection=%#v", projection)
	}
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM event_log),
		(SELECT count(*) FROM event_deliveries),
		(SELECT count(*) FROM outbound_task_job_links),
		(SELECT count(*) FROM order_external_effects),
		(SELECT count(*) FROM wecom_sync_state)`).
		Scan(&afterEffects.events, &afterEffects.deliveries, &afterEffects.jobLinks, &afterEffects.providerEffects, &afterEffects.providerState); err != nil || afterEffects != beforeEffects {
		t.Fatalf("read changed event/job/provider facts: before=%+v after=%+v err=%v", beforeEffects, afterEffects, err)
	}

	closedPool, err := pgxpool.New(ctx, *hxcDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	closedPool.Close()
	unavailable := hxcapp.Reader{Staff: contactstore.NewStaffDirectoryRepository(closedPool), Configs: hxcstore.NewSenderConfigRepository(closedPool)}
	if _, err := unavailable.Read(ctx); err == nil {
		t.Fatal("closed PostgreSQL pool unexpectedly produced an HXC sender projection")
	}
}

func hxcCandidate(projection hxcapp.Projection, userID string) (hxcapp.Candidate, bool) {
	for _, candidate := range projection.Directory {
		if candidate.WeComUserID == userID {
			return candidate, true
		}
	}
	return hxcapp.Candidate{}, false
}

func projectionHasHXCCandidate(projection hxcapp.Projection, userID string) bool {
	_, ok := hxcCandidate(projection, userID)
	return ok
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *hxcDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *hxcDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}
