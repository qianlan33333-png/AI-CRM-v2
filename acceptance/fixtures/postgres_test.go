package fixtures

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenPostgreSQLRejectsAnyNonFixtureDatabase(t *testing.T) {
	t.Parallel()

	fixture, err := OpenPostgreSQL(context.Background(), "postgres://example.invalid/production")
	if fixture != nil {
		t.Fatal("OpenPostgreSQL returned a fixture for a non-test database")
	}
	if !errors.Is(err, ErrUnsafeDatabaseURL) {
		t.Fatalf("OpenPostgreSQL error = %v, want ErrUnsafeDatabaseURL", err)
	}
}

func TestSafeDatabaseURLIsLoopbackAICRMTestOnly(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		databaseURL string
		wantError   bool
	}{
		{name: "ci port", databaseURL: DefaultDatabaseURL},
		{name: "isolated local port", databaseURL: "postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable"},
		{name: "literal IPv6 loopback", databaseURL: "postgres://postgres:postgres@[::1]:55432/aicrm_test?sslmode=disable"},
		{name: "hostname is not literal loopback", databaseURL: "postgres://postgres:postgres@localhost:5432/aicrm_test?sslmode=disable", wantError: true},
		{name: "other loopback address is not canonical", databaseURL: "postgres://postgres:postgres@127.0.0.2:5432/aicrm_test?sslmode=disable", wantError: true},
		{name: "external address", databaseURL: "postgres://postgres:postgres@192.0.2.10:5432/aicrm_test?sslmode=disable", wantError: true},
		{name: "wrong user", databaseURL: "postgres://root:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable", wantError: true},
		{name: "wrong password", databaseURL: "postgres://postgres:secret@127.0.0.1:5432/aicrm_test?sslmode=disable", wantError: true},
		{name: "production database", databaseURL: "postgres://postgres:postgres@127.0.0.1:5432/aicrm?sslmode=disable", wantError: true},
		{name: "unexpected query option", databaseURL: "postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=require", wantError: true},
		{name: "extra query option", databaseURL: "postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable&application_name=test", wantError: true},
		{name: "missing port", databaseURL: "postgres://postgres:postgres@127.0.0.1/aicrm_test?sslmode=disable", wantError: true},
		{name: "zero port", databaseURL: "postgres://postgres:postgres@127.0.0.1:0/aicrm_test?sslmode=disable", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDatabaseURL(test.databaseURL)
			if test.wantError && !errors.Is(err, ErrUnsafeDatabaseURL) {
				t.Fatalf("ValidateDatabaseURL() error = %v, want ErrUnsafeDatabaseURL", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("ValidateDatabaseURL() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateDatabaseURLDoesNotExposeRejectedInput(t *testing.T) {
	const secret = "dsn-password-sentinel"
	err := ValidateDatabaseURL("postgres://postgres:" + secret + "@127.0.0.1:5432/aicrm_test?sslmode=disable")
	if !errors.Is(err, ErrUnsafeDatabaseURL) {
		t.Fatalf("ValidateDatabaseURL() error = %v, want ErrUnsafeDatabaseURL", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("ValidateDatabaseURL() error exposes rejected credential")
	}
}

func TestValidateDatabaseURLForDedicatedTemporaryDatabases(t *testing.T) {
	t.Parallel()
	for _, databaseName := range []string{H01A1DatabaseName, I01BDatabaseName} {
		databaseURL := "postgres://postgres:postgres@127.0.0.1:5432/" + databaseName + "?sslmode=disable"
		if err := ValidateDatabaseURLForDatabase(databaseURL, databaseName); err != nil {
			t.Fatalf("temporary database %q rejected: %v", databaseName, err)
		}
	}
	if err := ValidateDatabaseURLForDatabase("postgres://postgres:postgres@127.0.0.1:5432/aicrm_test_other?sslmode=disable", "aicrm_test_other"); !errors.Is(err, ErrUnsafeDatabaseURL) {
		t.Fatalf("unapproved temporary database error=%v, want ErrUnsafeDatabaseURL", err)
	}
}

func TestPostgreSQLTransactionRollbackAndCleanup(t *testing.T) {
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fixture, err := OpenPostgreSQL(ctx, databaseURL)
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

	tx, err := fixture.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE acceptance_fixtures.rollback_probe (id bigint PRIMARY KEY)`); err != nil {
		t.Fatalf("create fixture table: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO acceptance_fixtures.rollback_probe (id) VALUES (1)`); err != nil {
		t.Fatalf("insert fixture row: %v", err)
	}
	if err = fixture.Rollback(ctx, tx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	var relation *string
	if err = fixture.Pool().QueryRow(ctx, `SELECT to_regclass('acceptance_fixtures.rollback_probe')::text`).Scan(&relation); err != nil {
		t.Fatalf("query rolled back relation: %v", err)
	}
	if relation != nil {
		t.Fatalf("rollback_probe still exists after rollback: %q", *relation)
	}

	if err = fixture.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err = fixture.Begin(ctx); !errors.Is(err, ErrFixtureClosed) {
		t.Fatalf("Begin() after cleanup error = %v, want ErrFixtureClosed", err)
	}
}
