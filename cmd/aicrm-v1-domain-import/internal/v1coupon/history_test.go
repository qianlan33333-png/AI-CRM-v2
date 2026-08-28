package v1coupon

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdaptHistoryProducesReadOnlyHistoryCandidates(t *testing.T) {
	coupon, binding, claim, redemption := couponFixtures()
	coupon["status"] = "expired_in_v1"
	claim["status"] = "reserved"
	redemption["status"] = "released"
	result := AdaptHistory([]json.RawMessage{raw(t, coupon)}, []json.RawMessage{raw(t, binding)}, []json.RawMessage{raw(t, claim)}, []json.RawMessage{raw(t, redemption)})
	if result.Coupons[0].Disposition != DispositionCandidate || result.Bindings[0].Disposition != DispositionCandidate {
		t.Fatalf("static candidates = %#v %#v", result.Coupons[0], result.Bindings[0])
	}
	if result.Claims[0].Disposition != DispositionCandidate || result.Redemptions[0].Disposition != DispositionCandidate {
		t.Fatalf("dynamic history candidates = %#v %#v", result.Claims[0], result.Redemptions[0])
	}
	if fact := result.Coupons[0].Fact; fact == nil || fact.SourceID != 101 || fact.OriginalStatus != "expired_in_v1" || fact.DiscountAmountMinor != 200 || !fact.CreatedAt.Equal(coupon["created_at"].(time.Time)) {
		t.Fatalf("coupon fact=%#v", fact)
	}
	if fact := result.Bindings[0].Fact; fact == nil || fact.ProductSourceID != 301 || fact.CouponSourceID != 101 {
		t.Fatalf("binding fact=%#v", fact)
	}
	if fact := result.Claims[0].Fact; fact == nil || fact.SourceID != 501 || fact.UnionID != "union-1" || fact.OriginalStatus != "reserved" || fact.DiscountAmountMinor != 200 || fact.ReservedAt != nil || fact.ConsumedAt != nil || fact.ExpiredAt != nil {
		t.Fatalf("claim fact=%#v", fact)
	}
	if fact := result.Redemptions[0].Fact; fact == nil || fact.SourceID != 601 || fact.OriginalStatus != "released" || fact.OrderSourceID != 701 || fact.OriginalAmountMinor != 1000 || fact.DiscountAmountMinor != 200 || fact.PayableAmountMinor != 800 || fact.ConsumedAt != nil || fact.ReleasedAt != nil {
		t.Fatalf("redemption fact=%#v", fact)
	}
}

func TestAdaptHistoryArchivesStaticSourceThatCannotSafelyBecomeV2Definition(t *testing.T) {
	coupon, binding, claim, redemption := couponFixtures()
	coupon["currency"] = "USD"
	result := AdaptHistory([]json.RawMessage{raw(t, coupon)}, []json.RawMessage{raw(t, binding)}, []json.RawMessage{raw(t, claim)}, []json.RawMessage{raw(t, redemption)})
	if result.Coupons[0].Disposition != DispositionArchive || result.Coupons[0].Reason != "coupon_target_ineligible" || result.Coupons[0].Fact == nil {
		t.Fatalf("non-CNY definition=%#v", result.Coupons[0])
	}
	if result.Bindings[0].Disposition != DispositionArchive || result.Bindings[0].Reason != "coupon_not_target_candidate" {
		t.Fatalf("binding was made target candidate=%#v", result.Bindings[0])
	}
	if result.Claims[0].Disposition != DispositionCandidate || result.Redemptions[0].Disposition != DispositionCandidate {
		t.Fatal("dynamic source facts were discarded with a non-target definition")
	}
}

func TestAdaptHistoryRequiresEveryCouponBindingBeforeDefinitionCandidate(t *testing.T) {
	coupon, binding, _, _ := couponFixtures()
	broken := copyMap(binding)
	broken["trade_product_id"] = 0
	result := AdaptHistory([]json.RawMessage{raw(t, coupon)}, []json.RawMessage{raw(t, binding), raw(t, broken)}, nil, nil)
	if result.Bindings[1].Disposition != DispositionQuarantine || result.Bindings[1].Reason != "coupon_binding_shape_invalid" {
		t.Fatalf("bad binding=%#v", result.Bindings[1])
	}
	if result.Coupons[0].Disposition != DispositionArchive || result.Coupons[0].Reason != "coupon_binding_unresolved" {
		t.Fatalf("definition accepted a partial binding set=%#v", result.Coupons[0])
	}
	if result.Bindings[0].Disposition != DispositionArchive || result.Bindings[0].Reason != "coupon_not_target_candidate" {
		t.Fatalf("remaining binding stayed executable=%#v", result.Bindings[0])
	}
}

func TestAdaptHistoryFailsClosedForDuplicateOrBrokenSourceRelations(t *testing.T) {
	coupon, binding, claim, redemption := couponFixtures()
	wrongTenantBinding := copyMap(binding)
	wrongTenantBinding["id"], wrongTenantBinding["tenant_id"] = 102, "other"
	wrongTenantClaim := copyMap(claim)
	wrongTenantClaim["tenant_id"] = "other"
	unknownClaimRedemption := copyMap(redemption)
	unknownClaimRedemption["claim_id"] = 999
	result := AdaptHistory([]json.RawMessage{raw(t, coupon), raw(t, coupon)}, []json.RawMessage{raw(t, wrongTenantBinding)}, []json.RawMessage{raw(t, wrongTenantClaim)}, []json.RawMessage{raw(t, unknownClaimRedemption)})
	if result.Coupons[0].Reason != "coupon_source_ambiguous" || result.Coupons[1].Reason != "coupon_source_ambiguous" {
		t.Fatalf("duplicate coupons=%#v", result.Coupons)
	}
	if result.Bindings[0].Reason != "coupon_binding_coupon_unresolved" || result.Claims[0].Reason != "coupon_claim_coupon_unresolved" || result.Redemptions[0].Reason != "coupon_redemption_claim_unresolved" {
		t.Fatalf("broken source relations=%#v %#v %#v", result.Bindings[0], result.Claims[0], result.Redemptions[0])
	}
}

