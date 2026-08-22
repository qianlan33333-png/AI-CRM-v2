package internaleventsacceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 internal-events acceptance database")

var adminReadFrozenStatuses = []string{"pending", "processing", "completed", "final_failed", "outcome_unknown"}

func TestInternalEventsAdminReadIsReadOnlyAndBounded(t *testing.T) {
	p, ctx := openPool(t)
	repository := eventstore.NewAdminReadRepository(p)
	service := eventapp.NewAdminReadService(repository, func() time.Time { return time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC) })
	baseline := captureBaseline(t, ctx, repository, service)
	fixture := seedAdminReadFixture(t, ctx, p)
	cleanupAdminReadFixture := func() { deleteAdminReadFixture(t, ctx, p, fixture.eventIDs) }
	t.Cleanup(cleanupAdminReadFixture)
	before := sourceFacts(t, ctx, p)

	snapshot, err := repository.Read(ctx, "")
	if err != nil {
		t.Fatalf("read internal-events projection: %v", err)
	}
	list, err := service.List(ctx, eventport.AdminReadQuery{Limit: 200})
	if err != nil {
		t.Fatalf("list internal-events projection: %v", err)
	}
	diagnostics, err := service.Diagnostics(ctx, eventport.AdminReadQuery{})
	if err != nil {
		t.Fatalf("diagnose internal-events projection: %v", err)
	}
	if list.Total != int64(len(snapshot.Events)) || diagnostics.EventCount != int64(len(snapshot.Events)) || list.Total != baseline.allEvents+int64(len(fixture.eventIDs)) {
		t.Fatalf("projection counts list=%d diagnostics=%d source=%d", list.Total, diagnostics.EventCount, len(snapshot.Events))
	}
	assertAdminReadFixtureList(t, ctx, service, fixture, baseline)
	assertAdminReadFixtureDiagnostics(t, ctx, service, fixture, baseline)

	encoded, err := json.Marshal(struct {
		List        any `json:"list"`
		Diagnostics any `json:"diagnostics"`
	}{List: list, Diagnostics: diagnostics})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"payload", "customer_id", "idempotency_key", "lease_owner", "river_job_id", "last_error_code", "provider"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatalf("forbidden internal-events field leaked: %q", forbidden)
		}
	}

	after := sourceFacts(t, ctx, p)
	if after != before {
		t.Fatalf("admin read changed PostgreSQL source facts: before=%+v after=%+v", before, after)
	}

	assertAdminReadBadBindings503(t, ctx, p, service, fixture)
	deleteAdminReadFixture(t, ctx, p, fixture.eventIDs)
	if afterCleanup := sourceFacts(t, ctx, p); afterCleanup.events != before.events-len(fixture.eventIDs) || afterCleanup.deliveries != before.deliveries-len(fixture.deliveryIDs) {
		t.Fatalf("fixture cleanup source facts=%+v before=%+v", afterCleanup, before)
	}
}

func TestInternalEventsAdminReadRepositoryFailureIsUnavailableAndReadOnly(t *testing.T) {
	sourcePool, ctx := openPool(t)
	before := sourceFacts(t, ctx, sourcePool)

	failedPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open independent admin-read pool: %v", err)
	}
	failedPool.Close()
	repository := eventstore.NewAdminReadRepository(failedPool)
	if _, err := repository.Read(ctx, ""); err == nil {
		t.Fatal("closed independent pool unexpectedly read internal-events source")
	}
	service := eventapp.NewAdminReadService(repository, func() time.Time { return time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC) })

	if _, err := service.List(ctx, eventport.AdminReadQuery{Limit: 1}); !errors.Is(err, eventapp.ErrAdminReadUnavailable) {
		t.Fatalf("list closed-source error=%v, want ErrAdminReadUnavailable", err)
	}
	if _, err := service.Diagnostics(ctx, eventport.AdminReadQuery{}); !errors.Is(err, eventapp.ErrAdminReadUnavailable) {
		t.Fatalf("diagnostics closed-source error=%v, want ErrAdminReadUnavailable", err)
	}
	if after := sourceFacts(t, ctx, sourcePool); after != before {
		t.Fatalf("failed admin reads changed PostgreSQL source facts: before=%+v after=%+v", before, after)
	}
}

