package segment_acceptance

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const segmentTestDSN = "postgres://postgres:postgres@127.0.0.1:55433/aicrm_test?sslmode=disable"

func TestQuerySetPG16FixedQueriesAndSyntheticAudiences(t *testing.T) {
	dsn := os.Getenv("SEGMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEGMENT_TEST_DATABASE_URL is required for the isolated PG16.14 test")
	}
	if dsn != segmentTestDSN {
		t.Fatal("SEGMENT_TEST_DATABASE_URL must select the isolated Segment slot")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var version string
	if err := connection.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version = %q, err = %v", version, err)
	}
	seedSegmentFixture(t, ctx, connection)
	if _, err := connection.Exec(ctx, "SET search_path TO acceptance_fixtures, pg_catalog"); err != nil {
		t.Fatal(err)
	}
	queries := segmentstore.NewQuerySet(connection)
	instant := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	leaves := []struct {
		name string
		load func() ([]int64, error)
		want []int64
	}{
		{"universe", func() ([]int64, error) { return queries.Universe(ctx) }, []int64{1, 2, 3, 4, 5, 6, 7, 8}},
		{"stage equal", func() ([]int64, error) { return queries.StageEqual(ctx, 1) }, []int64{1, 2, 5}},
		{"stage any", func() ([]int64, error) { return queries.StageAny(ctx, []int64{1, 2}) }, []int64{1, 2, 3, 4, 5}},
		{"owner equal", func() ([]int64, error) { return queries.OwnerEqual(ctx, 1) }, []int64{1, 3, 6}},
		{"owner any", func() ([]int64, error) { return queries.OwnerAny(ctx, []int64{1, 2}) }, []int64{1, 2, 3, 4, 6, 7}},
		{"channel equal", func() ([]int64, error) { return queries.ChannelEqual(ctx, 1) }, []int64{1, 4, 8}},
		{"channel any", func() ([]int64, error) { return queries.ChannelAny(ctx, []int64{1, 2}) }, []int64{1, 2, 4, 5, 8}},
		{"tag any", func() ([]int64, error) { return queries.TagAny(ctx, []int64{1, 2}) }, []int64{1, 2, 5, 6}},
		{"added before", func() ([]int64, error) { return queries.AddedBefore(ctx, instant) }, []int64{1, 2, 3, 4}},
		{"added after", func() ([]int64, error) { return queries.AddedAfter(ctx, instant) }, []int64{6, 7, 8}},
		{"last before", func() ([]int64, error) { return queries.LastInteractBefore(ctx, instant) }, []int64{1, 3, 5, 7}},
		{"last after", func() ([]int64, error) { return queries.LastInteractAfter(ctx, instant) }, []int64{2, 4, 6, 8}},
		{"deleted true", func() ([]int64, error) { return queries.DeletedEqual(ctx, true) }, []int64{7, 8}},
		{"deleted false", func() ([]int64, error) { return queries.DeletedEqual(ctx, false) }, []int64{1, 2, 3, 4, 5, 6}},
	}
	for _, test := range leaves {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.load()
			if err != nil {
				t.Fatal(err)
			}
			if !sameSet(got, test.want) {
				t.Fatalf("ids = %v, want set %v", got, test.want)
			}
		})
	}

	audiences := []struct {
		definition string
		want       []int64
	}{
		{`{"and":[{"field":"stage_id","op":"in","value":[1,2]},{"field":"is_deleted","op":"eq","value":false}]}`, []int64{1, 2, 3, 4, 5}},
		{`{"or":[{"field":"tag_id","op":"has_any","value":[1]},{"field":"channel_id","op":"eq","value":1}]}`, []int64{1, 4, 5, 8}},
		{`{"and":[{"field":"last_interact_at","op":"after","value":"2026-08-05T00:00:00Z"},{"field":"is_deleted","op":"eq","value":true}]}`, []int64{8}},
	}
	for _, test := range audiences {
		ast, err := dsl.Parse([]byte(test.definition))
		if err != nil {
			t.Fatal(err)
		}
		program, err := compiler.Compile(ast, instant)
		if err != nil {
			t.Fatal(err)
		}
		got, err := compiler.Execute(ctx, program, queries)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("synthetic audience %s = %v, want %v", test.definition, got, test.want)
		}
	}
	if _, err := dsl.Parse([]byte(`{"field":"stage_id) OR true --","op":"eq","value":1}`)); err == nil {
		t.Fatal("SQL-shaped field reached the fixed query family")
	}
}

func seedSegmentFixture(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	statements := []string{
		"DROP SCHEMA IF EXISTS acceptance_fixtures CASCADE",
		"CREATE SCHEMA acceptance_fixtures",
		`CREATE TABLE acceptance_fixtures.customers (
		 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		 stage_id BIGINT, owner_staff_id BIGINT, channel_id BIGINT,
		 added_at TIMESTAMPTZ, last_interact_at TIMESTAMPTZ,
		 is_deleted BOOLEAN NOT NULL
		)`,
		`CREATE TABLE acceptance_fixtures.customer_tags (
		 customer_id BIGINT NOT NULL, tag_id BIGINT NOT NULL,
		 PRIMARY KEY (customer_id, tag_id)
		)`,
		"CREATE INDEX idx_fixture_segment_stage ON acceptance_fixtures.customers (stage_id, id)",
		"CREATE INDEX idx_fixture_segment_owner ON acceptance_fixtures.customers (owner_staff_id, id)",
		"CREATE INDEX idx_fixture_segment_channel ON acceptance_fixtures.customers (channel_id, id)",
		"CREATE INDEX idx_fixture_segment_added ON acceptance_fixtures.customers (added_at, id)",
		"CREATE INDEX idx_fixture_segment_interact ON acceptance_fixtures.customers (last_interact_at, id)",
		"CREATE INDEX idx_fixture_segment_deleted ON acceptance_fixtures.customers (is_deleted, id)",
		"CREATE INDEX idx_fixture_segment_tags ON acceptance_fixtures.customer_tags (tag_id, customer_id)",
		`INSERT INTO acceptance_fixtures.customers (stage_id, owner_staff_id, channel_id, added_at, last_interact_at, is_deleted) VALUES
		 (1,1,1,'2026-08-01T00:00:00Z','2026-08-01T00:00:00Z',false),
		 (1,2,2,'2026-08-02T00:00:00Z','2026-08-08T00:00:00Z',false),
		 (2,1,3,'2026-08-03T00:00:00Z','2026-08-03T00:00:00Z',false),
		 (2,2,1,'2026-08-04T00:00:00Z','2026-08-09T00:00:00Z',false),
		 (1,3,2,'2026-08-05T00:00:00Z','2026-08-04T00:00:00Z',false),
		 (3,1,3,'2026-08-06T00:00:00Z','2026-08-10T00:00:00Z',false),
		 (3,2,3,'2026-08-07T00:00:00Z','2026-08-02T00:00:00Z',true),
		 (3,3,1,'2026-08-08T00:00:00Z','2026-08-11T00:00:00Z',true)`,
		"INSERT INTO acceptance_fixtures.customer_tags (customer_id, tag_id) VALUES (1,1),(2,2),(3,3),(5,1),(6,2),(8,3)",
	}
	for _, statement := range statements {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
}

func sameSet(got, want []int64) bool {
	seen := make(map[int64]bool, len(got))
	for _, id := range got {
		seen[id] = true
	}
	if len(seen) != len(want) {
		return false
	}
	for _, id := range want {
		if !seen[id] {
			return false
		}
	}
	return true
}
