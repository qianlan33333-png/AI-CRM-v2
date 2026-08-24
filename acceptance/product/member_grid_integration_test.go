package product_acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	membergrid "github.com/qianlan33333-png/AI-CRM-v2/internal/product/membergrid"
)

// TestLaneD2MemberGridRepositoryPG16_14 proves the grid reads only canonical
// service_period_members, not the coexisting legacy entitlement projection.
func TestLaneD2MemberGridRepositoryPG16_14(t *testing.T) {
	pool, ctx := openPool(t)
	code := uniqueCode("member-grid")
	now := time.Now().UTC().Truncate(time.Microsecond)

	productID := insertGridProduct(t, ctx, pool, code, true, now)
	otherProductID := insertGridProduct(t, ctx, pool, code+"-other", true, now)
	ordinaryProductID := insertGridProduct(t, ctx, pool, code+"-ordinary", false, now)
	var customerIDs [5]int64
	for index := range customerIDs {
		if err := pool.QueryRow(ctx, `INSERT INTO customers (name,extra)
VALUES ($1,$2::jsonb) RETURNING id`,
			fmt.Sprintf("成员客户%d", index),
			fmt.Sprintf(`{"external_userid":"decoy-%d","mobile":"1380013800%d"}`, index, index),
		).Scan(&customerIDs[index]); err != nil {
			t.Fatal(err)
		}
	}

	refs := []string{
		"spm_0000000000000000000004",
		"spm_0000000000000000000003",
		"spm_0000000000000000000002",
		"spm_0000000000000000000001",
	}
	states := []string{"active", "expired", "removed", "active"}
	sources := []string{"manual", "paid_order", "manual", "manual"}
	for index, ref := range refs {
		updatedAt := now
		if index == len(refs)-1 {
			updatedAt = now.Add(-time.Minute)
		}
		var expiredAt, removedAt any
		if states[index] == "expired" {
			expiredAt = updatedAt
		}
		if states[index] == "removed" {
			removedAt = updatedAt
		}
		if _, err := pool.Exec(ctx, `INSERT INTO service_period_members (
	member_ref,service_product_id,customer_id,state,source,starts_at,expired_at,removed_at,
	remark,alliance,version,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'private remark','private alliance',1,$6,$9)`,
			ref, productID, customerIDs[index], states[index], sources[index], now.Add(-time.Hour), expiredAt, removedAt, updatedAt,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO service_period_members (
	member_ref,service_product_id,customer_id,state,source,starts_at,version,created_at,updated_at
) VALUES ('spm_0000000000000000000099',$1,$2,'active','manual',$3,1,$3,$3)`, otherProductID, customerIDs[4], now); err != nil {
		t.Fatal(err)
	}

	var orderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO order_list_projections (
	provider,provider_label,merchant_order_no,customer_id,product_id,product_code,
	product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at
) VALUES ('wechat','legacy decoy',$1,$2,$3,$4,'legacy decoy',0,'CNY','paid','paid',$5,$6,$6)
RETURNING id`, code+"-legacy", customerIDs[4], productID, code, "/orders/"+code, now).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO product_local_entitlements (
	product_id,order_id,customer_id,state,version,granted_by,granted_at
) VALUES ($1,$2,$3,'active',1,7001,$4)`, productID, orderID, customerIDs[4], now); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM product_local_entitlements WHERE order_id=$1`, orderID)
		_, _ = pool.Exec(ctx, `DELETE FROM order_list_projections WHERE id=$1`, orderID)
		_, _ = pool.Exec(ctx, `DELETE FROM service_period_members WHERE service_product_id IN ($1,$2)`, productID, otherProductID)
		_, _ = pool.Exec(ctx, `DELETE FROM products WHERE id IN ($1,$2,$3)`, productID, otherProductID, ordinaryProductID)
		_, _ = pool.Exec(ctx, `DELETE FROM customers WHERE id = ANY($1::bigint[])`, customerIDs[:])
	})

	codec, err := membergrid.NewCursorCodec(bytes.Repeat([]byte("member-grid-pg16-managed-secret"), 2))
	if err != nil {
		t.Fatal(err)
	}
	service, err := membergrid.NewService(platformstore.NewUnitOfWork(pool), membergrid.NewRepository(), codec)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Query(ctx, membergrid.QueryInput{ProductID: productID, State: membergrid.StateAll, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Query(ctx, membergrid.QueryInput{ProductID: productID, State: membergrid.StateAll, Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" || second.HasMore || len(first.Rows) != 2 || len(second.Rows) != 2 {
		t.Fatalf("pages=%+v/%+v", first, second)
	}
	rows := append(append([]membergrid.MemberRow(nil), first.Rows...), second.Rows...)
	for index, row := range rows {
		if row.MemberRef != refs[index] || row.ServiceProductID != productID || row.CustomerID == customerIDs[4] || row.DisplayName == "成员客户4" {
			t.Fatalf("canonical-only row=%+v", row)
		}
	}
	assertCanonicalGridJSON(t, rows)

	for _, testCase := range []struct {
		state  membergrid.StateFilter
		source membergrid.SourceFilter
		want   int
	}{
		{membergrid.StateActive, membergrid.SourceAny, 2},
		{membergrid.StateExpired, membergrid.SourcePaidOrder, 1},
		{membergrid.StateRemoved, membergrid.SourceManual, 1},
	} {
		page, queryErr := service.Query(ctx, membergrid.QueryInput{ProductID: productID, State: testCase.state, Source: testCase.source, Limit: 50})
		if queryErr != nil || len(page.Rows) != testCase.want {
			t.Fatalf("filter=%+v rows=%+v error=%v", testCase, page.Rows, queryErr)
		}
	}
	if _, err = service.Query(ctx, membergrid.QueryInput{ProductID: otherProductID, State: membergrid.StateAll, Limit: 2, Cursor: first.NextCursor}); !errors.Is(err, membergrid.ErrInvalidCursor) {
		t.Fatalf("cross-product cursor error=%v", err)
	}
	if _, err = service.Query(ctx, membergrid.QueryInput{ProductID: productID, State: membergrid.StateAll, Source: membergrid.SourcePaidOrder, Limit: 2, Cursor: first.NextCursor}); !errors.Is(err, membergrid.ErrInvalidCursor) {
		t.Fatalf("cross-source cursor error=%v", err)
	}
	if _, err = service.Query(ctx, membergrid.QueryInput{ProductID: productID, State: membergrid.StateAll, Limit: 3, Cursor: first.NextCursor}); !errors.Is(err, membergrid.ErrInvalidCursor) {
		t.Fatalf("cross-limit cursor error=%v", err)
	}
	if _, err = service.Access(ctx, ordinaryProductID); !errors.Is(err, membergrid.ErrNotFound) {
		t.Fatalf("ordinary product=%v", err)
	}
	if _, err = service.Query(ctx, membergrid.QueryInput{ProductID: math.MaxInt64, State: membergrid.StateAll, Limit: 1}); !errors.Is(err, membergrid.ErrNotFound) {
		t.Fatalf("missing product=%v", err)
	}
}

func insertGridProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string, servicePeriod bool, now time.Time) int64 {
	t.Helper()
	var productID int64
	if !servicePeriod {
		if err := pool.QueryRow(ctx, `INSERT INTO products (
	product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at
) VALUES ($1,'普通商品','ordinary boundary',0,'CNY',0,7001,$2,$2) RETURNING id`, code, now).Scan(&productID); err != nil {
			t.Fatal(err)
		}
		return productID
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (
	product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,legacy_admin_projection
) VALUES ($1,'周期成员 grid','canonical local read',0,'CNY',0,7001,$2,$2,'{"schema_version":1,"status":"service_period_draft","enabled":false}'::jsonb) RETURNING id`, code, now).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	return productID
}

func assertCanonicalGridJSON(t *testing.T, rows []membergrid.MemberRow) {
	t.Helper()
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"remark", "alliance", "mobile", "external_userid", "unionid", "entitlement_id", "granted_at", "revoked_at", "masked_mobile", "order_id", "provider",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("grid JSON leaked %q: %s", forbidden, body)
		}
	}
}