type adminReadBaseline struct {
	allEvents      int64
	allDeliveries  int64
	all            eventport.AdminReadSnapshot
	tagEvents      eventport.AdminReadSnapshot
	opEvents       eventport.AdminReadSnapshot
	allDiagnostics eventapp.AdminReadDiagnosticResult
	tagDiagnostics eventapp.AdminReadDiagnosticResult
}

type adminReadFixture struct {
	marker      string
	eventIDs    []int64
	deliveryIDs []int64
	tagIDs      []int64
	opIDs       []int64
	noDelivery  int64
}

func captureBaseline(t *testing.T, ctx context.Context, repository *eventstore.AdminReadRepository, service *eventapp.AdminReadService) adminReadBaseline {
	t.Helper()
	all, err := repository.Read(ctx, "")
	if err != nil {
		t.Fatalf("read baseline events: %v", err)
	}
	tag, err := repository.Read(ctx, eventport.EvTagApplied)
	if err != nil {
		t.Fatalf("read baseline tag events: %v", err)
	}
	op, err := repository.Read(ctx, eventport.EvOperationCycleFact)
	if err != nil {
		t.Fatalf("read baseline operation events: %v", err)
	}
	diagnostics, err := service.Diagnostics(ctx, eventport.AdminReadQuery{})
	if err != nil {
		t.Fatalf("diagnose baseline events: %v", err)
	}
	tagDiagnostics, err := service.Diagnostics(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied})
	if err != nil {
		t.Fatalf("diagnose baseline tag events: %v", err)
	}
	return adminReadBaseline{
		allEvents: int64(len(all.Events)), allDeliveries: int64(len(all.Deliveries)),
		all: all, tagEvents: tag, opEvents: op, allDiagnostics: diagnostics, tagDiagnostics: tagDiagnostics,
	}
}

func seedAdminReadFixture(t *testing.T, ctx context.Context, p *pgxpool.Pool) adminReadFixture {
	t.Helper()
	marker := fmt.Sprintf("p4-0367-0368-%d", time.Now().UnixNano())
	stamp := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	fixture := adminReadFixture{marker: marker}
	seed := func(eventType string, dispatched bool) int64 {
		var id int64
		if err := p.QueryRow(ctx, `INSERT INTO event_log (event_type, occurred_at, idempotency_key, dispatched)
VALUES ($1, $2, $3, $4)
RETURNING id`, eventType, stamp, marker+fmt.Sprintf("-event-%d", len(fixture.eventIDs)), dispatched).Scan(&id); err != nil {
			t.Fatalf("seed event type=%q: %v", eventType, err)
		}
		fixture.eventIDs = append(fixture.eventIDs, id)
		return id
	}
	tagOne := seed(eventport.EvTagApplied, false)
	tagTwo := seed(eventport.EvTagApplied, true)
	opOne := seed(eventport.EvOperationCycleFact, false)
	opTwo := seed(eventport.EvOperationCycleFact, true)
	fixture.tagIDs = []int64{tagOne, tagTwo}
	fixture.opIDs = []int64{opOne, opTwo}
	fixture.noDelivery = seed(marker+".no_delivery", false)
	fixture.deliveryIDs = append(fixture.deliveryIDs, seedAdminReadDelivery(t, ctx, p, tagOne, eventport.ConsumerAutomationTagTrigger, string(eventport.DeliveryPending), 0, nil, "", nil))
	fixture.deliveryIDs = append(fixture.deliveryIDs, seedAdminReadDelivery(t, ctx, p, tagOne, eventport.ConsumerStatsTagApplied, string(eventport.DeliveryCompleted), 1, &stamp, "", nil))
	fixture.deliveryIDs = append(fixture.deliveryIDs, seedAdminReadDelivery(t, ctx, p, tagTwo, eventport.ConsumerStatsTagApplied, string(eventport.DeliveryProcessing), 2, nil, marker, ptrTime(stamp.Add(time.Hour))))
	fixture.deliveryIDs = append(fixture.deliveryIDs, seedAdminReadDelivery(t, ctx, p, opOne, eventport.ConsumerOperationCycleFact, string(eventport.DeliveryFinalFailed), 3, &stamp, "", nil))
	fixture.deliveryIDs = append(fixture.deliveryIDs, seedAdminReadDelivery(t, ctx, p, opTwo, eventport.ConsumerOperationCycleFact, string(eventport.DeliveryOutcomeUnknown), 4, &stamp, "", nil))
	return fixture
}

