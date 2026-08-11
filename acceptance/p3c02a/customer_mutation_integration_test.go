package p3c02a_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerMutationsCommitTimelineAndDomainEventsAtomically(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	stageOne, stageTwo, customerID, tagID := seedFacts(t, ctx, fixture)
	uow := fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}
	service := contactapp.NewCustomerMutationService(
		uow, contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(),
	)

	newName := "已联系客户"
	customer, err := service.Update(ctx, contactapp.CustomerUpdateCommand{
		ID: customerID, Name: &newName, Actor: "staff:7",
	})
	if err != nil || customer.Name != newName || customer.StageID == nil || *customer.StageID != stageOne {
		t.Fatalf("Update() = %#v, %v", customer, err)
	}
	assertEventParity(t, ctx, fixture, 1, "customer.updated", customerID)

	customer, err = service.SetStage(ctx, contactapp.CustomerStageCommand{
		ID: customerID, StageID: &stageTwo, Actor: "staff:7",
	})
	if err != nil || customer.StageID == nil || *customer.StageID != stageTwo {
		t.Fatalf("SetStage() = %#v, %v", customer, err)
	}
	assertEventParity(t, ctx, fixture, 2, "customer.stage_changed", customerID)

	if _, err = service.SetStage(ctx, contactapp.CustomerStageCommand{
		ID: customerID, StageID: &stageTwo, Actor: "staff:7",
	}); err != nil {
		t.Fatalf("idempotent SetStage() error = %v", err)
	}
	assertCounts(t, ctx, fixture, 2, 0)

	if err = service.AddTag(ctx, contactapp.CustomerTagCommand{
		ID: customerID, TagID: tagID, Actor: "staff:7",
	}); err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}
	assertEventParity(t, ctx, fixture, 3, "customer.tag_applied", customerID)
	assertCounts(t, ctx, fixture, 3, 1)
	if err = service.AddTag(ctx, contactapp.CustomerTagCommand{
		ID: customerID, TagID: tagID, Actor: "staff:7",
	}); err != nil {
		t.Fatalf("idempotent AddTag() error = %v", err)
	}
	assertCounts(t, ctx, fixture, 3, 1)

	if err = service.RemoveTag(ctx, contactapp.CustomerTagCommand{
		ID: customerID, TagID: tagID, Actor: "staff:7",
	}); err != nil {
		t.Fatalf("RemoveTag() error = %v", err)
	}
	assertEventParity(t, ctx, fixture, 4, "customer.tag_removed", customerID)
	assertCounts(t, ctx, fixture, 4, 0)
	if err = service.RemoveTag(ctx, contactapp.CustomerTagCommand{
		ID: customerID, TagID: tagID, Actor: "staff:7",
	}); err != nil {
		t.Fatalf("idempotent RemoveTag() error = %v", err)
	}
	assertCounts(t, ctx, fixture, 4, 0)

	failing := contactapp.NewCustomerMutationService(
		uow, contactstore.NewCustomerMutationRepository(), failingAppender{err: errors.New("append failed")},
	)
	rolledBackName := "不得提交"
	if _, err = failing.Update(ctx, contactapp.CustomerUpdateCommand{
		ID: customerID, Name: &rolledBackName, Actor: "staff:7",
	}); err == nil {
		t.Fatal("Update() with failing domain appender succeeded")
	}
	assertCustomerName(t, ctx, fixture, customerID, newName)
	assertCounts(t, ctx, fixture, 4, 0)
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

type failingAppender struct{ err error }

