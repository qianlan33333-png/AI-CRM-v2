package p3c02d_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerEventsUseStableKeysetAndOwnerScope(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	customerID, deletedCustomerID, wantIDs := seedFacts(t, ctx, fixture)
	service := contactapp.NewCustomerEventService(
		fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())},
		contactstore.NewCustomerEventRepository(),
	)
	ownerID := int64(7)

	first, err := service.List(ctx, contactapp.CustomerEventInput{
		CustomerID: customerID, OwnerStaffID: &ownerID, Limit: 2,
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.NextCursor == nil || len(first.Items) != 2 || first.Items[0].ID != wantIDs[0] || first.Items[1].ID != wantIDs[1] {
		t.Fatalf("first page = %#v, want ids %v", first, wantIDs[:2])
	}
	second, err := service.List(ctx, contactapp.CustomerEventInput{
		CustomerID: customerID, OwnerStaffID: &ownerID, Cursor: *first.NextCursor, Limit: 2,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.NextCursor != nil || len(second.Items) != 2 || second.Items[0].ID != wantIDs[2] || second.Items[1].ID != wantIDs[3] {
		t.Fatalf("second page = %#v, want ids %v", second, wantIDs[2:])
	}
	seen := map[int64]bool{}
	for _, event := range append(first.Items, second.Items...) {
		if seen[event.ID] {
			t.Fatalf("duplicate event id %d", event.ID)
		}
		seen[event.ID] = true
		if event.CustomerID != customerID || event.Payload == nil || event.OccurredAt.Location() != time.UTC {
			t.Fatalf("event = %#v", event)
		}
	}

	wrongOwner := int64(8)
	for name, input := range map[string]contactapp.CustomerEventInput{
		"wrong owner": {CustomerID: customerID, OwnerStaffID: &wrongOwner},
		"deleted":     {CustomerID: deletedCustomerID},
		"missing":     {CustomerID: customerID + 1000},
	} {
		if _, err = service.List(ctx, input); !errors.Is(err, contactapp.ErrCustomerNotFound) {
			t.Fatalf("%s error = %v, want not found", name, err)
		}
	}

	global, err := service.List(ctx, contactapp.CustomerEventInput{CustomerID: customerID, Limit: 4})
	if err != nil || len(global.Items) != 4 || global.NextCursor != nil {
		t.Fatalf("global page = %#v, err = %v", global, err)
	}
	assertIndexPlan(t, ctx, fixture, customerID, ownerID)
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
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("cleanup: %v", cleanupErr)
		}
	})
	return fixture, ctx
}

func createTables(t *testing.T, ctx context.Context, fixture *acceptancefixtures.PostgreSQL) {
	t.Helper()
	_, err := fixture.Pool().Exec(ctx, `
CREATE TABLE acceptance_fixtures.customers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  owner_staff_id BIGINT,
  is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE acceptance_fixtures.customer_events (
  id BIGINT GENERATED ALWAYS AS IDENTITY,
  customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  actor TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (occurred_at, id)
);
CREATE INDEX customer_events_timeline_idx
  ON acceptance_fixtures.customer_events (customer_id, occurred_at DESC, id DESC);
`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
}

func seedFacts(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
) (contactport.CustomerID, contactport.CustomerID, []int64) {
	t.Helper()
	var customerID, deletedCustomerID int64
	if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customers (owner_staff_id) VALUES (7) RETURNING id`).Scan(&customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customers (owner_staff_id, is_deleted) VALUES (7, TRUE) RETURNING id`).Scan(&deletedCustomerID); err != nil {
		t.Fatalf("seed deleted customer: %v", err)
	}
	times := []time.Time{
		time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC),
	}
	ids := make([]int64, 0, len(times))
	for index, occurredAt := range times {
		var id int64
		if err := fixture.Pool().QueryRow(ctx, `
INSERT INTO acceptance_fixtures.customer_events
  (customer_id, event_type, payload, actor, occurred_at)
VALUES ($1, 'stage_changed', jsonb_build_object('position', $2::integer), 'staff:7', $3)
RETURNING id`, customerID, index, occurredAt).Scan(&id); err != nil {
			t.Fatalf("seed event %d: %v", index, err)
		}
		ids = append(ids, id)
	}
	return contactport.CustomerID(customerID), contactport.CustomerID(deletedCustomerID), []int64{ids[3], ids[2], ids[1], ids[0]}
}

func assertIndexPlan(
	t *testing.T,
	ctx context.Context,
	fixture *acceptancefixtures.PostgreSQL,
	customerID contactport.CustomerID,
	ownerID int64,
) {
	t.Helper()
	if _, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.customer_events
  (customer_id, event_type, payload, actor, occurred_at)
SELECT $1, 'note', '{}', 'system', '2026-08-01'::timestamptz + (number || ' seconds')::interval
FROM generate_series(1, 5000) AS number`, customerID); err != nil {
		t.Fatalf("seed plan rows: %v", err)
	}
	if _, err := fixture.Pool().Exec(ctx, `
INSERT INTO acceptance_fixtures.customers (owner_staff_id)
SELECT 8 FROM generate_series(1, 1000);
WITH distractor AS (
  INSERT INTO acceptance_fixtures.customers (owner_staff_id) VALUES (8) RETURNING id
)
INSERT INTO acceptance_fixtures.customer_events
  (customer_id, event_type, payload, actor, occurred_at)
SELECT distractor.id, 'note', '{}', 'system',
  '2026-08-01'::timestamptz + (number || ' seconds')::interval
FROM distractor CROSS JOIN generate_series(1, 5000) AS number`); err != nil {
		t.Fatalf("seed distractor rows: %v", err)
	}
	if _, err := fixture.Pool().Exec(ctx, `ANALYZE acceptance_fixtures.customers`); err != nil {
		t.Fatalf("analyze customers: %v", err)
	}
	if _, err := fixture.Pool().Exec(ctx, `ANALYZE acceptance_fixtures.customer_events`); err != nil {
		t.Fatalf("analyze customer events: %v", err)
	}
	productionQuery := generatedCustomerEventQuery(t)
	productionQuery = strings.Replace(productionQuery, "FROM customers AS c", "FROM acceptance_fixtures.customers AS c", 1)
	productionQuery = strings.Replace(productionQuery, "FROM customer_events AS ce", "FROM acceptance_fixtures.customer_events AS ce", 1)
	rows, err := fixture.Pool().Query(ctx, "EXPLAIN (COSTS OFF)\n"+productionQuery, nil, nil, int32(51), customerID, ownerID)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatalf("scan explain: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("explain rows: %v", err)
	}
	if strings.Contains(plan.String(), "Seq Scan") || !strings.Contains(plan.String(), "customer_events_timeline_idx") {
		t.Fatalf("unexpected plan:\n%s", plan.String())
	}
}

func generatedCustomerEventQuery(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate acceptance source")
	}
	generatedPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "internal", "contact", "store", "generated", "customer_events.sql.go")
	source, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated customer event query: %v", err)
	}
	match := regexp.MustCompile("(?s)const listCustomerEvents = `([^`]*)`").FindSubmatch(source)
	if len(match) != 2 {
		t.Fatal("generated ListCustomerEvents query is unavailable")
	}
	return string(match[1])
}