func seedAdminReadDelivery(t *testing.T, ctx context.Context, p *pgxpool.Pool, eventID int64, consumer, status string, attempt int32, completed *time.Time, leaseOwner string, leaseExpires *time.Time) int64 {
	t.Helper()
	if err := insertAdminReadDelivery(ctx, p, eventID, consumer, status, attempt, completed, leaseOwner, leaseExpires); err != nil {
		t.Fatalf("seed delivery event=%d consumer=%q status=%q: %v", eventID, consumer, status, err)
	}
	// event_deliveries has no surrogate id; return the event id as the cleanup key.
	return eventID
}

func insertAdminReadDelivery(ctx context.Context, p *pgxpool.Pool, eventID int64, consumer, status string, attempt int32, completed *time.Time, leaseOwner string, leaseExpires *time.Time) error {
	_, err := p.Exec(ctx, `INSERT INTO event_deliveries (event_id, consumer, status, attempt_count, completed_at, lease_owner, lease_expires_at)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)`, eventID, consumer, status, attempt, completed, leaseOwner, leaseExpires)
	return err
}

func ptrTime(value time.Time) *time.Time { return &value }

func deleteAdminReadFixture(t *testing.T, ctx context.Context, p *pgxpool.Pool, eventIDs []int64) {
	t.Helper()
	if len(eventIDs) == 0 {
		return
	}
	if _, err := p.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs); err != nil {
		t.Fatalf("delete fixture deliveries: %v", err)
	}
	if _, err := p.Exec(ctx, `DELETE FROM event_log WHERE id = ANY($1::bigint[])`, eventIDs); err != nil {
		t.Fatalf("delete fixture events: %v", err)
	}
}

