//go:build p0s04_acceptance

package p0s04_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const (
	allowEnv = "ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST"
	fixedDSN = "postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable"
)

var tables = []string{"river_client", "river_client_queue", "river_job", "river_leader", "river_migration", "river_queue"}

var _ platformriver.Lifecycle = (*queueriver.Client[pgx.Tx])(nil)
var _ = queueriver.NewClient[pgx.Tx]
var _ appruntime.Component = (*platformriver.Runtime)(nil)
var _ func(platformriver.Lifecycle) *platformriver.Runtime = platformriver.NewRuntime
var _ func(context.Context, *pgxpool.Pool, platformriver.Direction, *platformriver.MigrateOptions) error = platformriver.Migrate

func TestPinnedRiverPublicAPISurface(t *testing.T) {
	if platformriver.PinnedVersion != "v0.24.0" {
		t.Fatalf("PinnedVersion = %q", platformriver.PinnedVersion)
	}
	m, err := rivermigrate.New(riverpgxv5.New(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if versions := m.AllVersions(); len(versions) != 6 || versions[5].Version != 6 {
		t.Fatalf("AllVersions = %#v", versions)
	}
	if _, err := m.GetVersion(6); err != nil {
		t.Fatal(err)
	}
	var _ func(context.Context, rivermigrate.Direction, *rivermigrate.MigrateOpts) (*rivermigrate.MigrateResult, error) = m.Migrate
	var _ func(context.Context) (*rivermigrate.ValidateResult, error) = m.Validate
}

type fakeLifecycle struct {
	start   func(context.Context) error
	stop    func(context.Context) error
	stopped chan struct{}
}

func (f *fakeLifecycle) Start(ctx context.Context) error { return f.start(ctx) }
func (f *fakeLifecycle) Stop(ctx context.Context) error  { return f.stop(ctx) }
func (f *fakeLifecycle) Stopped() <-chan struct{}        { return f.stopped }

func TestRuntimeLifecycleContract(t *testing.T) {
	startErr := errors.New("start")
	if err := platformriver.NewRuntime(&fakeLifecycle{start: func(context.Context) error { return startErr }, stopped: make(chan struct{})}).Run(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("Start error = %v", err)
	}

	type stopObservation struct {
		hasDeadline bool
		remaining   time.Duration
		err         error
	}
	stopErr := errors.New("stop")
	for _, tc := range []struct {
		name    string
		stopErr error
	}{{"success", nil}, {"error", stopErr}} {
		t.Run("cancel/"+tc.name, func(t *testing.T) {
			started, stops := make(chan struct{}), make(chan stopObservation, 1)
			fake := &fakeLifecycle{
				start: func(context.Context) error { close(started); return nil },
				stop: func(ctx context.Context) error {
					deadline, ok := ctx.Deadline()
					remaining := time.Duration(0)
					if ok {
						remaining = time.Until(deadline)
					}
					stops <- stopObservation{ok, remaining, ctx.Err()}
					return tc.stopErr
				},
				stopped: make(chan struct{}),
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- platformriver.NewRuntime(fake).Run(ctx) }()
			<-started
			cancel()
			select {
			case err := <-done:
				if tc.stopErr == nil && err != nil {
					t.Fatalf("successful Stop returned %v", err)
				}
				if tc.stopErr != nil && !errors.Is(err, tc.stopErr) {
					t.Fatalf("Stop error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Run did not stop")
			}
			var stop stopObservation
			select {
			case stop = <-stops:
			case <-time.After(time.Second):
				t.Fatal("Stop was not called")
			}
			if !stop.hasDeadline || stop.err != nil {
				t.Fatal("Stop context is not live and bounded")
			}
			if stop.remaining <= 0 || stop.remaining > appruntime.ShutdownGrace+10*time.Millisecond {
				t.Fatalf("Stop deadline remaining = %s", stop.remaining)
			}
		})
	}

	earlyStopped := make(chan struct{})
	close(earlyStopped)
	err := platformriver.NewRuntime(&fakeLifecycle{start: func(context.Context) error { return nil }, stopped: earlyStopped}).Run(context.Background())
	if !errors.Is(err, appruntime.ErrUnexpectedStop) {
		t.Fatalf("early stop = %v", err)
	}
}

func TestInvalidMigrationDirection(t *testing.T) {
	for _, pool := range []*pgxpool.Pool{nil, new(pgxpool.Pool)} {
		if err := platformriver.Migrate(context.Background(), pool, platformriver.Direction("sideways"), nil); !errors.Is(err, platformriver.ErrInvalidDirection) || err.Error() != `platform river migration: invalid direction "sideways"` {
			t.Fatalf("invalid direction = %v", err)
		}
	}
}

func TestOfficialMigrationUpDownUp(t *testing.T) {
	if os.Getenv(allowEnv) != "1" {
		t.Fatalf("PENDING_EXTERNAL_GATE: %s=1 is required for fixed loopback aicrm_test", allowEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, fixedDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err := pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("server_version_num = %q, err = %v", version, err)
	}
	var database string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || database != "aicrm_test" {
		t.Fatalf("database = %q, err = %v", database, err)
	}
	assertTables(t, ctx, pool, nil)
	for _, step := range []struct {
		direction platformriver.Direction
		opts      *platformriver.MigrateOptions
		want      []string
	}{
		{platformriver.DirectionUp, nil, tables},
		{platformriver.DirectionDown, &platformriver.MigrateOptions{TargetVersion: -1}, nil},
		{platformriver.DirectionUp, nil, tables},
	} {
		if err := platformriver.Migrate(ctx, pool, step.direction, step.opts); err != nil {
			t.Fatalf("Migrate(%s) = %v", step.direction, err)
		}
		if step.direction == platformriver.DirectionUp {
			m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
			if err != nil {
				t.Fatal(err)
			}
			if result, err := m.Validate(ctx); err != nil || !result.OK {
				t.Fatalf("Validate() = %#v, %v", result, err)
			}
		}
		assertTables(t, ctx, pool, step.want)
	}
}

func assertTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want []string) {
	t.Helper()
	rows, err := pool.Query(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_name LIKE 'river_%' ORDER BY table_name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil || !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v, err = %v", got, want, err)
	}
}
