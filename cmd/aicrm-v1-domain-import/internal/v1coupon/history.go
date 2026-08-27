// Package v1coupon classifies archived V1 coupon facts. It is deliberately
// persistence-free: only definitions and bindings may later be considered for
// an archived target; claims and redemptions remain archive-only facts.
package v1coupon

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionArchive    Disposition = "archive_only"
	DispositionQuarantine Disposition = "quarantine"
)

type CouponDefinitionFact struct {
	SourceID             int64
	TenantID             string
	Name                 string
	DiscountAmountMinor  int64
	Currency             string
	OriginalStatus       string
	TotalIssueLimit      int64
	PerUserIssueLimit    int64
	IssuedCount          int64
	ClaimStartsAt        time.Time
	ClaimEndsAt          time.Time
	ValidityMode         string
	UseStartsAt          *time.Time
	UseEndsAt            *time.Time
	RelativeValidityDays *int32
	Instructions         string
	FirstClaimAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ProductSourceID is a V1 source reference, never a V2 product ID.
type BindingFact struct {
	SourceID        int64
	TenantID        string
	CouponSourceID  int64
	ProductSourceID int64
	CreatedAt       time.Time
}

// ClaimFact is read-only history. It intentionally excludes source unionid
// and idempotency material, which remain available only in the sealed archive.
type ClaimFact struct {
	SourceID            int64
	TenantID            string
	CouponSourceID      int64
	ClaimNumber         string
	DiscountAmountMinor int64
	Currency            string
	OriginalStatus      string
	ValidFrom           time.Time
	ValidUntil          time.Time
	ClaimedAt           time.Time
	ReservedAt          *time.Time
	ConsumedAt          *time.Time
	ExpiredAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// RedemptionFact is read-only history. OrderSourceID remains a V1 reference
// until an owner-owned historical order crosswalk verifies it.
type RedemptionFact struct {
	SourceID            int64
	TenantID            string
	ClaimSourceID       int64
	OrderSourceID       int64
	OutTradeNo          string
	OriginalStatus      string
	OriginalAmountMinor int64
	DiscountAmountMinor int64
	PayableAmountMinor  int64
	Currency            string
	ReservedUntil       time.Time
	ReleaseReason       string
	ReservedAt          time.Time
	ConsumedAt          *time.Time
	ReleasedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type CouponResult struct {
	Disposition Disposition
	Reason      string
	Fact        *CouponDefinitionFact
}

type BindingResult struct {
	Disposition    Disposition
	Reason         string
	CouponSourceID int64
	Fact           *BindingFact
}

type ClaimResult struct {
	Disposition Disposition
	Reason      string
	Fact        *ClaimFact
}

type RedemptionResult struct {
	Disposition Disposition
	Reason      string
	Fact        *RedemptionFact
}

type History struct {
	Coupons     []CouponResult
	Bindings    []BindingResult
	Claims      []ClaimResult
	Redemptions []RedemptionResult
}

type couponJSON struct {
	ID                   int64      `json:"id"`
	TenantID             string     `json:"tenant_id"`
	Name                 string     `json:"name"`
	DiscountAmountMinor  int64      `json:"discount_amount_total"`
	Currency             string     `json:"currency"`
	Status               string     `json:"status"`
	TotalIssueLimit      int64      `json:"total_issue_limit"`
	PerUserIssueLimit    int64      `json:"per_user_issue_limit"`
	IssuedCount          int64      `json:"issued_count"`
	ClaimStartsAt        time.Time  `json:"claim_starts_at"`
	ClaimEndsAt          time.Time  `json:"claim_ends_at"`
	ValidityMode         string     `json:"validity_mode"`
	UseStartsAt          *time.Time `json:"use_starts_at"`
	UseEndsAt            *time.Time `json:"use_ends_at"`
	RelativeValidityDays *int32     `json:"relative_validity_days"`
	Instructions         string     `json:"instructions"`
	FirstClaimAt         *time.Time `json:"first_claim_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type bindingJSON struct {
	ID              int64     `json:"id"`
	TenantID        string    `json:"tenant_id"`
	CouponSourceID  int64     `json:"coupon_id"`
	ProductSourceID int64     `json:"trade_product_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type claimJSON struct {
	ID                  int64      `json:"id"`
	TenantID            string     `json:"tenant_id"`
	CouponSourceID      int64      `json:"coupon_id"`
	ClaimNumber         string     `json:"claim_no"`
	DiscountAmountMinor int64      `json:"discount_amount_total"`
	Currency            string     `json:"currency"`
	Status              string     `json:"status"`
	ValidFrom           time.Time  `json:"valid_from"`
	ValidUntil          time.Time  `json:"valid_until"`
	ClaimedAt           time.Time  `json:"claimed_at"`
	ReservedAt          *time.Time `json:"reserved_at"`
	ConsumedAt          *time.Time `json:"consumed_at"`
	ExpiredAt           *time.Time `json:"expired_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type redemptionJSON struct {
	ID                  int64      `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ClaimSourceID       int64      `json:"claim_id"`
	OrderSourceID       int64      `json:"order_id"`
	OutTradeNo          string     `json:"out_trade_no"`
	Status              string     `json:"status"`
	OriginalAmountMinor int64      `json:"original_amount_total"`
	DiscountAmountMinor int64      `json:"discount_amount_total"`
	PayableAmountMinor  int64      `json:"payable_amount_total"`
	Currency            string     `json:"currency"`
	ReservedUntil       time.Time  `json:"reserved_until"`
	ReleaseReason       string     `json:"release_reason"`
	ReservedAt          time.Time  `json:"reserved_at"`
	ConsumedAt          *time.Time `json:"consumed_at"`
	ReleasedAt          *time.Time `json:"released_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// AdaptHistory conserves input order and row count. It does not resolve any
// V2 IDs, emit a command, or turn a claim/redemption into a runtime action.
func AdaptHistory(coupons, bindings, claims, redemptions []json.RawMessage) History {
	result := History{Coupons: make([]CouponResult, len(coupons)), Bindings: make([]BindingResult, len(bindings)), Claims: make([]ClaimResult, len(claims)), Redemptions: make([]RedemptionResult, len(redemptions))}
	couponCounts := map[int64]int{}
	for i, row := range coupons {
		result.Coupons[i] = adaptCoupon(row)
		if result.Coupons[i].Fact != nil {
			couponCounts[result.Coupons[i].Fact.SourceID]++
		}
	}
	knownCoupons := map[int64]CouponDefinitionFact{}
	for i, coupon := range result.Coupons {
		if coupon.Fact == nil {
			continue
		}
		if couponCounts[coupon.Fact.SourceID] != 1 {
			result.Coupons[i] = CouponResult{Disposition: DispositionQuarantine, Reason: "coupon_source_ambiguous"}
			continue
		}
		knownCoupons[coupon.Fact.SourceID] = *coupon.Fact
	}

	bindingCounts, bindingPairs, bindingProblems := map[int64]int{}, map[[2]int64]int{}, map[int64]bool{}
	for i, row := range bindings {
		result.Bindings[i] = adaptBinding(row, knownCoupons)
		if result.Bindings[i].Fact != nil {
			bindingCounts[result.Bindings[i].Fact.SourceID]++
			if result.Bindings[i].Disposition == DispositionCandidate {
				bindingPairs[[2]int64{result.Bindings[i].Fact.CouponSourceID, result.Bindings[i].Fact.ProductSourceID}]++
			}
		}
	}
	for i, binding := range result.Bindings {
		if binding.Fact == nil {
			if binding.CouponSourceID > 0 {
				bindingProblems[binding.CouponSourceID] = true
			}
			continue
		}
		if bindingCounts[binding.Fact.SourceID] != 1 {
			result.Bindings[i] = BindingResult{Disposition: DispositionQuarantine, Reason: "coupon_binding_source_ambiguous", CouponSourceID: binding.Fact.CouponSourceID}
			bindingProblems[binding.Fact.CouponSourceID] = true
			continue
		}
		if binding.Disposition == DispositionCandidate && bindingPairs[[2]int64{binding.Fact.CouponSourceID, binding.Fact.ProductSourceID}] != 1 {
			result.Bindings[i].Disposition, result.Bindings[i].Reason = DispositionQuarantine, "coupon_binding_product_ambiguous"
			bindingProblems[binding.Fact.CouponSourceID] = true
		}
	}
	bindingCandidates := map[int64]int{}
	for _, binding := range result.Bindings {
		if binding.Disposition == DispositionCandidate && binding.Fact != nil {
			bindingCandidates[binding.Fact.CouponSourceID]++
		}
	}
	for i, coupon := range result.Coupons {
		if coupon.Disposition != DispositionCandidate || coupon.Fact == nil {
			continue
		}
		if bindingProblems[coupon.Fact.SourceID] || bindingCandidates[coupon.Fact.SourceID] == 0 {
			result.Coupons[i].Disposition, result.Coupons[i].Reason = DispositionArchive, "coupon_binding_unresolved"
		}
	}
	for i, binding := range result.Bindings {
		if binding.Disposition != DispositionCandidate || binding.Fact == nil {
			continue
		}
		for _, coupon := range result.Coupons {
			if coupon.Fact != nil && coupon.Fact.SourceID == binding.Fact.CouponSourceID && coupon.Disposition != DispositionCandidate {
				result.Bindings[i].Disposition, result.Bindings[i].Reason = DispositionArchive, "coupon_not_target_candidate"
				break
			}
		}
	}

	claimCounts := map[int64]int{}
	for i, row := range claims {
		result.Claims[i] = adaptClaim(row, knownCoupons)
		if result.Claims[i].Fact != nil {
			claimCounts[result.Claims[i].Fact.SourceID]++
		}
	}
	knownClaims := map[int64]ClaimFact{}
	for i, claim := range result.Claims {
		if claim.Fact == nil {
			continue
		}
		if claimCounts[claim.Fact.SourceID] != 1 {
			result.Claims[i] = ClaimResult{Disposition: DispositionQuarantine, Reason: "coupon_claim_source_ambiguous"}
			continue
		}
		knownClaims[claim.Fact.SourceID] = *claim.Fact
	}
	for i, row := range redemptions {
		result.Redemptions[i] = adaptRedemption(row, knownClaims)
	}
	redemptionCounts := map[int64]int{}
	for _, redemption := range result.Redemptions {
		if redemption.Fact != nil {
			redemptionCounts[redemption.Fact.SourceID]++
		}
	}
	for i, redemption := range result.Redemptions {
		if redemption.Fact != nil && redemptionCounts[redemption.Fact.SourceID] != 1 {
			result.Redemptions[i] = RedemptionResult{Disposition: DispositionQuarantine, Reason: "coupon_redemption_source_ambiguous"}
		}
	}
	return result
}

func adaptCoupon(row json.RawMessage) CouponResult {
	var source couponJSON
	if !decode(row, &source, "id tenant_id public_slug name discount_amount_total currency status total_issue_limit per_user_issue_limit issued_count claim_starts_at claim_ends_at validity_mode instructions created_by updated_by created_at updated_at") {
		return CouponResult{Disposition: DispositionQuarantine, Reason: "coupon_json_invalid"}
	}
	fact := CouponDefinitionFact{SourceID: source.ID, TenantID: source.TenantID, Name: source.Name, DiscountAmountMinor: source.DiscountAmountMinor, Currency: source.Currency, OriginalStatus: source.Status, TotalIssueLimit: source.TotalIssueLimit, PerUserIssueLimit: source.PerUserIssueLimit, IssuedCount: source.IssuedCount, ClaimStartsAt: source.ClaimStartsAt, ClaimEndsAt: source.ClaimEndsAt, ValidityMode: source.ValidityMode, UseStartsAt: source.UseStartsAt, UseEndsAt: source.UseEndsAt, RelativeValidityDays: source.RelativeValidityDays, Instructions: source.Instructions, FirstClaimAt: source.FirstClaimAt, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
	if !validCouponSource(fact) {
		return CouponResult{Disposition: DispositionQuarantine, Reason: "coupon_shape_invalid"}
	}
	if !validCouponCandidate(fact) {
		return CouponResult{Disposition: DispositionArchive, Reason: "coupon_target_ineligible", Fact: &fact}
	}
	return CouponResult{Disposition: DispositionCandidate, Fact: &fact}
}

func adaptBinding(row json.RawMessage, coupons map[int64]CouponDefinitionFact) BindingResult {
	var source bindingJSON
	if !decode(row, &source, "id tenant_id coupon_id trade_product_id created_at") {
		return BindingResult{Disposition: DispositionQuarantine, Reason: "coupon_binding_json_invalid"}
	}
	if source.ID < 1 || source.TenantID == "" || source.CouponSourceID < 1 || source.ProductSourceID < 1 || source.CreatedAt.IsZero() {
		return BindingResult{Disposition: DispositionQuarantine, Reason: "coupon_binding_shape_invalid", CouponSourceID: source.CouponSourceID}
	}
	fact := BindingFact{SourceID: source.ID, TenantID: source.TenantID, CouponSourceID: source.CouponSourceID, ProductSourceID: source.ProductSourceID, CreatedAt: source.CreatedAt}
	coupon, found := coupons[fact.CouponSourceID]
	if !found || coupon.TenantID != fact.TenantID {
		return BindingResult{Disposition: DispositionQuarantine, Reason: "coupon_binding_coupon_unresolved", CouponSourceID: fact.CouponSourceID}
	}
	if !validCouponCandidate(coupon) {
		return BindingResult{Disposition: DispositionArchive, Reason: "coupon_not_target_candidate", CouponSourceID: fact.CouponSourceID, Fact: &fact}
	}
	return BindingResult{Disposition: DispositionCandidate, CouponSourceID: fact.CouponSourceID, Fact: &fact}
}

func adaptClaim(row json.RawMessage, coupons map[int64]CouponDefinitionFact) ClaimResult {
	var source claimJSON
	if !decode(row, &source, "id tenant_id coupon_id claim_no unionid discount_amount_total currency valid_from valid_until status idempotency_key_hash claimed_at created_at updated_at") {
		return ClaimResult{Disposition: DispositionQuarantine, Reason: "coupon_claim_json_invalid"}
	}
	fact := ClaimFact{SourceID: source.ID, TenantID: source.TenantID, CouponSourceID: source.CouponSourceID, ClaimNumber: source.ClaimNumber, DiscountAmountMinor: source.DiscountAmountMinor, Currency: source.Currency, OriginalStatus: source.Status, ValidFrom: source.ValidFrom, ValidUntil: source.ValidUntil, ClaimedAt: source.ClaimedAt, ReservedAt: source.ReservedAt, ConsumedAt: source.ConsumedAt, ExpiredAt: source.ExpiredAt, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
	if source.ID < 1 || source.TenantID == "" || source.CouponSourceID < 1 || source.DiscountAmountMinor < 0 || !validNonzeroTimes(source.ValidFrom, source.ValidUntil, source.ClaimedAt, source.CreatedAt, source.UpdatedAt) || !validOptionalTimes(source.ReservedAt, source.ConsumedAt, source.ExpiredAt) {
		return ClaimResult{Disposition: DispositionQuarantine, Reason: "coupon_claim_shape_invalid"}
	}
	coupon, found := coupons[fact.CouponSourceID]
	if !found || coupon.TenantID != fact.TenantID {
		return ClaimResult{Disposition: DispositionQuarantine, Reason: "coupon_claim_coupon_unresolved"}
	}
	return ClaimResult{Disposition: DispositionArchive, Fact: &fact}
}

func adaptRedemption(row json.RawMessage, claims map[int64]ClaimFact) RedemptionResult {
	var source redemptionJSON
	if !decode(row, &source, "id tenant_id claim_id order_id out_trade_no status original_amount_total discount_amount_total payable_amount_total currency reserved_until idempotency_key_hash release_reason reserved_at created_at updated_at") {
		return RedemptionResult{Disposition: DispositionQuarantine, Reason: "coupon_redemption_json_invalid"}
	}
	fact := RedemptionFact{SourceID: source.ID, TenantID: source.TenantID, ClaimSourceID: source.ClaimSourceID, OrderSourceID: source.OrderSourceID, OutTradeNo: source.OutTradeNo, OriginalStatus: source.Status, OriginalAmountMinor: source.OriginalAmountMinor, DiscountAmountMinor: source.DiscountAmountMinor, PayableAmountMinor: source.PayableAmountMinor, Currency: source.Currency, ReservedUntil: source.ReservedUntil, ReleaseReason: source.ReleaseReason, ReservedAt: source.ReservedAt, ConsumedAt: source.ConsumedAt, ReleasedAt: source.ReleasedAt, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
	if source.ID < 1 || source.TenantID == "" || source.ClaimSourceID < 1 || source.OrderSourceID < 1 || source.OriginalAmountMinor < 0 || source.DiscountAmountMinor < 0 || source.PayableAmountMinor < 0 || !validNonzeroTimes(source.ReservedUntil, source.ReservedAt, source.CreatedAt, source.UpdatedAt) || !validOptionalTimes(source.ConsumedAt, source.ReleasedAt) {
		return RedemptionResult{Disposition: DispositionQuarantine, Reason: "coupon_redemption_shape_invalid"}
	}
	claim, found := claims[fact.ClaimSourceID]
	if !found || claim.TenantID != fact.TenantID {
		return RedemptionResult{Disposition: DispositionQuarantine, Reason: "coupon_redemption_claim_unresolved"}
	}
	return RedemptionResult{Disposition: DispositionArchive, Fact: &fact}
}

func validCouponSource(value CouponDefinitionFact) bool {
	return value.SourceID > 0 && value.TenantID != "" && value.DiscountAmountMinor >= 0 && value.TotalIssueLimit >= 0 && value.PerUserIssueLimit >= 0 && value.IssuedCount >= 0 && validNonzeroTimes(value.ClaimStartsAt, value.ClaimEndsAt, value.CreatedAt, value.UpdatedAt) && validOptionalTimes(value.UseStartsAt, value.UseEndsAt, value.FirstClaimAt)
}

func validCouponCandidate(value CouponDefinitionFact) bool {
	if !validCouponSource(value) || value.Currency != "CNY" || value.Name == "" || strings.TrimSpace(value.Name) != value.Name || utf8.RuneCountInString(value.Name) > 45 || value.DiscountAmountMinor < 1 || value.TotalIssueLimit < 1 || value.PerUserIssueLimit < 1 || value.PerUserIssueLimit > value.TotalIssueLimit || value.IssuedCount > value.TotalIssueLimit || !value.ClaimEndsAt.After(value.ClaimStartsAt) || strings.TrimSpace(value.Instructions) != value.Instructions || utf8.RuneCountInString(value.Instructions) > 200 || value.UpdatedAt.Before(value.CreatedAt) {
		return false
	}
	if value.IssuedCount == 0 && value.FirstClaimAt != nil || value.IssuedCount > 0 && value.FirstClaimAt == nil {
		return false
	}
	switch value.ValidityMode {
	case "fixed_range":
		return value.UseStartsAt != nil && value.UseEndsAt != nil && value.UseEndsAt.After(*value.UseStartsAt) && value.RelativeValidityDays == nil
	case "relative_days":
		return value.UseStartsAt == nil && value.UseEndsAt == nil && value.RelativeValidityDays != nil && *value.RelativeValidityDays > 0
	default:
		return false
	}
}

func validNonzeroTimes(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() {
			return false
		}
	}
	return true
}

func validOptionalTimes(values ...*time.Time) bool {
	for _, value := range values {
		if value != nil && value.IsZero() {
			return false
		}
	}
	return true
}

func decode(payload json.RawMessage, target any, required string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range strings.Fields(required) {
		value, found := fields[name]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return json.Unmarshal(payload, target) == nil
}
