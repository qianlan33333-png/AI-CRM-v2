package product_acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	membergrid "github.com/qianlan33333-png/AI-CRM-v2/internal/product/membergrid"
)

func TestLaneD2MemberGridRepositoryPG16_14(t *testing.T) {
	pool, ctx := openPool(t)
	code := uniqueCode("member-grid")
	now := time.Now().UTC().Truncate(time.Microsecond)

	var productID, emptyProductID int64
	if err := pool.QueryRow(ctx, `INSERT INTO products (
		product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at
	) VALUES ($1,'周期会员商品','Lane D2 local read',0,'CNY',0,7001,$2,$2) RETURNING id`, code, now).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (
		product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at
	) VALUES ($1,'空会员商品','Lane D2 empty',0,'CNY',0,7001,$2,$2) RETURNING id`, code+"-empty", now).Scan(&emptyProductID); err != nil {
		t.Fatal(err)
	}

	names := []string{"精确客户甲", "精确客户乙", "精确客户丙", "精确客户丁"}
	customerIDs := make([]int64, len(names))
	for index, name := range names {
		extra := fmt.Sprintf(`{"unionid":"raw-union-%d","external_userid":"raw-external-%d","mobile":"1380013800%d"}`, index, index, index)
		if err := pool.QueryRow(ctx, `INSERT INTO customers (name,extra) VALUES ($1,$2::jsonb) RETURNING id`, name, extra).Scan(&customerIDs[index]); err != nil {
			t.Fatal(err)
		}
	}
	var trapCustomerID int64
	if err := pool.QueryRow(ctx, `INSERT INTO customers (name,extra) VALUES (
		'禁止猜测命中的客户',
		'{"unionid":"raw-union-0","external_userid":"raw-external-0","mobile":"13800138000"}'::jsonb
	) RETURNING id`).Scan(&trapCustomerID); err != nil {
		t.Fatal(err)
	}

	orderIDs := make([]int64, len(names))
	for index, customerID := range customerIDs {
		merchantOrder := fmt.Sprintf("%s-order-%d", code, index)
		detailURL := fmt.Sprintf("/orders/%s-%d", code, index)
		if err := pool.QueryRow(ctx, `INSERT INTO order_list_projections (
			provider,provider_label,merchant_order_no,platform_transaction_no,customer_id,
			payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
			product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,
			detail_url,created_at,updated_at
		) VALUES (
			'wechat','微信支付',$1,$2,$3,$4,$5,'unionid',$6,$7,$8,'周期会员商品',0,'CNY','paid','已支付',$9,$10,$10
		) RETURNING id`,
			merchantOrder, "raw-transaction-"+merchantOrder, customerID,
			"raw-payer-"+fmt.Sprint(index), fmt.Sprintf("1380013800%d", index), "raw-union-"+fmt.Sprint(index),
			productID, code, detailURL, now,
		).Scan(&orderIDs[index]); err != nil {
			t.Fatal(err)
		}
	}

	entitlementIDs := make([]int64, len(names))
	for index := range names {
		if index == len(names)-1 {
			revokedAt := now.Add(time.Hour)
			if err := pool.QueryRow(ctx, `INSERT INTO product_local_entitlements (
				product_id,order_id,customer_id,state,version,granted_by,granted_at,revoked_by,revoked_at
			) VALUES ($1,$2,$3,'revoked',2,7001,$4,7001,$5) RETURNING id`,
				productID, orderIDs[index], customerIDs[index], now, revokedAt,
			).Scan(&entitlementIDs[index]); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := pool.QueryRow(ctx, `INSERT INTO product_local_entitlements (
			product_id,order_id,customer_id,state,version,granted_by,granted_at
		) VALUES ($1,$2,$3,'active',1,7001,$4) RETURNING id`,
			productID, orderIDs[index], customerIDs[index], now,
		).Scan(&entitlementIDs[index]); err != nil {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM product_local_entitlements WHERE product_id IN ($1,$2)`, productID, emptyProductID)
		_, _ = pool.Exec(ctx, `DELETE FROM order_list_projections WHERE product_id IN ($1,$2)`, productID, emptyProductID)
		_, _ = pool.Exec(ctx, `DELETE FROM products WHERE id IN ($1,$2)`, productID, emptyProductID)
		allCustomers := append(append([]int64(nil), customerIDs...), trapCustomerID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id = ANY($1::bigint[])`, allCustomers)
	})

	codec, err := membergrid.NewCursorCodec(bytes.Repeat([]byte("lane-d2-pg16-managed-secret-"), 2))
	if err != nil {
		t.Fatal(err)
	}
	service, err := membergrid.NewService(
		platformstore.NewUnitOfWork(pool),
		membergrid.NewRepository(),
		codec,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Query(ctx, membergrid.QueryInput{
		ProductID: productID, State: membergrid.StateAll, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Query(ctx, membergrid.QueryInput{
		ProductID: productID, State: membergrid.StateAll, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" || second.HasMore || second.NextCursor != "" || len(first.Rows) != 2 || len(second.Rows) != 2 {
		t.Fatalf("pages first/second=%+v/%+v", first, second)
	}

	wantIDs := append([]int64(nil), entitlementIDs...)
	sort.Slice(wantIDs, func(left, right int) bool { return wantIDs[left] > wantIDs[right] })
	gotIDs := []int64{
		first.Rows[0].EntitlementID, first.Rows[1].EntitlementID,
		second.Rows[0].EntitlementID, second.Rows[1].EntitlementID,
	}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("same-timestamp order=%v want=%v", gotIDs, wantIDs)
	}

	nameByEntitlement := make(map[int64]string, len(entitlementIDs))
	for index, entitlementID := range entitlementIDs {
		nameByEntitlement[entitlementID] = names[index]
	}
	seen := make(map[int64]struct{}, len(gotIDs))
	for _, row := range append(append([]membergrid.MemberRow(nil), first.Rows...), second.Rows...) {
		if _, duplicate := seen[row.EntitlementID]; duplicate {
			t.Fatalf("duplicate entitlement %d", row.EntitlementID)
		}
		seen[row.EntitlementID] = struct{}{}
		if row.ProductID != productID || row.DisplayName != nameByEntitlement[row.EntitlementID] || row.MaskedMobile != nil {
			t.Fatalf("row escaped exact customer projection: %+v", row)
		}
	}

	active, err := service.Query(ctx, membergrid.QueryInput{
		ProductID: productID, State: membergrid.StateActive, Limit: 50,
	})
	if err != nil || len(active.Rows) != 3 {
		t.Fatalf("active=%+v error=%v", active, err)
	}
	revoked, err := service.Query(ctx, membergrid.QueryInput{
		ProductID: productID, State: membergrid.StateRevoked, Limit: 50,
	})
	if err != nil || len(revoked.Rows) != 1 || revoked.Rows[0].RevokedAt == nil {
		t.Fatalf("revoked=%+v error=%v", revoked, err)
	}

	empty, err := service.Query(ctx, membergrid.QueryInput{
		ProductID: emptyProductID, State: membergrid.StateAll, Limit: 50,
	})
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 || empty.HasMore || empty.NextCursor != "" {
		t.Fatalf("empty=%+v error=%v", empty, err)
	}
	if _, err = service.Query(ctx, membergrid.QueryInput{
		ProductID: math.MaxInt64, State: membergrid.StateAll, Limit: 1,
	}); !errors.Is(err, membergrid.ErrNotFound) {
		t.Fatalf("missing product error=%v", err)
	}

	encoded, err := json.Marshal(append(append([]membergrid.MemberRow(nil), first.Rows...), second.Rows...))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"customer_id", "unionid", "external_userid", "order_id", "granted_by", "revoked_by",
		"receipt", "provider", "1380013800", "raw-union", "raw-external", "raw-payer", "raw-transaction",
		"禁止猜测命中的客户",
	} {
		if strings.Contains(body, strings.ToLower(forbidden)) {
			t.Fatalf("safe projection leaked %q: %s", forbidden, body)
		}
	}
}
