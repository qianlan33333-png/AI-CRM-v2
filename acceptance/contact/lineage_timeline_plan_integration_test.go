package contact_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	lineagePlanTargetCustomers     = 51
	lineagePlanDistractorCustomers = 19_950
	lineagePlanEventsPerTarget     = 100
	lineagePlanEventsPerDistractor = 10
)

func TestLineageTimelineGenericPlanUsesExistingIndexes(t *testing.T) {
	pool := openContactLineagePool(t)
	rootID, ownerStaffID := seedLineageTimelinePlanFacts(t, pool)
	query := generatedLineageTimelineQuery(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire plan connection: %v", err)
	}
	defer conn.Release()

	if _, err = conn.Exec(ctx, `SET plan_cache_mode = force_generic_plan`); err != nil {
		t.Fatalf("force generic plans: %v", err)
	}
	var planCacheMode string
	if err = conn.QueryRow(ctx, `SHOW plan_cache_mode`).Scan(&planCacheMode); err != nil || planCacheMode != "force_generic_plan" {
		t.Fatalf("plan_cache_mode=%q err=%v, want force_generic_plan", planCacheMode, err)
	}

	const statementName = "p3c07b2_lineage_timeline"
	prepare := `PREPARE ` + statementName + `(timestamptz,bigint,integer,bigint,bigint) AS ` + query
	if _, err = conn.Exec(ctx, prepare); err != nil {
		t.Fatalf("prepare generated lineage timeline query: %v", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `DEALLOCATE `+statementName) }()

	explain := fmt.Sprintf(
		`EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE %s(NULL,NULL,51,%d,%d)`,
		statementName,
		rootID,
		ownerStaffID,
	)
	var raw []byte
	if err = conn.QueryRow(ctx, explain).Scan(&raw); err != nil {
		t.Fatalf("explain generated lineage timeline query: %v", err)
	}
	assertLineageTimelinePlan(t, raw)

	var genericPlans, customPlans int64
	if err = conn.QueryRow(ctx, `
SELECT generic_plans, custom_plans
FROM pg_prepared_statements
WHERE name = $1`, statementName).Scan(&genericPlans, &customPlans); err != nil {
		t.Fatalf("read prepared-plan counters: %v", err)
	}
	if genericPlans < 1 || customPlans != 0 {
		t.Fatalf("prepared-plan counters generic=%d custom=%d, want generic-only execution", genericPlans, customPlans)
	}
}

