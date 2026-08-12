package contact_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type lineageTimelineEvent struct {
	ID         int64
	CustomerID contactport.CustomerID
	EventType  string
	Payload    json.RawMessage
	Actor      string
	OccurredAt time.Time
}

func TestLineageTimelineUsesStableGlobalKeysetAndPreservesOrigin(t *testing.T) {
	pool := openContactLineagePool(t)
	rootID, descendantIDs, ownerStaffID, expected := seedLineageTimeline(t, pool)
	service := contactapp.NewCustomerEventService(
		platformstore.NewUnitOfWork(pool),
		contactstore.NewCustomerEventRepository(),
	)

	var actual []contactapp.CustomerEventRecord
	seen := make(map[int64]struct{}, len(expected))
	cursor := ""
	pageCount := 0
	for page := 0; page < 10; page++ {
		result, err := service.List(context.Background(), contactapp.CustomerEventInput{
			CustomerID: rootID, OwnerStaffID: &ownerStaffID, Cursor: cursor, Limit: 2,
		})
		if err != nil {
			t.Fatalf("list lineage timeline page=%d: %v", page+1, err)
		}
		pageCount++
		for _, item := range result.Items {
			if _, duplicate := seen[item.ID]; duplicate {
				t.Fatalf("event id=%d repeated across keyset pages", item.ID)
			}
			seen[item.ID] = struct{}{}
			actual = append(actual, item)
		}
		if result.NextCursor == nil {
			break
		}
		cursor = *result.NextCursor
	}

	if pageCount < 3 {
		t.Fatalf("timeline pages=%d, want at least three", pageCount)
	}
	if len(actual) != len(expected) {
		t.Fatalf("timeline events=%d want=%d: %#v", len(actual), len(expected), actual)
	}
	for index, want := range expected {
		got := actual[index]
		if got.ID != want.ID || got.CustomerID != want.CustomerID || got.EventType != want.EventType ||
			got.Actor != want.Actor || !got.OccurredAt.Equal(want.OccurredAt) ||
			!lineageTimelineJSONEqual(got.Payload, want.Payload) {
			t.Fatalf("event[%d]=%+v payload=%s want=%+v payload=%s", index, got, got.Payload, want, want.Payload)
		}
	}
	if !actual[0].OccurredAt.Equal(actual[1].OccurredAt) || actual[0].ID <= actual[1].ID {
		t.Fatalf("equal-time tie break ids=%d/%d at %s/%s, want descending ids", actual[0].ID, actual[1].ID, actual[0].OccurredAt, actual[1].OccurredAt)
	}

	for _, mergedID := range descendantIDs {
		if _, err := service.List(context.Background(), contactapp.CustomerEventInput{
			CustomerID: mergedID, OwnerStaffID: &ownerStaffID, Limit: 2,
		}); !errors.Is(err, contactapp.ErrCustomerNotFound) {
			t.Fatalf("direct merged customer=%d error=%v, want not found", mergedID, err)
		}
	}
	wrongOwner := ownerStaffID + 1
	if _, err := service.List(context.Background(), contactapp.CustomerEventInput{
		CustomerID: rootID, OwnerStaffID: &wrongOwner, Limit: 2,
	}); !errors.Is(err, contactapp.ErrCustomerNotFound) {
		t.Fatalf("wrong owner error=%v, want not found", err)
	}
}

func seedLineageTimeline(
	t *testing.T,
	pool *pgxpool.Pool,
) (contactport.CustomerID, []contactport.CustomerID, int64, []lineageTimelineEvent) {
	t.Helper()
	ctx := context.Background()
	prefix := fmt.Sprintf("p3c07b1-%d", time.Now().UnixNano())
	var ownerStaffID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO staff (wecom_userid, name) VALUES ($1, $2) RETURNING id`,
		prefix+"-owner", prefix+"-owner").Scan(&ownerStaffID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	ids := make([]contactport.CustomerID, 4)
	for index, suffix := range []string{"root", "child-a", "child-b", "grandchild"} {
		if err := pool.QueryRow(ctx, `
INSERT INTO customers (name, owner_staff_id) VALUES ($1, $2) RETURNING id`,
			prefix+"-"+suffix, ownerStaffID).Scan(&ids[index]); err != nil {
			t.Fatalf("seed customer %s: %v", suffix, err)
		}
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO customer_merge_lineage (merged_customer_id, primary_customer_id, actor, reason)
VALUES ($2, $1, 'p3-c07b1-acceptance', 'lineage timeline'),
       ($3, $1, 'p3-c07b1-acceptance', 'lineage timeline'),
       ($4, $2, 'p3-c07b1-acceptance', 'lineage timeline')`,
		ids[0], ids[1], ids[2], ids[3]); err != nil {
		t.Fatalf("seed lineage: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE customers SET is_deleted=TRUE WHERE id = ANY($1::bigint[])`,
		[]int64{int64(ids[1]), int64(ids[2]), int64(ids[3])}); err != nil {
		t.Fatalf("soft-delete lineage descendants: %v", err)
	}

	now := time.Now().UTC()
	base := time.Date(now.Year(), now.Month(), 15, 12, 0, 0, 0, time.UTC)
	seeds := []struct {
		customerID contactport.CustomerID
		occurredAt time.Time
		suffix     string
	}{
		{ids[0], base.Add(-time.Minute), "root-equal"},
		{ids[1], base.Add(-2 * time.Minute), "child-a"},
		{ids[3], base.Add(-3 * time.Minute), "grandchild"},
		{ids[2], base.Add(-time.Minute), "child-b-equal"},
		{ids[0], base.Add(-4 * time.Minute), "root-old"},
		{ids[1], base.Add(-5 * time.Minute), "child-a-old"},
		{ids[2], base.Add(-6 * time.Minute), "child-b-old"},
	}
	events := make([]lineageTimelineEvent, 0, len(seeds))
	for ordinal, seed := range seeds {
		event := lineageTimelineEvent{
			CustomerID: seed.customerID,
			EventType:  "acceptance." + seed.suffix,
			Payload:    json.RawMessage(fmt.Sprintf(`{"ordinal":%d,"origin":"%s"}`, ordinal+1, seed.suffix)),
			Actor:      "p3-c07b1:" + seed.suffix,
			OccurredAt: seed.occurredAt,
		}
		if err := pool.QueryRow(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			event.CustomerID, event.EventType, event.Payload, event.Actor, event.OccurredAt).Scan(&event.ID); err != nil {
			t.Fatalf("seed event %s: %v", seed.suffix, err)
		}
		events = append(events, event)
	}
	sort.Slice(events, func(left, right int) bool {
		if events[left].OccurredAt.Equal(events[right].OccurredAt) {
			return events[left].ID > events[right].ID
		}
		return events[left].OccurredAt.After(events[right].OccurredAt)
	})
	return ids[0], ids[1:], ownerStaffID, events
}

func lineageTimelineJSONEqual(left, right json.RawMessage) bool {
	var leftObject, rightObject map[string]any
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	leftDecoder.UseNumber()
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&leftObject) != nil || rightDecoder.Decode(&rightObject) != nil {
		return false
	}
	leftJSON, leftErr := json.Marshal(leftObject)
	rightJSON, rightErr := json.Marshal(rightObject)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
