package internaleventsacceptance_test

import (
	"fmt"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
)

func TestCloudAuditUsesExactTraceAndSessionWithoutMutatingFacts(t *testing.T) {
	pool, ctx := openPool(t)
	marker := fmt.Sprintf("p4-cloud-audit-%d", time.Now().UnixNano())
	traceID, sessionID := marker+"-trace", marker+"-session"
	var eventID int64
	if err := pool.QueryRow(ctx, `INSERT INTO event_log(event_type,payload,occurred_at,idempotency_key,dispatched)
VALUES($1,jsonb_build_object('trace_id',$2::text,'session_id',$3::text),now(),$4,true) RETURNING id`,
		"campaign.recipient.dispatch.requested", traceID, sessionID, marker).Scan(&eventID); err != nil {
		t.Fatalf("seed cloud audit event: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO event_deliveries(event_id,consumer,status,attempt_count) VALUES
($1,$2,'completed',1),($1,$3,'outcome_unknown',1)`, eventID, marker+"-completed", marker+"-unknown"); err != nil {
		t.Fatalf("seed cloud audit deliveries: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id=$1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM event_log WHERE id=$1`, eventID)
	})

	before := sourceFacts(t, ctx, pool)
	items, err := eventstore.NewCloudAuditRepository(pool).ListCloudAudit(ctx, eventport.CloudAuditFilter{TraceID: traceID, SessionID: sessionID, Limit: 10})
	if err != nil {
		t.Fatalf("list cloud audit: %v", err)
	}
	if len(items) != 1 || items[0].EventID != eventport.EventID(eventID) || items[0].Completed != 1 || items[0].OutcomeUnknown != 1 || items[0].Pending != 0 || !items[0].Dispatched {
		t.Fatalf("cloud audit items=%+v", items)
	}
	if after := sourceFacts(t, ctx, pool); after != before {
		t.Fatalf("cloud audit changed source facts: before=%+v after=%+v", before, after)
	}
}
