package port

import "testing"

func TestPE01ProductKindIsClosed(t *testing.T) {
	for _, kind := range []ProductKind{ProductKindOrdinary, ProductKindServicePeriod} {
		if !kind.Valid() {
			t.Fatalf("valid kind %q rejected", kind)
		}
	}
	for _, kind := range []ProductKind{"", "coupon", "wechat_shop", "subscription"} {
		if kind.Valid() {
			t.Fatalf("unknown kind %q accepted", kind)
		}
	}
}
