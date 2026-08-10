//go:build p0s03_acceptance

package p0s03_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	dbgen "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store/generated"
)

var (
	_ dbgen.DBTX                                = dbtxFixture{}
	_ func(dbgen.DBTX) *platformstore.PingStore = platformstore.NewPingStore
	_ interface{ Ping(context.Context) error }  = (*platformstore.PingStore)(nil)
)

var errUnexpectedFixtureCall = errors.New("unexpected DBTX fixture call")

type dbtxFixture struct{ row pgx.Row }

func (fixture dbtxFixture) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	var zero pgconn.CommandTag
	return zero, errUnexpectedFixtureCall
}

func (fixture dbtxFixture) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errUnexpectedFixtureCall
}

func (fixture dbtxFixture) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return fixture.row
}

type scanRow struct {
	value int64
	err   error
}

func (row scanRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 1 {
		return fmt.Errorf("Scan destinations = %d, want 1", len(dest))
	}
	value, ok := dest[0].(*int64)
	if !ok {
		return fmt.Errorf("Scan destination = %T, want *int64", dest[0])
	}
	*value = row.value
	return nil
}

func TestPingStoreDirectDBTXContract(t *testing.T) {
	t.Run("generated one succeeds", func(t *testing.T) {
		store := platformstore.NewPingStore(dbtxFixture{row: scanRow{value: 1}})
		if err := store.Ping(context.Background()); err != nil {
			t.Fatalf("Ping() error = %v", err)
		}
	})

	t.Run("generated error remains errors.Is visible", func(t *testing.T) {
		sentinel := errors.New("generated query failed")
		store := platformstore.NewPingStore(dbtxFixture{row: scanRow{err: sentinel}})
		if err := store.Ping(context.Background()); !errors.Is(err, sentinel) {
			t.Fatalf("errors.Is(Ping() error, sentinel) = false; error = %v", err)
		}
	})

	for _, test := range []struct {
		name  string
		value int64
		want  string
	}{
		{name: "zero", value: 0, want: "platform store ping: unexpected value 0"},
		{name: "positive non-one", value: 42, want: "platform store ping: unexpected value 42"},
		{name: "negative non-one", value: -7, want: "platform store ping: unexpected value -7"},
	} {
		t.Run("unexpected generated value "+test.name+" has exact error", func(t *testing.T) {
			store := platformstore.NewPingStore(dbtxFixture{row: scanRow{value: test.value}})
			if err := store.Ping(context.Background()); err == nil {
				t.Fatal("Ping() error = nil, want non-nil")
			} else if got := err.Error(); got != test.want {
				t.Fatalf("Ping() error = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPingStorePostgreSQL16Integration(t *testing.T) {
	dsn, err := integrationDSN()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("pgx.Connect(fixed loopback aicrm_test) error = %v", err)
	}
	defer conn.Close(ctx)

	var version string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL server_version_num: %v", err)
	}
	if version != "160014" {
		t.Fatalf("PostgreSQL server_version_num = %q, want %q", version, "160014")
	}
	if value, err := dbgen.New(conn).Ping(ctx); err != nil || value != 1 {
		t.Fatalf("generated Ping() = (%d, %v), want (1, nil)", value, err)
	}
	if err := platformstore.NewPingStore(conn).Ping(ctx); err != nil {
		t.Fatalf("PingStore.Ping() through generated query: %v", err)
	}
}

func TestIntegrationConfigurationFailsClosed(t *testing.T) {
	t.Run("switch error does not echo input", func(t *testing.T) {
		const sentinel = "switch-secret-sentinel"
		t.Setenv("P0S03_PG_INTEGRATION", sentinel)
		t.Setenv("P0S03_TEST_DATABASE_URL", acceptancefixtures.DefaultDatabaseURL)
		assertIntegrationConfigError(t, sentinel, `P0S03_PG_INTEGRATION must equal "1"`)
	})
	t.Run("DSN error does not echo password", func(t *testing.T) {
		const passwordSentinel = "password-sentinel"
		t.Setenv("P0S03_PG_INTEGRATION", "1")
		t.Setenv("P0S03_TEST_DATABASE_URL", "postgres://postgres:"+passwordSentinel+"@127.0.0.1:5432/not_aicrm_test?sslmode=disable")
		assertIntegrationConfigError(t, passwordSentinel, "P0S03_TEST_DATABASE_URL must be the safe literal-loopback aicrm_test DSN")
	})
	t.Run("dynamic literal loopback port is accepted", func(t *testing.T) {
		const dynamicDSN = "postgres://postgres:postgres@127.0.0.1:55432/aicrm_test?sslmode=disable"
		t.Setenv("P0S03_PG_INTEGRATION", "1")
		t.Setenv("P0S03_TEST_DATABASE_URL", dynamicDSN)
		got, err := integrationDSN()
		if err != nil || got != dynamicDSN {
			t.Fatalf("integrationDSN() = (%q, %v), want (%q, nil)", got, err, dynamicDSN)
		}
	})
}

func assertIntegrationConfigError(t *testing.T, sentinel, want string) {
	t.Helper()
	_, err := integrationDSN()
	if err == nil {
		t.Fatal("integrationDSN() error = nil, want rejection")
	}
	if got := err.Error(); got != want {
		t.Fatalf("integrationDSN() error = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("integrationDSN() error exposes input sentinel %q", sentinel)
	}
}

func integrationDSN() (string, error) {
	if os.Getenv("P0S03_PG_INTEGRATION") != "1" {
		return "", errors.New(`P0S03_PG_INTEGRATION must equal "1"`)
	}
	dsn := os.Getenv("P0S03_TEST_DATABASE_URL")
	if err := acceptancefixtures.ValidateDatabaseURL(dsn); err != nil {
		return "", errors.New("P0S03_TEST_DATABASE_URL must be the safe literal-loopback aicrm_test DSN")
	}
	return dsn, nil
}