func assertAdminReadFixtureList(t *testing.T, ctx context.Context, service *eventapp.AdminReadService, fixture adminReadFixture, baseline adminReadBaseline) {
	t.Helper()
	// All fixture events deliberately share this timestamp. The generated IDs are
	// therefore the only legal tie-breaker for both pages below.
	tagPage, err := service.List(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied, Limit: 1, Offset: 0})
	if err != nil {
		t.Fatalf("list tag page one: %v", err)
	}
	if tagPage.Total != int64(len(baseline.tagEvents.Events))+2 || len(tagPage.Items) != 1 || tagPage.Items[0].EventID != eventport.EventID(fixture.tagIDs[1]) || len(tagPage.Items[0].Deliveries) != 1 {
		t.Fatalf("tag page one=%+v fixture=%+v baseline=%d", tagPage, fixture, len(baseline.tagEvents.Events))
	}
	tagPageTwo, err := service.List(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list tag page two: %v", err)
	}
	if len(tagPageTwo.Items) != 1 || tagPageTwo.Items[0].EventID != eventport.EventID(fixture.tagIDs[0]) {
		t.Fatalf("tag page two=%+v", tagPageTwo)
	}
	noDelivery, err := service.List(ctx, eventport.AdminReadQuery{EventType: fixture.marker + ".no_delivery", Limit: 1})
	if err != nil {
		t.Fatalf("list no-delivery event: %v", err)
	}
	if noDelivery.Total != 1 || len(noDelivery.Items) != 1 || noDelivery.Items[0].EventID != eventport.EventID(fixture.noDelivery) || noDelivery.Items[0].Deliveries == nil || len(noDelivery.Items[0].Deliveries) != 0 {
		t.Fatalf("no-delivery projection=%+v", noDelivery)
	}
	empty, err := service.List(ctx, eventport.AdminReadQuery{EventType: fixture.marker + ".empty", Limit: 1})
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("empty source list=%+v err=%v", empty, err)
	}

	for _, test := range []struct {
		name       string
		consumer   string
		fixtureAdd int
	}{
		{name: "automation binding", consumer: eventport.ConsumerAutomationTagTrigger, fixtureAdd: 1},
		{name: "stats binding", consumer: eventport.ConsumerStatsTagApplied, fixtureAdd: 2},
		{name: "operation binding", consumer: eventport.ConsumerOperationCycleFact, fixtureAdd: 2},
	} {
		query := eventport.AdminReadQuery{Consumer: test.consumer, Limit: 200}
		got, err := service.List(ctx, query)
		if err != nil {
			t.Fatalf("list %s: %v", test.name, err)
		}
		baselineMatches := countSelectedEvents(baseline.all, test.consumer, "")
		if got.Total != int64(baselineMatches+test.fixtureAdd) {
			t.Fatalf("list %s total=%d want=%d", test.name, got.Total, baselineMatches+test.fixtureAdd)
		}
		if test.consumer == eventport.ConsumerAutomationTagTrigger {
			item, ok := findAdminReadItem(got.Items, fixture.tagIDs[0])
			if !ok || len(item.Deliveries) != 2 || item.Deliveries[0].Consumer != eventport.ConsumerAutomationTagTrigger || item.Deliveries[1].Consumer != eventport.ConsumerStatsTagApplied {
				t.Fatalf("consumer-filtered event must retain all ordered deliveries: %+v", item)
			}
		}
	}
	for _, status := range adminReadFrozenStatuses {
		query := eventport.AdminReadQuery{Status: status, Limit: 200}
		got, err := service.List(ctx, query)
		if err != nil {
			t.Fatalf("list status=%q: %v", status, err)
		}
		baselineMatches := countSelectedEvents(baseline.all, "", status)
		if got.Total != int64(baselineMatches+1) {
			t.Fatalf("list status=%q total=%d want=%d", status, got.Total, baselineMatches+1)
		}
	}
	combined := eventport.AdminReadQuery{Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryProcessing), Limit: 200}
	got, err := service.List(ctx, combined)
	if err != nil || got.Total != int64(countSelectedEvents(baseline.all, combined.Consumer, combined.Status)+1) {
		t.Fatalf("list consumer/status=%+v err=%v", got, err)
	}
}

