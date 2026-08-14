package p2s09

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestBrowserSessionLifecycleOnRealPostgreSQL(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	seedAdmin(t, ctx, fixture)

	now := time.Now().UTC().Truncate(time.Second)
	clock := func() time.Time { return now }
	random := bytes.NewReader(append(bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)...))
	service, err := authapp.NewService(
		fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())},
		authstore.NewRepository(),
		authapp.Options{Clock: clock, Random: random},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	session, err := service.IssueVerified(ctx, authport.VerifiedLogin{
		Provider: authport.ProviderWeCom, CorpID: "corp-fixture", SubjectID: "user-fixture",
	})
	if err != nil {
		t.Fatalf("IssueVerified() error = %v", err)
	}
	if len(session.Session) != 43 || len(session.CSRF) != 43 {
		t.Fatalf("issued token lengths = %d/%d, want 43/43", len(session.Session), len(session.CSRF))
	}
	assertOnlyHashesPersisted(t, ctx, fixture, session)

	handler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	protectedAPI := handler.Authenticate(generated.Handler(handler))
	cookieRecorder := httptest.NewRecorder()
	if err = authhttp.WriteBrowserSession(cookieRecorder, session); err != nil {
		t.Fatalf("WriteBrowserSession() error = %v", err)
	}
	assertSecureCookies(t, cookieRecorder.Result().Cookies())

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	getRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: string(session.Session)})
	getRecorder := httptest.NewRecorder()
	protectedAPI.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET session status = %d, body=%s", getRecorder.Code, getRecorder.Body.String())
	}
	var principal generated.AuthSessionResponse
	if err = json.NewDecoder(getRecorder.Body).Decode(&principal); err != nil {
		t.Fatalf("decode principal: %v", err)
	}
	if principal.AdminUserId != 1 || principal.Role != generated.Admin || principal.StaffId == nil || *principal.StaffId != 42 {
		t.Fatalf("principal = %+v", principal)
	}

	missingCSRFRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	missingCSRFRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: string(session.Session)})
	missingCSRFRecorder := httptest.NewRecorder()
	protectedAPI.ServeHTTP(missingCSRFRecorder, missingCSRFRequest)
	if missingCSRFRecorder.Code != http.StatusBadRequest {
		t.Fatalf("logout without CSRF status = %d, want 400", missingCSRFRecorder.Code)
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: string(session.Session)})
	logoutRequest.Header.Set("X-CSRF-Token", string(session.CSRF))
	logoutRecorder := httptest.NewRecorder()
	protectedAPI.ServeHTTP(logoutRecorder, logoutRequest)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
	assertClearedCookies(t, logoutRecorder.Result().Cookies())
	if _, err = service.Authenticate(ctx, session.Session); !errors.Is(err, authport.ErrUnauthenticated) {
		t.Fatalf("Authenticate(after logout) error = %v, want ErrUnauthenticated", err)
	}
}

func TestSessionFailsClosedForCSRFVersionAndDisabledUser(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	seedAdmin(t, ctx, fixture)

	now := time.Now().UTC().Truncate(time.Second)
	service, err := authapp.NewService(
		fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())},
		authstore.NewRepository(),
		authapp.Options{Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x33}, 64))},
	)
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.IssueVerified(ctx, authport.VerifiedLogin{Provider: authport.ProviderWeCom, CorpID: "corp-fixture", SubjectID: "user-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Invalidate(ctx, session.Session, authport.CSRFToken(strings.Repeat("A", 43))); !errors.Is(err, authport.ErrCSRFInvalid) {
		t.Fatalf("wrong CSRF error = %v, want ErrCSRFInvalid", err)
	}
	if _, err = service.Authenticate(ctx, session.Session); err != nil {
		t.Fatalf("wrong CSRF revoked the session: %v", err)
	}
	if _, err = fixture.Pool().Exec(ctx, `UPDATE acceptance_fixtures.admin_users SET session_version = session_version + 1`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, session.Session); !errors.Is(err, authport.ErrUnauthenticated) {
		t.Fatalf("version mismatch error = %v, want ErrUnauthenticated", err)
	}
	if _, err = fixture.Pool().Exec(ctx, `UPDATE acceptance_fixtures.admin_users SET session_version = 1, is_active = false`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, session.Session); !errors.Is(err, authport.ErrUnauthenticated) {
		t.Fatalf("disabled user error = %v, want ErrUnauthenticated", err)
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
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.admin_users (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  auth_provider text NOT NULL,
  wecom_corp_id text NOT NULL,
  provider_subject_id text NOT NULL,
  display_name text NOT NULL,
  role text NOT NULL,
  staff_id bigint,
  is_active boolean NOT NULL,
  login_enabled boolean NOT NULL,
  session_version bigint NOT NULL,
  UNIQUE (auth_provider, wecom_corp_id, provider_subject_id)
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
		t.Fatalf("create acceptance tables: %v", err)
	}
}

func seedAdmin(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.admin_users (
  auth_provider, wecom_corp_id, provider_subject_id, display_name,
  role, staff_id, is_active, login_enabled, session_version
) VALUES ('wecom', 'corp-fixture', 'user-fixture', 'Fixture Admin', 'admin', 42, true, true, 1)`)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
}

func assertOnlyHashesPersisted(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL, session authport.BrowserSession) {
	t.Helper()
	var sessionHash, csrfHash []byte
	if err := fixture.Pool().QueryRow(ctx, `SELECT session_token_hash, csrf_token_hash FROM acceptance_fixtures.admin_sessions`).Scan(&sessionHash, &csrfHash); err != nil {
		t.Fatalf("scan stored hashes: %v", err)
	}
	if len(sessionHash) != 32 || len(csrfHash) != 32 || bytes.Equal(sessionHash, []byte(session.Session)) || bytes.Equal(csrfHash, []byte(session.CSRF)) {
		t.Fatal("database did not contain only fixed-length token hashes")
	}
	var rawCount int
	if err := fixture.Pool().QueryRow(ctx, `
SELECT count(*) FROM acceptance_fixtures.admin_sessions
WHERE encode(session_token_hash, 'escape') = $1 OR encode(csrf_token_hash, 'escape') = $2`, string(session.Session), string(session.CSRF)).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 0 {
		t.Fatal("raw browser bearer material reached PostgreSQL")
	}
}

func assertSecureCookies(t *testing.T, cookies []*http.Cookie) {
	t.Helper()
	if len(cookies) != 2 {
		t.Fatalf("cookie count = %d, want 2", len(cookies))
	}
	byName := map[string]*http.Cookie{cookies[0].Name: cookies[0], cookies[1].Name: cookies[1]}
	session := byName[authhttp.SessionCookieName]
	csrf := byName[authhttp.CSRFCookieName]
	if session == nil || !session.Secure || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.Path != "/" {
		t.Fatalf("session cookie flags = %+v", session)
	}
	if csrf == nil || !csrf.Secure || csrf.HttpOnly || csrf.SameSite != http.SameSiteStrictMode || csrf.Path != "/" {
		t.Fatalf("csrf cookie flags = %+v", csrf)
	}
}

func assertClearedCookies(t *testing.T, cookies []*http.Cookie) {
	t.Helper()
	if len(cookies) != 2 {
		t.Fatalf("cleared cookie count = %d, want 2", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 || cookie.Value != "" || !cookie.Secure || cookie.Path != "/" {
			t.Fatalf("cookie was not securely cleared: %+v", cookie)
		}
	}
}
