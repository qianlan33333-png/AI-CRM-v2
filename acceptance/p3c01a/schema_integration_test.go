package p3c01a_test

import (
	"context"
	"flag"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16 acceptance database")

func TestMigratedContactSchemaOnPostgreSQL16(t *testing.T) {
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatalf("unsafe test database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer pool.Close()

	var versionText string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&versionText); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	version, err := strconv.Atoi(versionText)
	if err != nil || version/10000 != 16 {
		t.Fatalf("PostgreSQL version=%q parse_err=%v, want major 16", versionText, err)
	}

	columns := queryStrings(t, ctx, pool, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'customers'
ORDER BY column_name`)
	if len(columns) == 0 {
		t.Fatal("migrated customers table is missing")
	}
	for _, forbidden := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
		position := sort.SearchStrings(columns, forbidden)
		if position < len(columns) && columns[position] == forbidden {
			t.Fatalf("migrated customers table leaks %s", forbidden)
		}
	}

	indexes := queryStrings(t, ctx, pool, `
SELECT indexname
FROM pg_indexes
WHERE schemaname = 'public' AND tablename IN ('customers', 'tags', 'customer_tags')
ORDER BY indexname`)
	gotIndexes := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		gotIndexes[index] = true
	}
	for _, index := range []string{
		"idx_customer_tags_tag", "idx_customers_added_keyset", "idx_customers_channel_keyset",
		"idx_customers_deleted_keyset", "idx_customers_interact_keyset", "idx_customers_name_trgm",
		"idx_customers_owner_keyset", "idx_customers_stage_keyset", "idx_tags_catalog",
	} {
		if !gotIndexes[index] {
			t.Fatalf("migrated schema missing index %s", index)
		}
	}
	var trigram bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')`).Scan(&trigram); err != nil || !trigram {
		t.Fatalf("pg_trgm installed=%t err=%v", trigram, err)
	}
}

func queryStrings(t *testing.T, ctx context.Context, pool *pgxpool.Pool, statement string) []string {
	t.Helper()
	rows, err := pool.Query(ctx, statement)
	if err != nil {
		t.Fatalf("query catalog: %v", err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatalf("scan catalog value: %v", err)
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("query catalog: %v", err)
	}
	return values
}