func TestAdaptHistoryRejectsMissingRequiredAndInvalidAmountsButAcceptsNullableTimes(t *testing.T) {
	coupon, binding, claim, redemption := couponFixtures()
	claim["reserved_at"], claim["consumed_at"], claim["expired_at"] = nil, nil, nil
	redemption["consumed_at"], redemption["released_at"] = nil, nil
	result := AdaptHistory([]json.RawMessage{raw(t, coupon)}, []json.RawMessage{raw(t, binding)}, []json.RawMessage{raw(t, claim)}, []json.RawMessage{raw(t, redemption)})
	if result.Claims[0].Disposition != DispositionCandidate || result.Redemptions[0].Disposition != DispositionCandidate {
		t.Fatal("nullable source timestamps were rejected")
	}
	badCoupon := copyMap(coupon)
	badCoupon["created_by"] = nil
	badClaim := copyMap(claim)
	badClaim["discount_amount_total"] = -1
	badRedemption := copyMap(redemption)
	badRedemption["payable_amount_total"] = -1
	result = AdaptHistory([]json.RawMessage{raw(t, badCoupon)}, nil, []json.RawMessage{raw(t, badClaim)}, []json.RawMessage{raw(t, badRedemption)})
	if result.Coupons[0].Reason != "coupon_json_invalid" || result.Claims[0].Reason != "coupon_claim_shape_invalid" || result.Redemptions[0].Reason != "coupon_redemption_shape_invalid" {
		t.Fatalf("invalid source accepted: %#v %#v %#v", result.Coupons[0], result.Claims[0], result.Redemptions[0])
	}
}

func TestAdaptHistoryDoesNotExposeArchiveOnlyCredentialOrIdentityFields(t *testing.T) {
	coupon, binding, claim, redemption := couponFixtures()
	coupon["public_slug"], coupon["created_by"], coupon["updated_by"] = "old-public-link", "legacy-actor", "legacy-editor"
	claim["unionid"], claim["idempotency_key_hash"] = "union-secret", "claim-secret"
	redemption["idempotency_key_hash"] = "redemption-secret"
	result := AdaptHistory([]json.RawMessage{raw(t, coupon)}, []json.RawMessage{raw(t, binding)}, []json.RawMessage{raw(t, claim)}, []json.RawMessage{raw(t, redemption)})
	encoded := string(raw(t, result))
	for _, value := range []string{"old-public-link", "legacy-actor", "legacy-editor", "union-secret", "claim-secret", "redemption-secret"} {
		if strings.Contains(encoded, value) {
			t.Fatalf("archive-only value escaped candidate boundary: %q", value)
		}
	}
}

func couponFixtures() (map[string]any, map[string]any, map[string]any, map[string]any) {
	stamp := time.Date(2026, 8, 28, 14, 5, 6, 123456000, time.FixedZone("source", 8*3600))
	end, days := stamp.Add(24*time.Hour), int32(30)
	coupon := map[string]any{
		"id": 101, "tenant_id": "tenant-1", "public_slug": "old-link", "name": "历史优惠券", "discount_amount_total": 200, "currency": "CNY", "status": "stopped", "total_issue_limit": 20, "per_user_issue_limit": 1, "issued_count": 1,
		"claim_starts_at": stamp, "claim_ends_at": end, "validity_mode": "relative_days", "use_starts_at": nil, "use_ends_at": nil, "relative_validity_days": days, "instructions": "", "first_claim_at": stamp,
		"created_by": "legacy", "updated_by": "legacy", "created_at": stamp, "updated_at": end,
	}
	binding := map[string]any{"id": 201, "tenant_id": "tenant-1", "coupon_id": 101, "trade_product_id": 301, "created_at": stamp}
	claim := map[string]any{
		"id": 501, "tenant_id": "tenant-1", "coupon_id": 101, "claim_no": "legacy-claim", "unionid": "union-1", "discount_amount_total": 200, "currency": "CNY", "valid_from": stamp, "valid_until": end, "status": "claimed", "idempotency_key_hash": "hash", "claimed_at": stamp,
		"reserved_at": nil, "consumed_at": nil, "expired_at": nil, "created_at": stamp, "updated_at": end,
	}
	redemption := map[string]any{
		"id": 601, "tenant_id": "tenant-1", "claim_id": 501, "order_id": 701, "out_trade_no": "order-701", "status": "reserved", "original_amount_total": 1000, "discount_amount_total": 200, "payable_amount_total": 800, "currency": "CNY", "reserved_until": end, "idempotency_key_hash": "hash", "release_reason": "", "reserved_at": stamp,
		"consumed_at": nil, "released_at": nil, "created_at": stamp, "updated_at": end,
	}
	return coupon, binding, claim, redemption
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func copyMap(value map[string]any) map[string]any {
	copy := make(map[string]any, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}