func (appender failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
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
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.stages (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config JSONB NOT NULL DEFAULT '{}'
);
CREATE TABLE acceptance_fixtures.staff (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
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
CREATE TABLE acceptance_fixtures.tags (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE acceptance_fixtures.customer_tags (
  customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),
  tag_id BIGINT NOT NULL REFERENCES acceptance_fixtures.tags(id),
  tagged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  tagged_by TEXT NOT NULL,
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
) (int64, int64, contactport.CustomerID, int64) {
	t.Helper()
	var stageOne, stageTwo, customerID, tagID int64
	err := fixture.Pool().QueryRow(ctx, `
WITH inserted_stages AS (
  INSERT INTO acceptance_fixtures.stages (name, sort_order)
  VALUES ('初识', 10), ('已联系', 20)
  RETURNING id, sort_order
), inserted_customer AS (
  INSERT INTO acceptance_fixtures.customers (name, stage_id)
  SELECT '待联系客户', id FROM inserted_stages WHERE sort_order = 10
  RETURNING id
), inserted_tag AS (
  INSERT INTO acceptance_fixtures.tags (name, sort_order) VALUES ('重点', 10)
  RETURNING id
)
SELECT
  (SELECT id FROM inserted_stages WHERE sort_order = 10),
  (SELECT id FROM inserted_stages WHERE sort_order = 20),
  (SELECT id FROM inserted_customer),
  (SELECT id FROM inserted_tag)
`).Scan(&stageOne, &stageTwo, &customerID, &tagID)
	if err != nil {
		t.Fatalf("seed acceptance facts: %v", err)
	}
	return stageOne, stageTwo, contactport.CustomerID(customerID), tagID
}

func assertEventParity(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	wantCount int,
	wantType string,
	wantCustomerID contactport.CustomerID,
) {
	t.Helper()
	var timelineType, domainType, timelineActor string
	var timelineCustomerID, domainCustomerID int64
	var timelinePayload, domainPayload []byte
	var timelineAt, domainAt time.Time
	err := fixture.Pool().QueryRow(ctx, `
SELECT ce.event_type, ce.customer_id, ce.payload, ce.actor, ce.occurred_at,
       el.event_type, el.customer_id, el.payload, el.occurred_at
FROM acceptance_fixtures.customer_events ce
JOIN acceptance_fixtures.event_log el ON el.id = (
  SELECT max(id) FROM acceptance_fixtures.event_log
)
ORDER BY ce.id DESC LIMIT 1
`).Scan(
		&timelineType, &timelineCustomerID, &timelinePayload, &timelineActor, &timelineAt,
		&domainType, &domainCustomerID, &domainPayload, &domainAt,
	)
	if err != nil {
		t.Fatalf("query event parity: %v", err)
	}
	if timelineType != wantType || domainType != wantType ||
		timelineCustomerID != int64(wantCustomerID) || domainCustomerID != int64(wantCustomerID) ||
		!timelineAt.Equal(domainAt) || !jsonEqual(timelinePayload, domainPayload) || timelineActor != "staff:7" {
		t.Fatalf("event parity mismatch timeline=%s/%d/%s/%s/%v domain=%s/%d/%s/%v",
			timelineType, timelineCustomerID, timelinePayload, timelineActor, timelineAt,
			domainType, domainCustomerID, domainPayload, domainAt)
	}
	assertCounts(t, ctx, fixture, wantCount, -1)
}

func assertCounts(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL, wantEvents, wantTags int) {
	t.Helper()
	var timelineCount, domainCount, tagCount int
	err := fixture.Pool().QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM acceptance_fixtures.customer_events),
  (SELECT count(*) FROM acceptance_fixtures.event_log),
  (SELECT count(*) FROM acceptance_fixtures.customer_tags)
`).Scan(&timelineCount, &domainCount, &tagCount)
	if err != nil {
		t.Fatalf("query acceptance counts: %v", err)
	}
	if timelineCount != wantEvents || domainCount != wantEvents || (wantTags >= 0 && tagCount != wantTags) {
		t.Fatalf("counts = timeline:%d domain:%d tags:%d", timelineCount, domainCount, tagCount)
	}
}

func assertCustomerName(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	customerID contactport.CustomerID,
	want string,
) {
	t.Helper()
	var got string
	if err := fixture.Pool().QueryRow(ctx,
		`SELECT name FROM acceptance_fixtures.customers WHERE id = $1`, customerID,
	).Scan(&got); err != nil || got != want {
		t.Fatalf("customer name = %q, %v; want %q", got, err, want)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil &&
		string(mustJSON(leftValue)) == string(mustJSON(rightValue))
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
