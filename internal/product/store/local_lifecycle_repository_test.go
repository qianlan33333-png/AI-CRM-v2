package store

import (
	"os"
	"strings"
	"testing"
)

func TestLocalProductLifecycleSQLContractIsSafeByConstruction(t *testing.T) {
	sql, err := os.ReadFile("queries/local_lifecycle.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(sql)
	for _, fragment := range []string{
		"-- name: UpdateLocalProductLifecycle :execrows",
		"-- name: DeleteLocalProductIfSafe :one",
		"local_lifecycle = command.payload->>'local_lifecycle'",
		"product.local_lifecycle = 'draft'",
		"product.version = (command.payload->>'expected_version')::bigint",
		"product_local_entitlements",
		"coupon_targets",
		"order_list_projections",
		"service_period_member_views",
		"service_period_member_grid_collaborators",
		"product_catalog_counters",
		"total_products = total_products - 1",
	} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("SQL contract missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(source), "ON DELETE CASCADE") || strings.Contains(source, "provider_token") || strings.Contains(source, "wechatpay") {
		t.Fatalf("local lifecycle SQL widened beyond local facts: %s", source)
	}
}
