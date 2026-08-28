package main

import (
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestCouponImportRequiresExplicitActorAndFrozenReferences(t *testing.T) {
	environment := appconfig.V1ArchiveRuntime{TargetDatabaseURL: "postgres://127.0.0.1:1/unreachable", ArchiveKey: strings.Repeat("a", 32)}
	for _, test := range []struct{ actor, run, key, want string }{
		{"0", "2", strings.Repeat("k", 32), "migration-actor"},
		{"1", "0", strings.Repeat("k", 32), "frozen DM01 source HMAC key"},
		{"1", "2", "", "frozen DM01 source HMAC key"},
	} {
		t.Setenv("AICRM_DM01_SOURCE_HMAC_KEY", test.key)
		err := run([]string{"--domain=coupon", "--archive-run-id=frozen", "--migration-actor=" + test.actor, "--dm01-run-id=" + test.run}, environment)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("expected %s before connecting, got %v", test.want, err)
		}
	}
}

func TestHistoricalCouponProductRetainsNativeDiscountConstraint(t *testing.T) {
	product := productport.Product{ID: 1, Currency: "CNY", PriceMinor: 100}
	for _, test := range []struct {
		discount int64
		want     bool
	}{{-1, false}, {0, false}, {1, true}, {99, true}, {100, false}, {101, false}} {
		if historicalCouponProductEligible(product, test.discount) != test.want {
			t.Fatalf("discount %d eligibility mismatch", test.discount)
		}
	}
	product.Currency = "USD"
	if historicalCouponProductEligible(product, 1) {
		t.Fatal("non-CNY product accepted")
	}
}
