package p3c02b_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerDetailReadHonorsOwnerScopeAndStableTagOrder(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	customerID, customerWithoutTagsID, expectedTagIDs := seedFacts(t, ctx, fixture)
	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	service := contactapp.NewCustomerDetailService(
		uow,
		contactstore.NewCustomerDetailRepository(),
	)

	global, err := service.Get(ctx, contactapp.CustomerDetailInput{ID: customerID})
	if err != nil {
		t.Fatalf("global detail error = %v", err)
	}
	assertCustomerAndTags(t, global, customerID, int64(7), expectedTagIDs)

	ownerID := int64(7)
	owned, err := service.Get(ctx, contactapp.CustomerDetailInput{ID: customerID, OwnerStaffID: &ownerID})
	if err != nil {
		t.Fatalf("owner detail error = %v", err)
	}
	assertCustomerAndTags(t, owned, customerID, ownerID, expectedTagIDs)

	wrongOwnerID := int64(8)
	if _, err = service.Get(ctx, contactapp.CustomerDetailInput{
		ID: customerID, OwnerStaffID: &wrongOwnerID,
	}); !errors.Is(err, contactapp.ErrCustomerNotFound) {
		t.Fatalf("wrong owner error = %v, want not found", err)
	}
	if _, err = service.Get(ctx, contactapp.CustomerDetailInput{ID: customerID + 1000}); !errors.Is(err, contactapp.ErrCustomerNotFound) {
		t.Fatalf("missing customer error = %v, want not found", err)
	}

	empty, err := service.Get(ctx, contactapp.CustomerDetailInput{ID: customerWithoutTagsID})
	if err != nil {
		t.Fatalf("empty-tag detail error = %v", err)
	}
	if empty.Tags == nil || len(empty.Tags) != 0 {
		t.Fatalf("empty tags = %#v, want non-nil empty slice", empty.Tags)
	}

	var externalIdentityCustomerID int64
	if err = fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customers (name, owner_staff_id, extra)
VALUES ('身份污染客户', 7, '{"nested":{"external_userid":"must-not-leak"}}')
RETURNING id`).Scan(&externalIdentityCustomerID); err != nil {
		t.Fatalf("seed external identity customer: %v", err)
	}
	if _, err = service.Get(ctx, contactapp.CustomerDetailInput{
		ID: contactport.CustomerID(externalIdentityCustomerID),
	}); !errors.Is(err, contactapp.ErrCustomerDetailUnavailable) {
		t.Fatalf("external identity extra error = %v, want unavailable", err)
	}
	assertPollutedMutationRollsBack(t, ctx, fixture, uow, externalIdentityCustomerID)
}

func assertPollutedMutationRollsBack(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	uow fixtureUoW,
	customerID int64,
) {
	t.Helper()
	var beforeName, beforeExtra string
	var beforeUpdatedAt time.Time
	if err := fixture.Pool().QueryRow(ctx, `
SELECT name, updated_at, extra::text
FROM acceptance_fixtures.customers WHERE id = $1`, customerID).Scan(
		&beforeName, &beforeUpdatedAt, &beforeExtra,
	); err != nil {
		t.Fatalf("read polluted customer before mutation: %v", err)
	}

	mutation := contactapp.NewCustomerMutationService(
		uow, contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(),
	)
	changedName := "不得提交的名称"
	if _, err := mutation.Update(ctx, contactapp.CustomerUpdateCommand{
		ID: contactport.CustomerID(customerID), Name: &changedName, Actor: "staff:7",
	}); !errors.Is(err, contactapp.ErrCustomerMutationFailed) {
		t.Fatalf("polluted mutation error = %v, want fail-closed", err)
	}

	var afterName, afterExtra string
	var afterUpdatedAt time.Time
	if err := fixture.Pool().QueryRow(ctx, `
SELECT name, updated_at, extra::text
FROM acceptance_fixtures.customers WHERE id = $1`, customerID).Scan(
		&afterName, &afterUpdatedAt, &afterExtra,
	); err != nil {
		t.Fatalf("read polluted customer after mutation: %v", err)
	}
	if afterName != beforeName || !afterUpdatedAt.Equal(beforeUpdatedAt) || afterExtra != beforeExtra {
		t.Fatalf("polluted mutation committed: before=%q/%v/%s after=%q/%v/%s",
			beforeName, beforeUpdatedAt, beforeExtra, afterName, afterUpdatedAt, afterExtra)
	}
	var customerEvents, domainEvents int
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.customer_events`).Scan(&customerEvents); err != nil {
		t.Fatalf("count customer events: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.event_log`).Scan(&domainEvents); err != nil {
		t.Fatalf("count domain events: %v", err)
	}
	if customerEvents != 0 || domainEvents != 0 {
		t.Fatalf("polluted mutation emitted events: customer=%d domain=%d", customerEvents, domainEvents)
	}
}