func assertAdminReadFixtureDiagnostics(t *testing.T, ctx context.Context, service *eventapp.AdminReadService, fixture adminReadFixture, baseline adminReadBaseline) {
	t.Helper()
	all, err := service.Diagnostics(ctx, eventport.AdminReadQuery{})
	if err != nil {
		t.Fatalf("diagnose fixture: %v", err)
	}
	if all.EventCount != baseline.allDiagnostics.EventCount+5 || all.UndispatchedEventCount != baseline.allDiagnostics.UndispatchedEventCount+3 {
		t.Fatalf("fixture diagnostic event counts=%+v baseline=%+v", all, baseline.allDiagnostics)
	}
	if all.DeliveryCounts.Pending != baseline.allDiagnostics.DeliveryCounts.Pending+1 || all.DeliveryCounts.Processing != baseline.allDiagnostics.DeliveryCounts.Processing+1 || all.DeliveryCounts.Completed != baseline.allDiagnostics.DeliveryCounts.Completed+1 || all.DeliveryCounts.FinalFailed != baseline.allDiagnostics.DeliveryCounts.FinalFailed+1 || all.DeliveryCounts.OutcomeUnknown != baseline.allDiagnostics.DeliveryCounts.OutcomeUnknown+1 {
		t.Fatalf("fixture diagnostic status counts=%+v baseline=%+v", all.DeliveryCounts, baseline.allDiagnostics.DeliveryCounts)
	}
	assertAdminReadRegistry(t, all.ConsumerRegistry)
	if strings.Join(all.ObservedDomains, ",") != "event_log,event_deliveries" || strings.Join(all.UnobservedDomains, ",") != "river_queue,outbound_provider,external_delivery" {
		t.Fatalf("diagnostic domains observed=%q unobserved=%q", all.ObservedDomains, all.UnobservedDomains)
	}
	empty, err := service.Diagnostics(ctx, eventport.AdminReadQuery{EventType: fixture.marker + ".empty"})
	if err != nil || empty.EventCount != 0 || empty.UndispatchedEventCount != 0 || empty.DeliveryCounts != (eventapp.AdminReadDeliveryCounts{}) {
		t.Fatalf("empty source diagnostics=%+v err=%v", empty, err)
	}
	tag, err := service.Diagnostics(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied})
	if err != nil || tag.EventCount != baseline.tagDiagnostics.EventCount+2 || tag.UndispatchedEventCount != baseline.tagDiagnostics.UndispatchedEventCount+1 || tag.DeliveryCounts.Pending != baseline.tagDiagnostics.DeliveryCounts.Pending+1 || tag.DeliveryCounts.Processing != baseline.tagDiagnostics.DeliveryCounts.Processing+1 || tag.DeliveryCounts.Completed != baseline.tagDiagnostics.DeliveryCounts.Completed+1 || tag.DeliveryCounts.FinalFailed != baseline.tagDiagnostics.DeliveryCounts.FinalFailed || tag.DeliveryCounts.OutcomeUnknown != baseline.tagDiagnostics.DeliveryCounts.OutcomeUnknown {
		t.Fatalf("tag diagnostics must count two distinct events and three deliveries: %+v baseline=%+v err=%v", tag, baseline.tagDiagnostics, err)
	}
	for _, status := range adminReadFrozenStatuses {
		query := eventport.AdminReadQuery{Status: status}
		got, err := service.Diagnostics(ctx, query)
		if err != nil {
			t.Fatalf("diagnose status=%q: %v", status, err)
		}
		wantEvents := countSelectedEvents(baseline.all, "", status) + 1
		wantUndispatched := countSelectedUndispatchedEvents(baseline.all, "", status) + statusFixtureUndispatched(status)
		if got.EventCount != int64(wantEvents) || got.UndispatchedEventCount != int64(wantUndispatched) || adminReadStatusCount(got.DeliveryCounts, status) != adminReadStatusCount(baseline.allDiagnostics.DeliveryCounts, status)+1 {
			t.Fatalf("diagnose status=%q got=%+v want events/undispatched=%d/%d", status, got, wantEvents, wantUndispatched)
		}
	}
	for _, test := range []struct {
		name       string
		query      eventport.AdminReadQuery
		fixtureAdd int
	}{
		{name: "consumer plus status", query: eventport.AdminReadQuery{Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryProcessing)}, fixtureAdd: 1},
		{name: "event type", query: eventport.AdminReadQuery{EventType: eventport.EvOperationCycleFact}, fixtureAdd: 2},
	} {
		got, err := service.Diagnostics(ctx, test.query)
		if err != nil {
			t.Fatalf("diagnose %s: %v", test.name, err)
		}
		wantEvents := countSelectedEvents(baseline.all, test.query.Consumer, test.query.Status)
		if test.query.EventType != "" {
			wantEvents = countSelectedEvents(baseline.opEvents, test.query.Consumer, test.query.Status)
		}
		if got.EventCount != int64(wantEvents+test.fixtureAdd) {
			t.Fatalf("diagnose %s event_count=%d want=%d", test.name, got.EventCount, wantEvents+test.fixtureAdd)
		}
	}
}

