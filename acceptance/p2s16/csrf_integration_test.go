package p2s16

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCSRFIsBoundToCurrentActiveSessionOnRealPostgreSQL(t *testing.T) {
	fixture, ctx := openFixture(t)
	createAuthTables(t, ctx, fixture)
	seedAdmin(t, ctx, fixture)

	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	random := bytes.NewReader(append(
		append(append(bytes.Repeat([]byte{0x31}, 32), bytes.Repeat([]byte{0x32}, 32)...), bytes.Repeat([]byte{0x33}, 32)...),
		bytes.Repeat([]byte{0x34}, 32)...,
	))
	service, err := authapp.NewService(
		fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())},
		authstore.NewRepository(),
		authapp.Options{Clock: func() time.Time { return now }, Random: random},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	first := issueSession(t, ctx, service)
	second := issueSession(t, ctx, service)

	if err = service.ValidateCSRF(ctx, first.Session, first.CSRF); err != nil {
		t.Fatalf("ValidateCSRF(active session) error = %v", err)
	}
	assertCSRFInvalid(t, service.ValidateCSRF(ctx, first.Session, second.CSRF))
	assertCSRFInvalid(t, service.ValidateCSRF(ctx, second.Session, first.CSRF))

	firstHash := sha256.Sum256([]byte(first.Session))
	if _, err = fixture.Pool().Exec(ctx, `
UPDATE acceptance_fixtures.admin_sessions
SET revoked_at = $1
WHERE session_token_hash = $2`, now, firstHash[:]); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	assertCSRFInvalid(t, service.ValidateCSRF(ctx, first.Session, first.CSRF))

	secondHash := sha256.Sum256([]byte(second.Session))
	if _, err = fixture.Pool().Exec(ctx, `
UPDATE acceptance_fixtures.admin_sessions
SET expires_at = $1
WHERE session_token_hash = $2`, now.Add(-time.Second), secondHash[:]); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	assertCSRFInvalid(t, service.ValidateCSRF(ctx, second.Session, second.CSRF))

	if _, err = fixture.Pool().Exec(ctx, `DROP TABLE acceptance_fixtures.admin_sessions`); err != nil {
		t.Fatalf("drop session table: %v", err)
	}
	if err = service.ValidateCSRF(ctx, first.Session, first.CSRF); !errors.Is(err, authport.ErrAuthenticationUnavailable) {
		t.Fatalf("ValidateCSRF(database error) = %v, want ErrAuthenticationUnavailable", err)
	}
}

type fixtureUoW struct{ delegate platformport.UnitOfWork }

func (uow fixtureUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return uow.delegate.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `SET LOCAL search_path TO acceptance_fixtures, pg_catalog`); err != nil {
			return err
		}
		return callback(txCtx)
	})
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

func createAuthTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.admin_users (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  auth_provider text NOT NULL,
  provider_tenant_id text NOT NULL,
  provider_subject_id text NOT NULL,
  display_name text NOT NULL,
  role text NOT NULL,
  staff_id bigint,
  is_active boolean NOT NULL,
  login_enabled boolean NOT NULL,
  session_version bigint NOT NULL,
  UNIQUE (auth_provider, provider_tenant_id, provider_subject_id)
);
CREATE TABLE acceptance_fixtures.admin_sessions (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_token_hash bytea NOT NULL UNIQUE,
  csrf_token_hash bytea NOT NULL,
  admin_user_id bigint NOT NULL REFERENCES acceptance_fixtures.admin_users(id),
  session_version bigint NOT NULL,
  auth_time timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  revoked_reason text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);`)
	if err != nil {
		t.Fatalf("create auth fixtures: %v", err)
	}
}

func seedAdmin(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.admin_users (
  auth_provider, provider_tenant_id, provider_subject_id, display_name,
  role, staff_id, is_active, login_enabled, session_version
) VALUES ('wecom', 'corp-fixture', 'user-fixture', 'Fixture Admin', 'admin', 42, true, true, 1)`)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
}

func issueSession(t *testing.T, ctx context.Context, service *authapp.Service) authport.BrowserSession {
	t.Helper()
	session, err := service.IssueVerified(ctx, authport.VerifiedLogin{
		Provider: authport.ProviderWeCom, TenantID: "corp-fixture", SubjectID: "user-fixture",
	})
	if err != nil {
		t.Fatalf("IssueVerified() error = %v", err)
	}
	return session
}

func assertCSRFInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, authport.ErrCSRFInvalid) {
		t.Fatalf("ValidateCSRF() error = %v, want ErrCSRFInvalid", err)
	}
}
