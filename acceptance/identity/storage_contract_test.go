package identity_test

import (
	"context"
	"errors"
	"flag"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 identity database")

func TestIdentityStorageCatalogAndPrivacyBoundary(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityTables(t, pool)

	columns := queryStrings(t, pool, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema='public' AND table_name='identities'
ORDER BY column_name`)
	for _, required := range []string{
		"assurance", "customer_id", "fingerprint_key_version", "kind", "normalized_value",
		"normalizer_version", "review_fingerprint", "scope", "source",
	} {
		if !containsSorted(columns, required) {
			t.Fatalf("identities missing column %s: %v", required, columns)
		}
	}
	for _, forbidden := range []string{"raw_identity", "raw_value", "id_value", "value"} {
		if containsSorted(columns, forbidden) {
			t.Fatalf("identities persists forbidden raw column %s", forbidden)
		}
	}

	tables := queryStrings(t, pool, `
SELECT table_name FROM information_schema.tables
WHERE table_schema='public' AND table_name IN (
  'identities','customer_merges','pending_events','identity_operation_receipts'
) ORDER BY table_name`)
	if len(tables) != 4 {
		t.Fatalf("identity tables=%v, want four owned tables", tables)
	}

	indexes := queryStrings(t, pool, `
SELECT indexdef FROM pg_indexes
WHERE schemaname='public' AND tablename='identity_operation_receipts'
ORDER BY indexname`)
	for _, definition := range indexes {
		if containsInsensitive(definition, " using gin ") || containsInsensitive(definition, "(state") {
			t.Fatalf("receipt has forbidden queue/search index: %s", definition)
		}
	}
}

func TestIdentityConstraintsKeepNormalizedValueOnlyInIdentities(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityTables(t, pool)
	primaryID, mergedID := seedIdentityCustomers(t, pool)

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var identityID int64
	err = tx.QueryRow(context.Background(), `
INSERT INTO identities (
  customer_id, kind, scope, normalized_value, normalizer_version, assurance,
  source, review_fingerprint, fingerprint_key_version, bound_at
) VALUES ($1, 'unionid', 'wechat-open-platform:acct-A', 'CaseSensitiveValue', 1,
  'verified', 'acceptance', decode('00112233445566778899aabbccddeeff','hex'), 1, now())
RETURNING id`, primaryID).Scan(&identityID)
	if err != nil || identityID < 1 {
		t.Fatalf("insert normalized identity id=%d err=%v", identityID, err)
	}

	_, err = tx.Exec(context.Background(), `
INSERT INTO pending_events (
  kind, identity_ids, candidate_customer_ids, payload, source, policy_version,
  event_type, occurred_at
) VALUES ('attribution', ARRAY[$1]::bigint[], ARRAY[$2]::bigint[],
  '{"normalized_value":"forbidden"}'::jsonb, 'acceptance', 'identity_v1', 'event', now())`,
		identityID, mergedID)
	if err == nil {
		t.Fatal("pending event accepted normalized identity payload")
	}
	assertPGCode(t, err, "23514")
}

func TestReceiptCommitAndClosedResultConstraints(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityTables(t, pool)

	t.Run("committed in progress is forbidden", func(t *testing.T) {
		tx, err := pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), `
INSERT INTO identity_operation_receipts (
  operation, idempotency_scope, key_digest, command_schema_version,
  payload_hmac, payload_hmac_key_version, result_schema_version
) VALUES ('bind', 'admin:17', decode(repeat('11',32),'hex'), 1,
  decode(repeat('22',32),'hex'), 1, 1)`)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit(context.Background())
		if err == nil {
			t.Fatal("committed in_progress receipt")
		}
		assertPGCode(t, err, "55000")
	})

	t.Run("closed completed result is accepted and immutable", func(t *testing.T) {
		tx, err := pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		var receiptID int64
		err = tx.QueryRow(context.Background(), `
INSERT INTO identity_operation_receipts (
  operation, idempotency_scope, key_digest, command_schema_version,
  payload_hmac, payload_hmac_key_version, state, result_schema_version,
  result, completed_at
) VALUES ('bind', 'admin:17', decode(repeat('33',32),'hex'), 1,
  decode(repeat('44',32),'hex'), 1, 'completed', 1,
  '{"status":"bound","customer_id":17,"policy_version":"bind_v1"}'::jsonb, now())
RETURNING id`).Scan(&receiptID)
		if err != nil || receiptID < 1 {
			t.Fatalf("completed receipt id=%d err=%v", receiptID, err)
		}
		_, err = tx.Exec(context.Background(), `UPDATE identity_operation_receipts SET completed_at=now() WHERE id=$1`, receiptID)
		if err == nil {
			t.Fatal("completed receipt was mutable")
		}
		assertPGCode(t, err, "55000")
	})

	t.Run("result rejects identity and receipt details", func(t *testing.T) {
		for name, result := range map[string]string{
			"normalized": `{"status":"bound","normalized_value":"forbidden"}`,
			"raw":        `{"status":"bound","raw_identity":"forbidden"}`,
			"hmac":       `{"status":"bound","payload_hmac":"forbidden"}`,
			"unknown":    `{"status":"unknown"}`,
		} {
			t.Run(name, func(t *testing.T) {
				_, err := pool.Exec(context.Background(), `
INSERT INTO identity_operation_receipts (
  operation, idempotency_scope, key_digest, command_schema_version,
  payload_hmac, payload_hmac_key_version, state, result_schema_version,
  result, completed_at
) VALUES ('bind', 'admin:18', decode(repeat($1, 32), 'hex'), 1,
  decode(repeat($3, 32), 'hex'), 1, 'completed', 1, $2::jsonb, now())`,
					"55", result, "66")
				if err == nil {
					t.Fatal("receipt accepted non-closed result")
				}
				assertPGCode(t, err, "23514")
			})
		}
	})
}

func TestMergeAndReviewRowsAreConstrainedInsideRollback(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityTables(t, pool)
	primaryID, mergedID := seedIdentityCustomers(t, pool)
	_, err := pool.Exec(context.Background(), `
INSERT INTO customer_merges (
  primary_customer_id, merged_customer_id, mode, policy_version,
  review_fingerprint, fingerprint_key_version, operated_by, detail
) VALUES ($1,$2,'auto','verified_unionid_unique_wecom_v1',
  decode('11112222333344445555666677778888','hex'),1,'acceptance',
  '{"normalized_value":"forbidden"}'::jsonb)`, primaryID, mergedID)
	if err == nil {
		t.Fatal("customer merge accepted identity-bearing audit detail")
	}
	assertPGCode(t, err, "23514")

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	_, err = tx.Exec(context.Background(), `
INSERT INTO customer_merges (
  primary_customer_id, merged_customer_id, mode, policy_version,
  review_fingerprint, fingerprint_key_version, operated_by
) VALUES ($1,$2,'auto','verified_unionid_unique_wecom_v1',
  decode('11112222333344445555666677778888','hex'),1,'acceptance')`, primaryID, mergedID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `UPDATE customer_merges SET mode='manual' WHERE merged_customer_id=$1`, mergedID)
	if err == nil {
		t.Fatal("customer_merges accepted update")
	}
	assertPGCode(t, err, "55000")
	_ = tx.Rollback(context.Background())

	tx, err = pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(context.Background(), `
INSERT INTO pending_events (
  kind, identity_ids, candidate_customer_ids, source, review_fingerprint,
  fingerprint_key_version, policy_version
) VALUES ('merge_review', ARRAY[1]::bigint[], ARRAY[$1,$2]::bigint[], 'acceptance',
  decode('9999aaaabbbbccccddddeeeeffff0000','hex'), 1, 'phone_review_v1')`, primaryID, mergedID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), `
INSERT INTO pending_events (
  kind, identity_ids, candidate_customer_ids, source, review_fingerprint,
  fingerprint_key_version, policy_version
) VALUES ('merge_review', ARRAY[1]::bigint[], ARRAY[$2,$1]::bigint[], 'acceptance',
  decode('9999aaaabbbbccccddddeeeeffff0000','hex'), 1, 'phone_review_v1')`, primaryID, mergedID)
	if err == nil {
		t.Fatal("merge review accepted noncanonical candidate order")
	}
	assertPGCode(t, err, "23514")
}

func openIdentityPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe identity database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		pool.Close()
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetIdentityTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
TRUNCATE identity_operation_receipts, pending_events, customer_merges, identities RESTART IDENTITY;
DELETE FROM customers WHERE name LIKE 'identity-acceptance-%'`)
	if err != nil {
		t.Fatalf("reset identity acceptance rows: %v", err)
	}
}

func seedIdentityCustomers(t *testing.T, pool *pgxpool.Pool) (int64, int64) {
	t.Helper()
	var first, second int64
	err := pool.QueryRow(context.Background(), `
WITH inserted AS (
  INSERT INTO customers (name) VALUES
    ('identity-acceptance-primary'), ('identity-acceptance-merged')
  RETURNING id
)
SELECT min(id), max(id) FROM inserted`).Scan(&first, &second)
	if err != nil || first < 1 || first == second {
		t.Fatalf("seed identity customers=%d,%d err=%v", first, second, err)
	}
	return first, second
}

func queryStrings(t *testing.T, pool *pgxpool.Pool, statement string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), statement)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(values)
	return values
}

func containsSorted(values []string, want string) bool {
	index := sort.SearchStrings(values, want)
	return index < len(values) && values[index] == want
}

func containsInsensitive(value, fragment string) bool {
	return len(value) >= len(fragment) &&
		containsFolded(value, fragment)
}

func containsFolded(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		match := true
		for offset := range fragment {
			left, right := value[index+offset], fragment[offset]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if right >= 'A' && right <= 'Z' {
				right += 'a' - 'A'
			}
			if left != right {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func assertPGCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("PostgreSQL error=%v, want SQLSTATE %s", err, code)
	}
}
