package identity_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 identity database")

func TestIdentityStorageCatalogAndOwnership(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)

	tables := queryStrings(t, pool, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' AND table_name IN ('identities', 'customer_merges', 'pending_events', 'identity_operation_receipts')
ORDER BY table_name`)
	if want := []string{"customer_merges", "identities", "identity_operation_receipts", "pending_events"}; !equalStrings(tables, want) {
		t.Fatalf("identity tables=%v, want %v", tables, want)
	}
	columns := queryStrings(t, pool, `
SELECT column_name FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'identities' ORDER BY column_name`)
	for _, required := range []string{"customer_id", "kind", "normalized_value", "review_fingerprint"} {
		if !containsSorted(columns, required) {
			t.Fatalf("identities missing column %q: %v", required, columns)
		}
	}
	for _, forbidden := range []string{"raw_identity", "raw_value", "value"} {
		if containsSorted(columns, forbidden) {
			t.Fatalf("identities has forbidden raw column %q", forbidden)
		}
	}
	indexes := queryStrings(t, pool, `
SELECT indexdef FROM pg_indexes
WHERE schemaname = 'public' AND tablename = 'identity_operation_receipts' ORDER BY indexname`)
	for _, index := range indexes {
		if containsInsensitive(index, " using gin ") || containsInsensitive(index, "(state") {
			t.Fatalf("receipt has prohibited queue/search index: %s", index)
		}
	}
}

func TestIdentityStorageConstraintsAndContactOwnedParents(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	primaryID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create primary Contact parent: %v", err)
	}
	mergedID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatalf("create merged Contact parent: %v", err)
	}
	if primaryID < 1 || mergedID < 1 || primaryID == mergedID {
		t.Fatalf("Contact-owned parent IDs=(%d,%d)", primaryID, mergedID)
	}

	var identityID int64
	err = tx.QueryRow(ctx, `
INSERT INTO identities (customer_id, kind, scope, normalized_value, normalizer_version, assurance, source, review_fingerprint, fingerprint_key_version, bound_at)
VALUES ($1::bigint, 'unionid', 'wechat-open-platform:acceptance', 'normalized-only', 1, 'verified', 'acceptance', decode($2::text, 'hex'), 1, now())
RETURNING id`, primaryID, "00112233445566778899aabbccddeeff").Scan(&identityID)
	if err != nil || identityID < 1 {
		t.Fatalf("insert identity id=%d err=%v", identityID, err)
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO pending_events (kind, identity_ids, candidate_customer_ids, source, policy_version, event_type, idempotency_key, occurred_at)
VALUES ('attribution', ARRAY[$1::bigint], ARRAY[$2::bigint], 'acceptance', 'identity_v1', 'event', 'event-key', now())`, identityID, mergedID); err != nil {
		t.Fatalf("insert attribution pending event: %v", err)
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT invalid_merge_detail`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO customer_merges (primary_customer_id, merged_customer_id, mode, policy_version, review_fingerprint, fingerprint_key_version, operated_by, detail)
VALUES ($1::bigint, $2::bigint, 'auto', 'merge_v1', decode($3::text, 'hex'), 1, 'acceptance', $4::jsonb)`, primaryID, mergedID, "11112222333344445555666677778888", `{"raw_value":"forbidden"}`)
	if err == nil {
		t.Fatal("customer_merges accepted a non-closed detail object")
	}
	assertPGCode(t, err, "23514")
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT invalid_merge_detail`); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityReceiptCommitAndMutationConstraints(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityStorage(t, pool)
	ctx := context.Background()
	t.Run("in progress commit is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, result_schema_version)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 1)`, fmt.Sprintf("acceptance-%d", time.Now().UnixNano()), repeatHex("11"), repeatHex("22"))
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit(ctx)
		if err == nil {
			t.Fatal("in_progress receipt committed")
		}
		assertPGCode(t, err, "23514")
	})
	t.Run("completed receipt is immutable", func(t *testing.T) {
		var receiptID int64
		err := pool.QueryRow(ctx, `
INSERT INTO identity_operation_receipts (operation, idempotency_scope, key_digest, command_schema_version, payload_hmac, payload_hmac_key_version, state, result_schema_version, result_status, completed_at)
VALUES ('bind', $1::text, decode($2::text, 'hex'), 1, decode($3::text, 'hex'), 1, 'completed', 1, 'bound', now()) RETURNING id`, fmt.Sprintf("acceptance-%d", time.Now().UnixNano()), repeatHex("33"), repeatHex("44")).Scan(&receiptID)
		if err != nil || receiptID < 1 {
			t.Fatalf("insert completed receipt id=%d err=%v", receiptID, err)
		}
		_, err = pool.Exec(ctx, `UPDATE identity_operation_receipts SET completed_at = now() WHERE id = $1::bigint`, receiptID)
		if err == nil {
			t.Fatal("completed receipt was mutable")
		}
		assertPGCode(t, err, "55000")
	})
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
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetIdentityStorage(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE identity_operation_receipts, pending_events, customer_merges, identities RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
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

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func repeatHex(pair string) string {
	value := ""
	for range 32 {
		value += pair
	}
	return value
}

func containsInsensitive(value, fragment string) bool {
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
