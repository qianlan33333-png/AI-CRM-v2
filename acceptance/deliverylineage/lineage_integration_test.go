package deliverylineage_acceptance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eventfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/acceptancefixture"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/acceptancefixture"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var deliveryLineageDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 delivery-lineage database")

func TestDeliveryLineagePostgreSQLReadOnlyProjection(t *testing.T) {
	pool, ctx := openDeliveryLineagePool(t)
	marker := fmt.Sprintf("delivery-lineage-%d", time.Now().UnixNano())
	stamp := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	customerDecoy := "acceptance-contact-fixture"
	ownerDecoy := "delivery-lineage-owner-must-not-leak"
	providerDecoy := "delivery-lineage-provider-must-not-leak"
	errorDecoy := "delivery-lineage-error-must-not-leak"
	payloadDecoy := "delivery-lineage-payload-must-not-leak"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := contactfixture.CreateStaffWithDetails(ctx, tx, marker+"-owner", ownerDecoy, true, stamp)
	if err != nil {
		t.Fatal(err)
	}
	if err := contactfixture.AssignCustomerOwner(ctx, tx, customerID, ownerID); err != nil {
		t.Fatal(err)
	}
	sentTaskID, err := outboundfixture.CreateSentTask(ctx, tx, customerID, time.Now().UnixNano(), providerDecoy, stamp.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	unknownTaskID, err := outboundfixture.CreateOutcomeUnknownTask(ctx, tx, customerID, time.Now().UnixNano()+1, providerDecoy, errorDecoy, stamp)
	if err != nil {
		t.Fatal(err)
	}
	completedConsumer := marker + ".completed.v1"
	completedEventID, err := eventfixture.CreateCompletedDelivery(ctx, tx, completedConsumer, payloadDecoy, marker+"-completed", stamp.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	unknownConsumer := marker + ".unknown.v1"
	unknownEventID, err := eventfixture.CreateOutcomeUnknownDelivery(ctx, tx, unknownConsumer, payloadDecoy, marker+"-unknown", stamp.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, invalidEventIDs := range [][]int64{nil, {0}, {completedEventID, completedEventID}, {completedEventID, 9223372036854775807}} {
		if cleanupErr := eventfixture.DeleteDeliveryLineageEvents(ctx, pool, invalidEventIDs); cleanupErr == nil {
			t.Fatalf("cleanup accepted invalid event IDs: %v", invalidEventIDs)
		}
	}
	t.Cleanup(func() {
		if cleanupErr := eventfixture.DeleteDeliveryLineageEvents(ctx, pool, []int64{completedEventID, unknownEventID}); cleanupErr != nil {
			t.Errorf("cleanup delivery-lineage events: %v", cleanupErr)
		}
	})

	before := deliveryLineageFactSnapshot(t, ctx, pool)
	uow := platformstore.NewUnitOfWork(pool)
	outboundReader := outboundapp.NewDeliveryLineageReader(uow, outboundstore.NewDeliveryLineageRepository())
	key := []byte("01234567890123456789012345678901")
	eventReader, err := eventapp.NewDeliveryLineageReader(uow, eventstore.NewDeliveryLineageRepository(), key)
	if err != nil {
		t.Fatal(err)
	}
	// Read the complete source windows so this test remains independent of
	// fixture rows left by other isolated acceptance cases.
	outbound, err := outboundReader.ListDeliveryLineage(ctx, 1_000_101)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventReader.ListDeliveryLineage(ctx, 1_000_101)
	if err != nil {
		t.Fatal(err)
	}
	if !outbound.Complete || !events.Complete || len(outbound.Items) < 2 || len(events.Items) < 2 {
		t.Fatalf("pages are not complete: outbound=%+v events=%+v", outbound, events)
	}
	sentOutbound, ok := findOutboundLineageItem(outbound.Items, fmt.Sprintf("outbound-task:%d", sentTaskID))
	if !ok || sentOutbound.InternalState != "sent" || sentOutbound.AttemptCount != 1 || !sentOutbound.UpdatedAt.Equal(stamp.Add(time.Second)) {
		t.Fatalf("sent outbound projection=%+v", sentOutbound)
	}
	unknownOutbound, ok := findOutboundLineageItem(outbound.Items, fmt.Sprintf("outbound-task:%d", unknownTaskID))
	if !ok || unknownOutbound.InternalState != "outcome_unknown" || unknownOutbound.AttemptCount != 1 || !unknownOutbound.UpdatedAt.Equal(stamp) {
		t.Fatalf("unknown outbound projection=%+v", unknownOutbound)
	}
	wantCompletedEventID := expectedEventLineageID(key, completedEventID, completedConsumer)
	completedEvent, ok := findEventLineageItem(events.Items, wantCompletedEventID)
	if !ok || completedEvent.InternalState != "completed" || completedEvent.AttemptCount != 1 || !completedEvent.UpdatedAt.Equal(stamp.Add(3*time.Second)) {
		t.Fatalf("completed event projection=%+v", completedEvent)
	}
	wantUnknownEventID := expectedEventLineageID(key, unknownEventID, unknownConsumer)
	unknownEvent, ok := findEventLineageItem(events.Items, wantUnknownEventID)
	if !ok || unknownEvent.InternalState != "outcome_unknown" || unknownEvent.AttemptCount != 1 || !unknownEvent.UpdatedAt.Equal(stamp.Add(2*time.Second)) {
		t.Fatalf("unknown event projection=%+v", unknownEvent)
	}
	if completedEvent.LineageID == fmt.Sprintf("event-delivery:%d", completedEventID) || len(completedEvent.LineageID) != len("event-delivery:v1:")+64 {
		t.Fatalf("event identifier is not opaque: %q", completedEvent.LineageID)
	}
	encoded, err := json.Marshal(struct {
		Outbound any `json:"outbound"`
		Events   any `json:"events"`
	}{Outbound: outbound, Events: events})
	if err != nil || strings.Contains(string(encoded), customerDecoy) || strings.Contains(string(encoded), ownerDecoy) || strings.Contains(string(encoded), providerDecoy) || strings.Contains(string(encoded), errorDecoy) || strings.Contains(string(encoded), payloadDecoy) || strings.Contains(string(encoded), completedConsumer) || strings.Contains(string(encoded), unknownConsumer) {
		t.Fatalf("redacted projection leaked a fixture value")
	}
	after := deliveryLineageFactSnapshot(t, ctx, pool)
	if after != before {
		t.Fatalf("delivery lineage read wrote database facts: before=%+v after=%+v", before, after)
	}
}

func findOutboundLineageItem(items []outboundport.DeliveryLineageItem, lineageID string) (outboundport.DeliveryLineageItem, bool) {
	for _, item := range items {
		if item.LineageID == lineageID {
			return item, true
		}
	}
	return outboundport.DeliveryLineageItem{}, false
}

func findEventLineageItem(items []eventport.DeliveryLineageItem, lineageID string) (eventport.DeliveryLineageItem, bool) {
	for _, item := range items {
		if item.LineageID == lineageID {
			return item, true
		}
	}
	return eventport.DeliveryLineageItem{}, false
}

func expectedEventLineageID(key []byte, eventID int64, consumer string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("delivery-lineage:v1\x00"))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d", eventID)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(consumer))
	return "event-delivery:v1:" + hex.EncodeToString(mac.Sum(nil))
}

type deliveryLineageFacts struct {
	Tasks, Events, Deliveries               int
	TaskDigest, EventDigest, DeliveryDigest string
}

func deliveryLineageFactSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) deliveryLineageFacts {
	t.Helper()
	var facts deliveryLineageFacts
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM outbound_tasks),
  (SELECT count(*) FROM event_log),
  (SELECT count(*) FROM event_deliveries),
  (SELECT md5(COALESCE(string_agg(id::text || ':' || status || ':' || attempt_count::text || ':' || status_updated_at::text, ',' ORDER BY id), '')) FROM outbound_tasks),
  (SELECT md5(COALESCE(string_agg(id::text || ':' || dispatched::text || ':' || occurred_at::text, ',' ORDER BY id), '')) FROM event_log),
  (SELECT md5(COALESCE(string_agg(event_id::text || ':' || consumer || ':' || status || ':' || attempt_count::text || ':' || updated_at::text, ',' ORDER BY event_id, consumer), '')) FROM event_deliveries)`).
		Scan(&facts.Tasks, &facts.Events, &facts.Deliveries, &facts.TaskDigest, &facts.EventDigest, &facts.DeliveryDigest); err != nil {
		t.Fatal(err)
	}
	return facts
}

func openDeliveryLineagePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *deliveryLineageDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*deliveryLineageDatabaseURL); err != nil {
		t.Fatalf("unsafe delivery-lineage database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *deliveryLineageDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err := pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool, ctx
}
