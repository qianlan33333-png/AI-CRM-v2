package p2s03_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	configstore "github.com/qianlan33333-png/AI-CRM-v2/internal/config/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestSettingAuditAndEventAreOneTransactionAndSecretsNeverReachDatabase(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	repository := configstore.NewRepository()

	sentinel := errors.New("event append failed")
	failing := configapp.NewManager(uow, repository, failingAppender{err: sentinel})
	command := configport.SetCommand{
		Key: configport.WeComCorpID, Value: []byte(`"corp-1"`), Actor: "admin:1", RequestID: "request-1",
	}
	if _, err := failing.Set(ctx, command); !errors.Is(err, sentinel) {
		t.Fatalf("Set() with failing event error = %v, want sentinel", err)
	}
	assertCounts(t, ctx, fixture, 0, 0, 0)

	manager := configapp.NewManager(uow, repository, eventstore.NewAppender())
	created, err := manager.Set(ctx, command)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if string(created.Value) != `"corp-1"` {
		t.Fatalf("Set() value = %s, want corp-1", created.Value)
	}
	read, err := manager.Get(ctx, configport.WeComCorpID)
	if err != nil || string(read.Value) != `"corp-1"` {
		t.Fatalf("Get() = %#v, %v", read, err)
	}

	replayed, err := manager.Set(ctx, command)
	if err != nil || string(replayed.Value) != `"corp-1"` {
		t.Fatalf("idempotent Set() = %#v, %v", replayed, err)
	}
	assertCounts(t, ctx, fixture, 1, 1, 1)

	conflict := command
	conflict.Value = []byte(`"corp-2"`)
	if _, err = manager.Set(ctx, conflict); !errors.Is(err, configport.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Set() error = %v, want ErrIdempotencyConflict", err)
	}
	assertCounts(t, ctx, fixture, 1, 1, 1)

	second := conflict
	second.RequestID = "request-2"
	if _, err = manager.Set(ctx, second); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}
	assertCounts(t, ctx, fixture, 1, 2, 2)
	var oldValue string
	if err = fixture.Pool().QueryRow(ctx, `SELECT old_value::text FROM acceptance_fixtures.settings_audit WHERE request_id = 'request-2'`).Scan(&oldValue); err != nil {
		t.Fatalf("query old audit value: %v", err)
	}
	if oldValue != `"corp-1"` {
		t.Fatalf("old audit value = %s, want corp-1", oldValue)
	}

	const secretSentinel = "database-password-sentinel"
	_, err = manager.Set(ctx, configport.SetCommand{
		Key: configport.WeComSecret, Value: []byte(`"` + secretSentinel + `"`),
		Actor: "admin:1", RequestID: "secret-request",
	})
	if !errors.Is(err, configport.ErrSecretSetting) || strings.Contains(err.Error(), secretSentinel) {
		t.Fatalf("secret Set() error = %v", err)
	}
	var databaseText string
	if err = fixture.Pool().QueryRow(ctx, `
SELECT concat(
  coalesce((SELECT string_agg(row_to_json(s)::text, '') FROM acceptance_fixtures.settings s), ''),
  coalesce((SELECT string_agg(row_to_json(a)::text, '') FROM acceptance_fixtures.settings_audit a), ''),
  coalesce((SELECT string_agg(row_to_json(e)::text, '') FROM acceptance_fixtures.event_log e), '')
)`).Scan(&databaseText); err != nil {
		t.Fatalf("scan database text: %v", err)
	}
	if strings.Contains(databaseText, secretSentinel) {
		t.Fatal("secret sentinel reached settings, audit, or event_log")
	}
}

type fixtureUoW struct{ delegate platformport.UnitOfWork }

func (uow fixtureUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return uow.delegate.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `SET LOCAL search_path TO acceptance_fixtures, public`); err != nil {
			return err
		}
		return callback(txCtx)
	})
}

type failingAppender struct{ err error }

func (appender failingAppender) Append(ctx context.Context, _ eventport.Event) (eventport.EventID, error) {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return 0, err
	}
	return 0, appender.err
}

func openFixture(t *testing.T) (*acceptancefixtures.PostgreSQL, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgreSQL() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})
	return fixture, ctx
}

func createTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	ddl := `
CREATE TABLE acceptance_fixtures.settings (
  key text PRIMARY KEY, value jsonb NOT NULL, updated_by text NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE acceptance_fixtures.settings_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, key text NOT NULL, old_value jsonb,
  new_value jsonb NOT NULL, updated_by text NOT NULL, request_id text NOT NULL UNIQUE,
  updated_at timestamptz NOT NULL
);
CREATE TABLE acceptance_fixtures.event_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, event_type text NOT NULL, customer_id bigint,
  payload jsonb NOT NULL DEFAULT '{}', occurred_at timestamptz NOT NULL DEFAULT now(),
  idempotency_key text NOT NULL UNIQUE, dispatched boolean NOT NULL DEFAULT false
);`
	if _, err := fixture.Pool().Exec(ctx, ddl); err != nil {
		t.Fatalf("create acceptance tables: %v", err)
	}
}

func assertCounts(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL, settings, audits, events int) {
	t.Helper()
	var gotSettings, gotAudits, gotEvents int
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.settings`).Scan(&gotSettings); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.settings_audit`).Scan(&gotAudits); err != nil {
		t.Fatalf("count settings audit: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.event_log`).Scan(&gotEvents); err != nil {
		t.Fatalf("count event log: %v", err)
	}
	if gotSettings != settings || gotAudits != audits || gotEvents != events {
		t.Fatalf("settings/audits/events = %d/%d/%d, want %d/%d/%d", gotSettings, gotAudits, gotEvents, settings, audits, events)
	}
}