func (baseline adminReadBaseline) opEventsCount() int64 { return int64(len(baseline.opEvents.Events)) }

func countSelectedEvents(snapshot eventport.AdminReadSnapshot, consumer, status string) int {
	count := 0
	for _, selected := range selectedAdminReadEventIDs(snapshot, consumer, status) {
		if selected {
			count++
		}
	}
	return count
}

func countSelectedUndispatchedEvents(snapshot eventport.AdminReadSnapshot, consumer, status string) int {
	selected := selectedAdminReadEventIDs(snapshot, consumer, status)
	count := 0
	for _, event := range snapshot.Events {
		if selected[event.EventID] && !event.Dispatched {
			count++
		}
	}
	return count
}

func selectedAdminReadEventIDs(snapshot eventport.AdminReadSnapshot, consumer, status string) map[eventport.EventID]bool {
	byEvent := make(map[eventport.EventID]bool, len(snapshot.Events))
	for _, event := range snapshot.Events {
		byEvent[event.EventID] = consumer == "" && status == ""
	}
	for _, delivery := range snapshot.Deliveries {
		if (consumer == "" || delivery.Consumer == consumer) && (status == "" || delivery.Status == status) {
			byEvent[delivery.EventID] = true
		}
	}
	return byEvent
}

func assertAdminReadBadBindings503(t *testing.T, ctx context.Context, p *pgxpool.Pool, service *eventapp.AdminReadService, fixture adminReadFixture) {
	t.Helper()
	before := sourceFacts(t, ctx, p)
	unknownID := seedAdminReadEvent(t, ctx, p, eventport.EvTagApplied, false, fixture.marker+"-unknown")
	if err := insertAdminReadDelivery(ctx, p, unknownID, fixture.marker+"-unknown-consumer", string(eventport.DeliveryPending), 0, nil, "", nil); err != nil {
		// A future schema may make an unknown consumer unrepresentable. That
		// constraint is then the database-level proof for this source shape.
		deleteAdminReadFixture(t, ctx, p, []int64{unknownID})
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != "23514" || !strings.Contains(pgError.ConstraintName, "consumer") {
			t.Fatalf("unknown consumer insertion failed without consumer CHECK: %v", err)
		}
	} else {
		readBefore := sourceFacts(t, ctx, p)
		if _, err := service.List(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied, Limit: 200}); !errors.Is(err, eventapp.ErrAdminReadUnavailable) {
			t.Fatalf("unknown consumer error=%v", err)
		}
		if readAfter := sourceFacts(t, ctx, p); readAfter != readBefore {
			t.Fatalf("unknown-consumer read changed source facts before=%+v after=%+v", readBefore, readAfter)
		}
		deleteAdminReadFixture(t, ctx, p, []int64{unknownID})
	}
	wrongBindingID := seedAdminReadEvent(t, ctx, p, eventport.EvTagApplied, false, fixture.marker+"-wrong-binding")
	seedAdminReadDelivery(t, ctx, p, wrongBindingID, eventport.ConsumerOperationCycleFact, string(eventport.DeliveryPending), 0, nil, "", nil)
	readBefore := sourceFacts(t, ctx, p)
	if _, err := service.Diagnostics(ctx, eventport.AdminReadQuery{EventType: eventport.EvTagApplied}); !errors.Is(err, eventapp.ErrAdminReadUnavailable) {
		t.Fatalf("wrong consumer/event binding error=%v", err)
	}
	if readAfter := sourceFacts(t, ctx, p); readAfter != readBefore {
		t.Fatalf("wrong-binding read changed source facts before=%+v after=%+v", readBefore, readAfter)
	}
	deleteAdminReadFixture(t, ctx, p, []int64{wrongBindingID})
	if after := sourceFacts(t, ctx, p); after != before {
		t.Fatalf("bad-binding assertions changed source facts before=%+v after=%+v", before, after)
	}
}