func assertCustomerAndTags(
	t *testing.T,
	result contactapp.CustomerDetailStoreResult,
	wantCustomerID contactport.CustomerID,
	wantOwnerID int64,
	wantTagIDs []int64,
) {
	t.Helper()
	if result.Customer.ID != wantCustomerID || result.Customer.OwnerStaffID == nil ||
		*result.Customer.OwnerStaffID != wantOwnerID || result.Customer.Name != "目标客户" {
		t.Fatalf("customer = %#v", result.Customer)
	}
	if len(result.Tags) != len(wantTagIDs) {
		t.Fatalf("tags = %#v, want ids %v", result.Tags, wantTagIDs)
	}
	for index, wantID := range wantTagIDs {
		if result.Tags[index].ID != wantID {
			t.Fatalf("tag order = %#v, want ids %v", result.Tags, wantTagIDs)
		}
	}
	if result.Tags[0].GroupID != nil || result.Tags[0].GroupName != nil ||
		result.Tags[1].GroupName == nil || *result.Tags[1].GroupName != "低序组" ||
		result.Tags[2].GroupName == nil || *result.Tags[2].GroupName != "高序组" {
		t.Fatalf("tag groups = %#v", result.Tags)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
CREATE TABLE acceptance_fixtures.stages (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE acceptance_fixtures.staff (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL
);
CREATE TABLE acceptance_fixtures.channels (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL
);
CREATE TABLE acceptance_fixtures.customers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  avatar_url TEXT,
  gender SMALLINT,
  stage_id BIGINT REFERENCES acceptance_fixtures.stages(id),
  owner_staff_id BIGINT REFERENCES acceptance_fixtures.staff(id),
  channel_id BIGINT REFERENCES acceptance_fixtures.channels(id),
  added_at TIMESTAMPTZ,
  last_interact_at TIMESTAMPTZ,
  is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  extra JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
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
CREATE TABLE acceptance_fixtures.customer_tags (
  customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),
  tag_id BIGINT NOT NULL REFERENCES acceptance_fixtures.tags(id),
  tagged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  tagged_by TEXT NOT NULL DEFAULT 'system',
  PRIMARY KEY (customer_id, tag_id)
);
CREATE TABLE acceptance_fixtures.customer_events (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  actor TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE acceptance_fixtures.event_log (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type TEXT NOT NULL,
  customer_id BIGINT,
  payload JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  dispatched BOOLEAN NOT NULL DEFAULT FALSE
)`)
	if err != nil {
		t.Fatalf("create acceptance tables: %v", err)
	}
}

func seedFacts(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
) (contactport.CustomerID, contactport.CustomerID, []int64) {
	t.Helper()
	if _, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.staff (id, name) VALUES (7, '销售甲'), (8, '销售乙')`); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	var customerID, customerWithoutTagsID int64
	if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customers (name, owner_staff_id, extra)
VALUES ('目标客户', 7, '{"source":"acceptance"}') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customers (name, owner_staff_id)
VALUES ('空标签客户', 7) RETURNING id`).Scan(&customerWithoutTagsID); err != nil {
		t.Fatalf("seed empty-tag customer: %v", err)
	}

	var lowGroupID, highGroupID int64
	if _, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.tag_groups (name, sort_order)
VALUES ('低序组', 10), ('高序组', 20)`); err != nil {
		t.Fatalf("seed tag groups: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx,
		`SELECT id FROM acceptance_fixtures.tag_groups WHERE name = '低序组'`,
	).Scan(&lowGroupID); err != nil {
		t.Fatalf("read low group: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx,
		`SELECT id FROM acceptance_fixtures.tag_groups WHERE name = '高序组'`,
	).Scan(&highGroupID); err != nil {
		t.Fatalf("read high group: %v", err)
	}

	tagIDs := make([]int64, 3)
	rows, err := fixture.Pool().Query(ctx, `
INSERT INTO acceptance_fixtures.tags (group_id, name, wecom_tag_id, sort_order)
VALUES
  (NULL, '无分组', 'wx-hidden-1', 5),
  ($1, '低序标签', 'wx-hidden-2', 5),
  ($2, '高序标签', 'wx-hidden-3', 1)
RETURNING id`, lowGroupID, highGroupID)
	if err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		if index >= len(tagIDs) {
			t.Fatal("seed returned more tag ids than expected")
		}
		if scanErr := rows.Scan(&tagIDs[index]); scanErr != nil {
			t.Fatalf("read seeded tag id: %v", scanErr)
		}
		index++
	}
	if err = rows.Err(); err != nil || index != len(tagIDs) {
		t.Fatalf("seeded tag ids = %v/%d, error=%v", tagIDs, index, err)
	}
	if _, err = fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.customer_tags (customer_id, tag_id, tagged_by)
SELECT $1, id, 'staff:7' FROM acceptance_fixtures.tags`, customerID); err != nil {
		t.Fatalf("seed customer tags: %v", err)
	}
	return contactport.CustomerID(customerID), contactport.CustomerID(customerWithoutTagsID), tagIDs
}
