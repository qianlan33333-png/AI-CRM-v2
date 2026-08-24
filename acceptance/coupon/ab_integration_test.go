package coupon_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/acceptancefixture"
)

func TestP4CouponABClaimConcurrencyReplayAndClaimedRuleFreeze(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	service := realService(pool)
	now := time.Now().UTC()
	runID := fmt.Sprint(time.Now().UnixNano())
	created := p4ABPublishedCoupon(t, ctx, service, product.ID, now, 20, 1)

	const workers = 16
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := service.Claim(ctx, couponport.ClaimCommand{CouponID: created.ID, CustomerID: 811, IdempotencyKey: fmt.Sprintf("p4ab-distinct-claim-%s-%02d", runID, index)})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, couponapp.ErrConflict) {
			t.Fatalf("concurrent claim error=%v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("same-customer successes=%d want 1", successes)
	}

	replayCoupon := p4ABPublishedCoupon(t, ctx, service, product.ID, now, 20, 2)
	replayKey := "p4ab-replay-claim-" + runID
	start = make(chan struct{})
	errs = make(chan error, workers)
	refs := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, err := service.Claim(ctx, couponport.ClaimCommand{CouponID: replayCoupon.ID, CustomerID: 812, IdempotencyKey: replayKey})
			if err == nil {
				refs <- claim.ClaimRef
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(refs)
	var ref string
	for err := range errs {
		if err != nil {
			t.Fatalf("same-key replay error=%v", err)
		}
	}
	for got := range refs {
		if ref == "" {
			ref = got
		} else if got != ref {
			t.Fatalf("replay refs %q and %q differ", ref, got)
		}
	}
	var claims, events int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM coupon_claims WHERE coupon_id=$1),(SELECT count(*) FROM event_log WHERE event_type='coupon.claimed' AND payload->>'coupon_id'=($1::bigint)::text)`, int64(replayCoupon.ID)).Scan(&claims, &events); err != nil || claims != 1 || events != 1 {
		t.Fatalf("replay facts claims/events=%d/%d err=%v", claims, events, err)
	}

	updated := replayCoupon
	updated.TotalIssueLimit++
	updated, err := service.Update(ctx, couponport.UpsertCommand{Coupon: updated, Actor: replayCoupon.UpdatedBy, IdempotencyKey: "p4ab-claimed-limit-" + runID})
	if err != nil || updated.TotalIssueLimit != replayCoupon.TotalIssueLimit+1 {
		t.Fatalf("claimed quantity increase=%#v err=%v", updated, err)
	}
	updated.Name = "不允许改规则"
	if _, err = service.Update(ctx, couponport.UpsertCommand{Coupon: updated, Actor: replayCoupon.UpdatedBy, IdempotencyKey: "p4ab-claimed-rule-" + runID}); !errors.Is(err, couponapp.ErrRulesFrozen) {
		t.Fatalf("claimed rule update error=%v", err)
	}
}

func TestP4CouponABOpaqueSessionsRejectExpiredRevokedReplacedAndCrossGrant(t *testing.T) {
	pool, ctx := openPool(t)
	product := createProduct(t, ctx, pool, "CNY", 9900)
	service := realService(pool)
	now := time.Now().UTC()
	runID := fmt.Sprint(now.UnixNano())
	paymentCustomer, sidebarCustomer := now.UnixNano(), now.UnixNano()+1
	created := p4ABPublishedCoupon(t, ctx, service, product.ID, now, 4, 1)
	active, expired, revoked, replaced, sidebar := p4ABToken("active-"+runID), p4ABToken("expired-"+runID), p4ABToken("revoked-"+runID), p4ABToken("replaced-"+runID), p4ABToken("sidebar-"+runID)
	for _, row := range []struct {
		token    string
		expires  time.Time
		revoked  bool
		replaced bool
	}{
		{active, now.Add(time.Hour), false, false}, {expired, now.Add(-time.Minute), false, false}, {revoked, now.Add(time.Hour), true, false}, {replaced, now.Add(time.Hour), false, true},
	} {
		digest := sha256.Sum256([]byte(row.token))
		createdAt := now
		if !row.expires.After(now) {
			createdAt = row.expires.Add(-time.Hour)
		}
		_, err := pool.Exec(ctx, `INSERT INTO coupon_payment_identity_sessions(token_digest,customer_id,expires_at,revoked_at,replaced_at,created_at) VALUES($1,$2,$3,CASE WHEN $4::boolean THEN $5::timestamptz ELSE NULL END,CASE WHEN $6::boolean THEN $5::timestamptz ELSE NULL END,$5::timestamptz)`, digest[:], paymentCustomer, row.expires, row.revoked, createdAt, row.replaced)
		if err != nil {
			t.Fatal(err)
		}
	}
	digest := sha256.Sum256([]byte(sidebar))
	if _, err := pool.Exec(ctx, `INSERT INTO coupon_sidebar_grants(token_digest,customer_id,expires_at,created_at) VALUES($1,$2,$3,$4)`, digest[:], sidebarCustomer, now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if customer, err := service.ResolvePaymentIdentitySession(ctx, active); err != nil || customer != paymentCustomer {
		t.Fatalf("active payment customer=%d err=%v", customer, err)
	}
	for _, token := range []string{expired, revoked, replaced, sidebar, "customer:" + runID} {
		if _, err := service.ResolvePaymentIdentitySession(ctx, token); !errors.Is(err, couponapp.ErrNotClaimable) {
			t.Fatalf("payment token %q error=%v", token, err)
		}
	}
	if customer, err := service.ResolveSidebarGrant(ctx, sidebar); err != nil || customer != sidebarCustomer {
		t.Fatalf("active sidebar customer=%d err=%v", customer, err)
	}
	if _, err := service.ResolveSidebarGrant(ctx, active); !errors.Is(err, couponapp.ErrNotClaimable) {
		t.Fatalf("payment token accepted as sidebar error=%v", err)
	}
	claim, err := service.Claim(ctx, couponport.ClaimCommand{CouponID: created.ID, CustomerID: paymentCustomer, IdempotencyKey: fmt.Sprintf("p4ab-identity-claim-%d", now.UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	if available, listErr := service.ListAvailable(ctx, fmt.Sprintf("standard_product:%d", product.ID), paymentCustomer); listErr != nil || len(available) != 0 {
		t.Fatalf("claimed customer availability=%#v err=%v", available, listErr)
	}
	rows, err := service.ListSidebarCoupons(ctx, paymentCustomer)
	if err != nil || len(rows) != 1 || rows[0].ClaimRef != claim.ClaimRef || rows[0].CouponID != created.ID {
		t.Fatalf("sidebar rows=%#v err=%v", rows, err)
	}
}

func TestP4CouponABS200KAvailableLookupUsesTargetIndex(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	prefix := fmt.Sprintf("p4ab-plan-%d-", time.Now().UnixNano())
	if _, err = tx.Exec(ctx, `INSERT INTO coupons(name,discount_amount_total,total_issue_limit,per_user_issue_limit,claim_starts_at,claim_ends_at,validity_mode,relative_validity_days,instructions,created_by,updated_by,created_at,updated_at,status) SELECT $1||g,1,1,1,now()-interval '1 day',now()+interval '1 day','relative_days',30,'',771,771,now(),now(),'published' FROM generate_series(1,200000) g`, prefix); err != nil {
		t.Fatal(err)
	}
	// Keep each target reference selective for the index plan while satisfying
	// the product-reference FK on a fresh PG16 acceptance database. The whole
	// performance fixture is rolled back with this transaction.
	var couponIDs []int64
	if err = tx.QueryRow(ctx, `SELECT coalesce(array_agg(id ORDER BY id), '{}'::bigint[]) FROM coupons WHERE name LIKE $1`, prefix+"%").Scan(&couponIDs); err != nil {
		t.Fatal(err)
	}
	if len(couponIDs) != 200000 {
		t.Fatalf("coupon ids=%d, want 200000", len(couponIDs))
	}
	for _, invalidIDs := range [][]int64{
		{0},
		{couponIDs[1], couponIDs[0]},
		{couponIDs[0], couponIDs[0]},
	} {
		if err = productfixture.CreateCouponTargetProducts(ctx, tx, invalidIDs); err == nil {
			t.Fatalf("invalid Product IDs accepted: %v", invalidIDs)
		}
	}
	if err = productfixture.CreateCouponTargetProducts(ctx, tx, couponIDs); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO coupon_targets(coupon_id,position,target_ref,product_id) SELECT id,0,'standard_product:'||id,id FROM coupons WHERE name LIKE $1`, prefix+"%"); err != nil {
		t.Fatal(err)
	}
	var target string
	if err = tx.QueryRow(ctx, `SELECT target_ref FROM coupon_targets WHERE coupon_id=(SELECT max(id) FROM coupons WHERE name LIKE $1)`, prefix+"%").Scan(&target); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE coupons; ANALYZE coupon_targets`); err != nil {
		t.Fatal(err)
	}
	plan := explainP4AB(t, ctx, tx, `EXPLAIN (FORMAT JSON,COSTS OFF) SELECT c.id FROM coupons c JOIN coupon_targets target ON target.coupon_id=c.id AND target.target_ref=$1 WHERE c.status='published' AND c.issued_count<c.total_issue_limit AND c.claim_starts_at<=now() AND c.claim_ends_at>now() ORDER BY c.id LIMIT 200`, target)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) && strings.Contains(plan, `"Relation Name": "coupon_targets"`) {
		t.Fatalf("coupon target lookup must not seq scan at 200k: %s", plan)
	}
	if !strings.Contains(plan, "coupon_targets_target_ref_coupon_id") {
		t.Fatalf("target lookup did not use required index: %s", plan)
	}
}

func p4ABPublishedCoupon(t *testing.T, ctx context.Context, service *couponapp.Service, productID productport.ID, now time.Time, total, perUser int64) couponport.Coupon {
	t.Helper()
	command := validCommand(productID, now)
	command.Name = fmt.Sprintf("P4AB优惠券-%d", time.Now().UnixNano())
	command.TotalIssueLimit, command.PerUserIssueLimit = total, perUser
	command.ClaimStartsAt, command.ClaimEndsAt = now.Add(-time.Hour), now.Add(time.Hour)
	command.IdempotencyKey = fmt.Sprintf("p4ab-create-%d", time.Now().UnixNano())
	created, err := service.Create(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Publish(ctx, created.ID, created.UpdatedBy, fmt.Sprintf("p4ab-publish-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func p4ABToken(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func explainP4AB(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) string {
	t.Helper()
	var plan string
	if err := tx.QueryRow(ctx, query, args...).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	return plan
}
