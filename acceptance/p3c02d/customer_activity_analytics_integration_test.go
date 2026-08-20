package p3c02d_test

import (
	"errors"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCustomerActivityAnalyticsAggregatesMergedLineageWithoutPayload(t *testing.T) {
	fixture, ctx := openFixture(t)
	createTables(t, ctx, fixture)
	var rootID, mergedID int64
	if err := fixture.Pool().QueryRow(ctx, `INSERT INTO acceptance_fixtures.customers (owner_staff_id) VALUES (7) RETURNING id`).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Pool().QueryRow(ctx, `INSERT INTO acceptance_fixtures.customers (owner_staff_id, is_deleted) VALUES (8, TRUE) RETURNING id`).Scan(&mergedID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Pool().Exec(ctx, `INSERT INTO acceptance_fixtures.customer_merge_lineage (merged_customer_id, primary_customer_id, actor, reason) VALUES ($1,$2,'system','dedupe')`, mergedID, rootID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	for _, fact := range []struct {
		id   int64
		kind string
		at   time.Time
	}{{rootID, "customer.updated", now.Add(-time.Hour)}, {mergedID, "customer.updated", now.Add(-2 * time.Hour)}, {mergedID, "customer.created", now.AddDate(0, 0, -1)}, {rootID, "outside.window", now.AddDate(0, 0, -31)}} {
		if _, err := fixture.Pool().Exec(ctx, `INSERT INTO acceptance_fixtures.customer_events (customer_id,event_type,payload,actor,occurred_at) VALUES ($1,$2,'{"secret":"must-not-leak"}','staff:secret',$3)`, fact.id, fact.kind, fact.at); err != nil {
			t.Fatal(err)
		}
	}
	service := contactapp.NewCustomerActivityAnalyticsService(fixtureUoW{delegate: platformstore.NewUnitOfWork(fixture.Pool())}, contactstore.NewCustomerActivityAnalyticsRepository(), func() time.Time { return now })
	owner := int64(7)
	result, err := service.ReadCustomerActivityAnalytics(ctx, contactport.CustomerActivityAnalyticsQuery{CustomerID: contactport.CustomerID(rootID), OwnerStaffID: &owner, WindowDays: 30})
	if err != nil {
		t.Fatalf("ReadCustomerActivityAnalytics: %v", err)
	}
	if result.TotalEvents != 3 || result.ActiveDays != 2 || result.UniqueEventTypes != 2 || len(result.TypeFacets) != 2 || result.TypeFacets[0].EventType != "customer.updated" || result.TypeFacets[0].Count != 2 || len(result.DailyCounts) != 2 {
		t.Fatalf("result=%#v", result)
	}
	wrong := int64(8)
	if _, err = service.ReadCustomerActivityAnalytics(ctx, contactport.CustomerActivityAnalyticsQuery{CustomerID: contactport.CustomerID(rootID), OwnerStaffID: &wrong, WindowDays: 30}); !errors.Is(err, contactapp.ErrCustomerNotFound) {
		t.Fatalf("wrong owner err=%v", err)
	}
	var emptyID int64
	if err = fixture.Pool().QueryRow(ctx, `INSERT INTO acceptance_fixtures.customers (owner_staff_id) VALUES (7) RETURNING id`).Scan(&emptyID); err != nil {
		t.Fatal(err)
	}
	empty, err := service.ReadCustomerActivityAnalytics(ctx, contactport.CustomerActivityAnalyticsQuery{CustomerID: contactport.CustomerID(emptyID), OwnerStaffID: &owner, WindowDays: 7})
	if err != nil || empty.TotalEvents != 0 || empty.LastOccurredAt != nil || len(empty.TypeFacets) != 0 || len(empty.DailyCounts) != 0 {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
}