func findAdminReadItem(items []eventapp.AdminReadListItem, id int64) (eventapp.AdminReadListItem, bool) {
	for _, item := range items {
		if item.EventID == eventport.EventID(id) {
			return item, true
		}
	}
	return eventapp.AdminReadListItem{}, false
}

func assertAdminReadRegistry(t *testing.T, got []eventport.AdminReadBinding) {
	t.Helper()
	want := []eventport.AdminReadBinding{
		{Consumer: "automation.tag-trigger.v1", EventTypes: []string{"customer.tag_applied"}},
		{Consumer: "stats.tag-applied.v1", EventTypes: []string{"customer.tag_applied"}},
		{Consumer: "operation-cycle.fact.v1", EventTypes: []string{"operation_cycle.fact_recorded"}},
		{Consumer: "cloud-campaign.fact.v1", EventTypes: []string{"cloud_campaign.fact_recorded"}},
	}
	if len(got) != len(want) {
		t.Fatalf("consumer registry length=%d want=%d", len(got), len(want))
	}
	for index := range want {
		if got[index].Consumer != want[index].Consumer || strings.Join(got[index].EventTypes, ",") != strings.Join(want[index].EventTypes, ",") {
			t.Fatalf("consumer registry[%d]=%+v want=%+v", index, got[index], want[index])
		}
	}
}

func statusFixtureUndispatched(status string) int {
	switch status {
	case string(eventport.DeliveryPending), string(eventport.DeliveryCompleted), string(eventport.DeliveryFinalFailed):
		return 1
	default:
		return 0
	}
}

func adminReadStatusCount(counts eventapp.AdminReadDeliveryCounts, status string) int64 {
	switch status {
	case string(eventport.DeliveryPending):
		return counts.Pending
	case string(eventport.DeliveryProcessing):
		return counts.Processing
	case string(eventport.DeliveryCompleted):
		return counts.Completed
	case string(eventport.DeliveryFinalFailed):
		return counts.FinalFailed
	case string(eventport.DeliveryOutcomeUnknown):
		return counts.OutcomeUnknown
	default:
		return -1
	}
}

func seedAdminReadEvent(t *testing.T, ctx context.Context, p *pgxpool.Pool, eventType string, dispatched bool, key string) int64 {
	t.Helper()
	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO event_log (event_type, occurred_at, idempotency_key, dispatched)
VALUES ($1, $2, $3, $4)
RETURNING id`, eventType, time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC), key, dispatched).Scan(&id); err != nil {
		t.Fatalf("seed validation event type=%q: %v", eventType, err)
	}
	return id
}

type sourceFactSnapshot struct {
	events, deliveries          int
	eventDigest, deliveryDigest string
}

func sourceFacts(t *testing.T, ctx context.Context, p *pgxpool.Pool) sourceFactSnapshot {
	t.Helper()
	var facts sourceFactSnapshot
	err := p.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM event_log),
  (SELECT count(*) FROM event_deliveries),
  (SELECT md5(COALESCE(string_agg(id::text || ':' || event_type || ':' || occurred_at::text || ':' || dispatched::text, ',' ORDER BY id), '')) FROM event_log),
  (SELECT md5(COALESCE(string_agg(event_id::text || ':' || consumer || ':' || status || ':' || attempt_count::text || ':' || completed_at::text, ',' ORDER BY event_id, consumer), '')) FROM event_deliveries)`).
		Scan(&facts.events, &facts.deliveries, &facts.eventDigest, &facts.deliveryDigest)
	if err != nil {
		t.Fatalf("snapshot internal-events source facts: %v", err)
	}
	return facts
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe internal-events database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	p, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if err := p.Ping(ctx); err != nil {
		t.Fatalf("ping internal-events database: %v", err)
	}
	var version string
	if err := p.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return p, ctx
}
