package p3c01b_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerQuerySQLUsesBoundedIDsWithoutCount(t *testing.T) {
	contents, err := os.ReadFile("../../internal/contact/store/queries/customers.sql")
	if err != nil {
		t.Fatalf("read customer queries: %v", err)
	}
	querySQL := string(contents)
	if strings.Contains(strings.ToUpper(querySQL), "COUNT") {
		t.Fatal("customer query SQL must not use COUNT")
	}
	boundedStart := strings.Index(querySQL, "-- name: ListCustomerIDsBounded :many")
	if boundedStart < 0 {
		t.Fatal("bounded customer-id query is missing")
	}
	boundedSQL := querySQL[boundedStart:]
	for _, required := range []string{
		"SELECT c.id",
		"c.updated_at <= sqlc.arg(watermark)::timestamptz",
		"ORDER BY c.updated_at DESC, c.id DESC",
		"LIMIT sqlc.arg(total_limit)::integer",
	} {
		if !strings.Contains(boundedSQL, required) {
			t.Fatalf("bounded customer-id query missing %q", required)
		}
	}
	for _, forbidden := range []string{"after_updated_at", "after_id"} {
		if strings.Contains(boundedSQL, forbidden) {
			t.Fatalf("bounded customer-id query must not apply cursor %q", forbidden)
		}
	}
}