func seedLineageTimelinePlanFacts(t *testing.T, pool *pgxpool.Pool) (int64, int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	prefix := fmt.Sprintf("p3c07b2-%d", time.Now().UnixNano())

	var ownerStaffID int64
	if err := pool.QueryRow(ctx, `
INSERT INTO staff (wecom_userid, name)
VALUES ($1, $1)
RETURNING id`, prefix+"-owner").Scan(&ownerStaffID); err != nil {
		t.Fatalf("seed plan owner: %v", err)
	}

	targetIDs := make([]int64, lineagePlanTargetCustomers)
	rows, err := pool.Query(ctx, `
INSERT INTO customers (name, owner_staff_id, is_deleted)
SELECT $1 || number::text, $2, number > 1
FROM generate_series(1, $3::integer) AS number
RETURNING id, name`, prefix+"-target-", ownerStaffID, lineagePlanTargetCustomers)
	if err != nil {
		t.Fatalf("seed target customers: %v", err)
	}
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			rows.Close()
			t.Fatalf("scan target customer: %v", err)
		}
		ordinal, parseErr := strconv.Atoi(strings.TrimPrefix(name, prefix+"-target-"))
		if parseErr != nil || ordinal < 1 || ordinal > len(targetIDs) {
			rows.Close()
			t.Fatalf("target customer name=%q ordinal=%d err=%v", name, ordinal, parseErr)
		}
		targetIDs[ordinal-1] = id
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("read target customers: %v", err)
	}
	rows.Close()
	for ordinal, id := range targetIDs {
		if id <= 0 {
			t.Fatalf("target customer ordinal=%d was not returned", ordinal+1)
		}
	}

	if _, err = pool.Exec(ctx, `
INSERT INTO customers (name, owner_staff_id)
SELECT $1 || number::text, $2
FROM generate_series(1, $3::integer) AS number`,
		prefix+"-distractor-", ownerStaffID, lineagePlanDistractorCustomers); err != nil {
		t.Fatalf("seed distractor customers: %v", err)
	}
	if _, err = pool.Exec(ctx, `
INSERT INTO customer_merge_lineage
  (merged_customer_id, primary_customer_id, actor, reason)
SELECT merged_id, $1, 'p3-c07b2-plan', 'target lineage'
FROM unnest($2::bigint[]) AS merged_id`, targetIDs[0], targetIDs[1:]); err != nil {
		t.Fatalf("seed target lineage: %v", err)
	}
	if _, err = pool.Exec(ctx, `
WITH distractors AS (
  SELECT id, row_number() OVER (ORDER BY id) AS ordinal
  FROM customers
  WHERE name LIKE $1
)
INSERT INTO customer_merge_lineage
  (merged_customer_id, primary_customer_id, actor, reason)
SELECT child.id, parent.id, 'p3-c07b2-plan', 'distractor lineage'
FROM distractors AS parent
JOIN distractors AS child ON child.ordinal = parent.ordinal + 1
WHERE parent.ordinal % 2 = 1`, prefix+"-distractor-%"); err != nil {
		t.Fatalf("seed distractor lineage: %v", err)
	}

	if _, err = pool.Exec(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
SELECT customer_id,
       'p3-c07b2.target',
       jsonb_build_object('ordinal', ordinal),
       'p3-c07b2-plan',
       date_trunc('month', CURRENT_TIMESTAMP) + interval '20 days'
         - make_interval(secs => (customer_id % 1000)::integer * 100 + ordinal)
FROM unnest($1::bigint[]) AS customer_id
CROSS JOIN generate_series(1, $2::integer) AS ordinal`, targetIDs, lineagePlanEventsPerTarget); err != nil {
		t.Fatalf("seed target events: %v", err)
	}
	if _, err = pool.Exec(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
SELECT c.id,
       'p3-c07b2.distractor',
       '{}'::jsonb,
       'p3-c07b2-plan',
       date_trunc('month', CURRENT_TIMESTAMP) + interval '10 days'
         + make_interval(secs => ordinal)
FROM customers AS c
CROSS JOIN generate_series(1, $2::integer) AS ordinal
WHERE c.name LIKE $1`, prefix+"-distractor-%", lineagePlanEventsPerDistractor); err != nil {
		t.Fatalf("seed distractor events: %v", err)
	}

	for _, relation := range []string{"customers", "customer_merge_lineage", "customer_events"} {
		if _, err = pool.Exec(ctx, `ANALYZE `+relation); err != nil {
			t.Fatalf("analyze %s: %v", relation, err)
		}
	}
	return targetIDs[0], ownerStaffID
}

func generatedLineageTimelineQuery(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate C07B2 acceptance source")
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

type lineagePlanEnvelope struct {
	Plan lineagePlanNode `json:"Plan"`
}

type lineagePlanNode struct {
	NodeType     string            `json:"Node Type"`
	RelationName string            `json:"Relation Name"`
	IndexName    string            `json:"Index Name"`
	Plans        []lineagePlanNode `json:"Plans"`
}

func assertLineageTimelinePlan(t *testing.T, raw []byte) {
	t.Helper()
	var envelopes []lineagePlanEnvelope
	if err := json.Unmarshal(raw, &envelopes); err != nil || len(envelopes) != 1 {
		t.Fatalf("decode EXPLAIN JSON envelopes=%d err=%v raw=%s", len(envelopes), err, raw)
	}
	foundCustomerPK := false
	foundLineageIndex := false
	foundTimelinePartitionIndex := false
	var walk func(lineagePlanNode)
	walk = func(node lineagePlanNode) {
		if node.NodeType == "Seq Scan" && isLineageTimelinePlanTarget(node.RelationName) {
			t.Fatalf("unexpected Seq Scan on %s: %s", node.RelationName, raw)
		}
		switch {
		case node.IndexName == "customers_pkey":
			foundCustomerPK = true
		case node.IndexName == "idx_customer_merge_lineage_primary":
			foundLineageIndex = true
		case strings.HasPrefix(node.RelationName, "customer_events_") &&
			strings.HasSuffix(node.IndexName, "_customer_id_occurred_at_id_idx"):
			foundTimelinePartitionIndex = true
		}
		for _, child := range node.Plans {
			walk(child)
		}
	}
	walk(envelopes[0].Plan)
	if !foundCustomerPK || !foundLineageIndex || !foundTimelinePartitionIndex {
		t.Fatalf("required indexes customer_pk=%t lineage=%t timeline_partition=%t: %s",
			foundCustomerPK, foundLineageIndex, foundTimelinePartitionIndex, raw)
	}
}

func isLineageTimelinePlanTarget(relation string) bool {
	return relation == "customers" || relation == "customer_merge_lineage" ||
		relation == "customer_events" || strings.HasPrefix(relation, "customer_events_")
}
