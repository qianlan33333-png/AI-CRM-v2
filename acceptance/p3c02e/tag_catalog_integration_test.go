package p3c02e_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const tagReaderRole = "acceptance_fixtures_tag_reader"

func TestTagCatalogUsesStableOrderWithoutWeComTagColumn(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTablesAndReader(t, ctx, fixture)
	wantIDs := seedTags(t, ctx, fixture)
	service := contactapp.NewTagCatalogService(
		roleUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())},
		contactstore.NewTagCatalogRepository(),
	)

	records, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list tag catalog: %v", err)
	}
	if records == nil || len(records) != len(wantIDs) {
		t.Fatalf("records = %#v, want ids %v", records, wantIDs)
	}
	for index, id := range wantIDs {
		if records[index].ID != id {
			t.Fatalf("tag order = %#v, want ids %v", records, wantIDs)
		}
	}
	if records[0].GroupName == nil || *records[0].GroupName != "低序组" ||
		records[1].GroupName == nil || *records[1].GroupName != "高序组" ||
		records[2].GroupID != nil || records[2].GroupName != nil || records[2].GroupSortOrder != nil {
		t.Fatalf("group mapping = %#v", records)
	}
	assertWeComTagColumnForbidden(t, ctx, fixture)

	if _, err = fixture.Pool().Exec(ctx, `TRUNCATE acceptance_fixtures.tags, acceptance_fixtures.tag_groups RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("clear tag fixtures: %v", err)
	}
	empty, err := service.List(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty catalog = %#v, err = %v", empty, err)
	}
}

type roleUoW struct{ delegate platformport.UnitOfWork }

func (uow roleUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return uow.delegate.Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `SET LOCAL search_path TO acceptance_fixtures, pg_catalog`); err != nil {
			return err
		}
		if _, err = tx.Exec(txCtx, `SET LOCAL ROLE acceptance_fixtures_tag_reader`); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = fixture.Pool().Exec(cleanupCtx, `DROP OWNED BY acceptance_fixtures_tag_reader; DROP ROLE IF EXISTS acceptance_fixtures_tag_reader`)
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("cleanup: %v", cleanupErr)
		}
	})
	return fixture, ctx
}

func createTablesAndReader(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.tag_groups (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE acceptance_fixtures.tags (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  group_id BIGINT REFERENCES acceptance_fixtures.tag_groups(id),
  name TEXT NOT NULL,
  wecom_tag_id TEXT UNIQUE,
  sort_order INTEGER NOT NULL DEFAULT 0
);
DROP ROLE IF EXISTS acceptance_fixtures_tag_reader;
CREATE ROLE acceptance_fixtures_tag_reader NOLOGIN;
GRANT USAGE ON SCHEMA acceptance_fixtures TO acceptance_fixtures_tag_reader;
GRANT SELECT (id, name, sort_order) ON acceptance_fixtures.tag_groups TO acceptance_fixtures_tag_reader;
GRANT SELECT (id, group_id, name, sort_order) ON acceptance_fixtures.tags TO acceptance_fixtures_tag_reader;
`)
	if err != nil {
		t.Fatalf("create tag fixtures: %v", err)
	}
}

func seedTags(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) []int64 {
	t.Helper()
	var lowGroupID, highGroupID int64
	if err := fixture.Pool().QueryRow(ctx, `INSERT INTO acceptance_fixtures.tag_groups (name, sort_order) VALUES ('低序组', 1) RETURNING id`).Scan(&lowGroupID); err != nil {
		t.Fatalf("seed low group: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `INSERT INTO acceptance_fixtures.tag_groups (name, sort_order) VALUES ('高序组', 9) RETURNING id`).Scan(&highGroupID); err != nil {
		t.Fatalf("seed high group: %v", err)
	}
	ids := make([]int64, 3)
	for index, fact := range []struct {
		groupID any
		name    string
		sort    int32
		wecom   string
	}{
		{highGroupID, "高序标签", 1, "external-high"},
		{nil, "未分组标签", -5, "external-none"},
		{lowGroupID, "低序标签", 8, "external-low"},
	} {
		if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.tags (group_id, name, sort_order, wecom_tag_id)
VALUES ($1, $2, $3, $4) RETURNING id`, fact.groupID, fact.name, fact.sort, fact.wecom).Scan(&ids[index]); err != nil {
			t.Fatalf("seed tag %d: %v", index, err)
		}
	}
	return []int64{ids[2], ids[0], ids[1]}
}

func assertWeComTagColumnForbidden(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	tx, err := fixture.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin column privilege probe: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE acceptance_fixtures_tag_reader`); err != nil {
		t.Fatalf("set reader role: %v", err)
	}
	_, err = tx.Exec(ctx, `SELECT wecom_tag_id FROM acceptance_fixtures.tags LIMIT 1`)
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "42501" {
		t.Fatalf("wecom_tag_id select error = %v, want SQLSTATE 42501", err)
	}
}