func TestCustomerQueryStoreUsesRealPostgreSQLFiltersKeysetAndBoundedTotal(t *testing.T) {
	fixture, ctx := openFixture(t)
	createContactTables(t, ctx, fixture)
	repository := contactstore.NewCustomerQueryRepository()
	if _, err := repository.ListCustomers(ctx, validQuery()); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ListCustomers() outside UoW error = %v, want transaction requirement", err)
	}

	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	err := uow.Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		_, txErr = tx.Exec(txCtx, `
INSERT INTO acceptance_fixtures.customers (
  id, name, stage_id, owner_staff_id, channel_id, added_at,
  last_interact_at, is_deleted, extra, created_at, updated_at
) VALUES
  (1, '张三', 10, 20, 30, '2026-08-01T00:00:00Z', '2026-08-10T00:00:00Z', false, '{"rank":1}', '2026-08-01T00:00:00Z', '2026-08-12T09:00:00Z'),
  (2, '李四', 11, 20, 31, '2026-08-02T00:00:00Z', '2026-08-09T00:00:00Z', false, '{"rank":2}', '2026-08-02T00:00:00Z', '2026-08-12T08:00:00Z'),
  (3, 'Alex Chen', 10, 21, 30, '2026-08-03T00:00:00Z', '2026-08-08T00:00:00Z', false, '{"rank":3}', '2026-08-03T00:00:00Z', '2026-08-12T08:00:00Z'),
  (4, '王五', 12, 22, 32, NULL, NULL, true, '{"rank":4}', '2026-08-04T00:00:00Z', '2026-08-12T07:00:00Z'),
  (5, '赵六', 10, 20, 30, '2026-08-04T00:00:00Z', '2026-08-11T00:00:00Z', false, '{"rank":5}', '2026-08-04T00:00:00Z', '2026-08-12T10:30:00Z');
INSERT INTO acceptance_fixtures.customer_tags (customer_id, tag_id) VALUES (1, 40), (3, 40), (2, 41)`)
		if txErr != nil {
			return txErr
		}

		watermark := mustTime("2026-08-12T10:00:00Z")
		query := validQuery()
		query.Watermark = watermark
		query.Limit = 2
		result, listErr := repository.ListCustomers(txCtx, query)
		if listErr != nil {
			return listErr
		}
		if !result.HasMore || result.BoundedTotal != 3 || len(result.Items) != 2 ||
			result.Items[0].ID != 1 || result.Items[1].ID != 3 {
			t.Fatalf("first page = %#v, want ids [1 3], total 3, has_more", result)
		}

		afterTime := result.Items[1].UpdatedAt
		afterID := result.Items[1].ID
		query.AfterUpdatedAt, query.AfterID = &afterTime, &afterID
		second, listErr := repository.ListCustomers(txCtx, query)
		if listErr != nil || second.HasMore || second.BoundedTotal != 3 || len(second.Items) != 1 || second.Items[0].ID != 2 {
			t.Fatalf("second page = %#v, %v, want id [2] and fixed total", second, listErr)
		}

		owner, stage, channel, tag := int64(20), int64(10), int64(30), int64(40)
		addedAfter := mustTime("2026-08-02T12:00:00Z")
		interactAfter := mustTime("2026-08-07T00:00:00Z")
		filtered := validQuery()
		filtered.Watermark = watermark
		filtered.Keyword = "Alex"
		filtered.OwnerStaffID, filtered.StageID, filtered.ChannelID, filtered.TagID = &owner, &stage, &channel, &tag
		filtered.AddedAfter, filtered.LastInteractAfter = &addedAfter, &interactAfter
		filteredResult, listErr := repository.ListCustomers(txCtx, filtered)
		if listErr != nil || len(filteredResult.Items) != 0 || filteredResult.BoundedTotal != 0 {
			t.Fatalf("combined filters = %#v, %v, want empty intersection", filteredResult, listErr)
		}
		filtered.OwnerStaffID = nil
		filteredResult, listErr = repository.ListCustomers(txCtx, filtered)
		if listErr != nil || len(filteredResult.Items) != 1 || filteredResult.Items[0].ID != 3 {
			t.Fatalf("combined filters without owner = %#v, %v, want id 3", filteredResult, listErr)
		}

		deleted := validQuery()
		deleted.Watermark, deleted.IsDeleted = watermark, true
		deletedResult, listErr := repository.ListCustomers(txCtx, deleted)
		if listErr != nil || len(deletedResult.Items) != 1 || deletedResult.Items[0].ID != 4 {
			t.Fatalf("deleted filter = %#v, %v, want id 4", deletedResult, listErr)
		}

		_, txErr = tx.Exec(txCtx, `
INSERT INTO acceptance_fixtures.customers (id, name, is_deleted, extra, created_at, updated_at)
SELECT id, 'bulk-' || id, false, '{}', '2026-01-01T00:00:00Z', '2026-08-11T00:00:00Z'
FROM generate_series(100, 10101) AS id`)
		if txErr != nil {
			return txErr
		}
		bounded, listErr := repository.ListCustomers(txCtx, validQuery())
		if listErr != nil || bounded.BoundedTotal != contactapp.CustomerListExactTotalCap+1 {
			t.Fatalf("bounded total = %d, %v, want cap+1", bounded.BoundedTotal, listErr)
		}
		return errors.New("rollback fixture")
	})
	if err == nil || err.Error() != "rollback fixture" {
		t.Fatalf("fixture transaction error = %v, want rollback sentinel", err)
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

func createContactTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE acceptance_fixtures.customers (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  avatar_url TEXT,
  gender SMALLINT,
  stage_id BIGINT,
  owner_staff_id BIGINT,
  channel_id BIGINT,
  added_at TIMESTAMPTZ,
  last_interact_at TIMESTAMPTZ,
  is_deleted BOOLEAN NOT NULL DEFAULT false,
  extra JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE acceptance_fixtures.customer_tags (
  customer_id BIGINT NOT NULL,
  tag_id BIGINT NOT NULL,
  tagged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (customer_id, tag_id)
)`)
	if err != nil {
		t.Fatalf("create contact acceptance tables: %v", err)
	}
}

func validQuery() contactapp.CustomerListQuery {
	return contactapp.CustomerListQuery{
		Watermark: mustTime("2026-08-12T10:00:00Z"),
		Limit:     contactapp.CustomerListDefaultLimit,
	}
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
