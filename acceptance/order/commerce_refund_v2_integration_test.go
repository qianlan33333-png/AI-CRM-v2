package order_acceptance

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCommerceRefundV2ProviderMaterialCallbackAndQueryClosure(t *testing.T) {
	pool, ctx := openPE01Pool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	prefix := fmt.Sprintf("wechat-shop-refund-96-%d", now.UnixNano())
	orderNo, transactionNo := prefix+"-M", prefix+"-T"
	orderID := insertCommerceRefundShopOrder(t, ctx, pool, orderNo, transactionNo, 880, now)
	repository, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	provider := &acceptanceWeChatShopProvider{afterSaleID: fmt.Sprint(now.UnixNano()), occurredAt: now.Add(3 * time.Second)}
	service, err := orderapp.NewWeChatShopRefundService(platformstore.NewUnitOfWork(pool), repository, provider, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	command := orderport.WeChatShopRefundCommand{
		OrderReference: orderNo, TransactionIDConfirmation: transactionNo,
		ProductID: "product-1", SKUID: "sku-1", Count: 1, AmountMinor: 880,
		ReasonCode: "10000000", Reason: "acceptance return", Checked: true,
		Actor: 702, IdempotencyKey: prefix + "/refund-key",
	}

	for attempt := 0; attempt < 2; attempt++ {
		if _, requestErr := service.RequestRefund(ctx, command); !errors.Is(requestErr, orderport.ErrWeChatShopMaterialUnavailable) {
			t.Fatalf("material attempt=%d error=%v", attempt, requestErr)
		}
	}
	var refundsBefore, syncRequests, syncJobs int
	if err = pool.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM order_wechat_shop_refunds WHERE order_id=$1),
	  (SELECT count(*) FROM order_wechat_shop_material_sync_requests WHERE provider_order_id=$2 AND state='queued'),
	  (SELECT count(*) FROM river_job WHERE kind='order_wechat_shop_material_sync')`, orderID, orderNo).Scan(&refundsBefore, &syncRequests, &syncJobs); err != nil {
		t.Fatal(err)
	}
	if refundsBefore != 0 || syncRequests != 1 || syncJobs != 1 {
		t.Fatalf("before material refunds=%d requests=%d jobs=%d", refundsBefore, syncRequests, syncJobs)
	}

	material := readyAcceptanceShopMaterial(now.Add(time.Second), orderNo, transactionNo, 880)
	materialRepository := orderstore.NewWeChatShopMaterialRepository()
	if err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		changed, upsertErr := materialRepository.UpsertWeChatShopOrderMaterial(tx, material)
		if upsertErr == nil && !changed {
			return errors.New("ready material did not change")
		}
		return upsertErr
	}); err != nil {
		t.Fatal(err)
	}

	refund, err := service.RequestRefund(ctx, command)
	if err != nil || refund.OrderID != orderport.ID(orderID) || refund.State != orderport.WeChatShopRefundAccepted || provider.requestCalls != 0 {
		t.Fatalf("reservation=%+v calls=%d error=%v", refund, provider.requestCalls, err)
	}
	replay, err := service.RequestRefund(ctx, command)
	if err != nil || replay.ID != refund.ID {
		t.Fatalf("reservation replay=%+v error=%v", replay, err)
	}

	job := orderport.WeChatShopExecutionJob{RefundID: refund.ID, RiverJobID: now.UnixNano(), RiverAttempt: 1, ArgsDigest: sha256.Sum256([]byte(prefix + "/job")), ScheduledAt: now.Add(2 * time.Second)}
	refund, err = service.ExecuteRefund(ctx, job)
	if err != nil || refund.State != orderport.WeChatShopRefundProviderAccepted || refund.ProviderAfterSaleID != provider.afterSaleID || provider.requestCalls != 1 || !refund.SettledAt.IsZero() {
		t.Fatalf("provider acceptance=%+v request=%+v calls=%d error=%v", refund, provider.request, provider.requestCalls, err)
	}
	if provider.request.ProviderOrderID != orderNo || provider.request.ProductID != command.ProductID || provider.request.SKUID != command.SKUID || provider.request.Count != command.Count || provider.request.AmountMinor != command.AmountMinor || provider.request.ReasonCode != command.ReasonCode {
		t.Fatalf("provider request lost exact material: %+v", provider.request)
	}
	if _, err = service.ExecuteRefund(ctx, job); err != nil || provider.requestCalls != 1 {
		t.Fatalf("provider-accepted replay calls=%d error=%v", provider.requestCalls, err)
	}

	callback := orderport.WeChatShopRefundCallbackCommand{
		AfterSaleID: provider.afterSaleID, ProviderOrderID: orderNo, ProviderStatus: "MERCHANT_REFUND_SUCCESS",
		ProviderEventDigest: sha256.Sum256([]byte(prefix + "/event")), PayloadDigest: sha256.Sum256([]byte(prefix + "/payload")), OccurredAt: now.Add(3 * time.Second),
	}
	queued, err := service.ApplyRefundCallback(ctx, callback)
	if err != nil || queued.State != orderport.WeChatShopRefundProviderAccepted || !queued.SettledAt.IsZero() {
		t.Fatalf("callback queued=%+v error=%v", queued, err)
	}
	var reconcileJobsBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='order_wechat_shop_refund_reconcile'`).Scan(&reconcileJobsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApplyRefundCallback(ctx, callback); err != nil {
		t.Fatalf("callback replay error=%v", err)
	}
	var reconcileJobsAfter int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='order_wechat_shop_refund_reconcile'`).Scan(&reconcileJobsAfter); err != nil || reconcileJobsAfter != reconcileJobsBefore {
		t.Fatalf("callback replay jobs=%d->%d error=%v", reconcileJobsBefore, reconcileJobsAfter, err)
	}

	provider.query = orderport.WeChatShopRefundQueryResult{
		EvidenceDigest: sha256.Sum256([]byte(prefix + "/query-evidence")), ProviderRefundDigest: sha256.Sum256([]byte(prefix + "/provider-refund")),
		AfterSaleID: provider.afterSaleID, ProviderOrderID: orderNo, ProductID: command.ProductID, SKUID: command.SKUID,
		Count: command.Count, AmountMinor: command.AmountMinor, Currency: "CNY", Type: "REFUND", Status: "MERCHANT_REFUND_SUCCESS", OccurredAt: provider.occurredAt,
	}
	refund, err = service.ReconcileRefund(ctx, refund.ID)
	if err != nil || refund.State != orderport.WeChatShopRefundSucceeded || refund.SettledAt.IsZero() || provider.queryCalls != 1 {
		t.Fatalf("query settlement=%+v calls=%d error=%v", refund, provider.queryCalls, err)
	}

	var canonical, attempts, callbacks, queries, legacyRefunds, legacyEffects, completedSync int
	if err = pool.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM order_wechat_shop_refunds WHERE id=$1 AND contract_version='provider/v2' AND state='succeeded' AND provider_aftersale_id=$2),
	  (SELECT count(*) FROM order_wechat_shop_refund_attempts WHERE refund_id=$1 AND outcome='provider_accepted'),
	  (SELECT count(*) FROM order_wechat_shop_refund_callbacks WHERE refund_id=$1 AND contract_version='provider/v2' AND state='completed' AND outcome='query_queued' AND river_job_id IS NOT NULL),
	  (SELECT count(*) FROM order_wechat_shop_refund_queries WHERE refund_id=$1 AND outcome='applied'),
	  (SELECT count(*) FROM order_refunds WHERE order_id=$3),
	  (SELECT count(*) FROM order_external_effects WHERE order_id=$3),
	  (SELECT count(*) FROM order_wechat_shop_material_sync_requests WHERE provider_order_id=$4 AND state='completed')`, refund.ID, provider.afterSaleID, orderID, orderNo).Scan(&canonical, &attempts, &callbacks, &queries, &legacyRefunds, &legacyEffects, &completedSync); err != nil {
		t.Fatal(err)
	}
	if canonical != 1 || attempts != 1 || callbacks != 1 || queries != 1 || legacyRefunds != 0 || legacyEffects != 0 || completedSync != 1 {
		t.Fatalf("canonical/attempt/callback/query=%d/%d/%d/%d legacy=%d/%d sync=%d", canonical, attempts, callbacks, queries, legacyRefunds, legacyEffects, completedSync)
	}
}

func TestCommerceRefundV2UnknownProviderResponseEvidencePersists(t *testing.T) {
	pool, ctx := openPE01Pool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	prefix := fmt.Sprintf("wechat-shop-refund-96-unknown-%d", now.UnixNano())
	orderNo, transactionNo := prefix+"-M", prefix+"-T"
	insertCommerceRefundShopOrder(t, ctx, pool, orderNo, transactionNo, 660, now)
	repository, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	evidence := sha256.Sum256([]byte(prefix + "/unknown-response"))
	provider := &acceptanceWeChatShopProvider{requestResult: orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown, EvidenceDigest: evidence}}
	service, err := orderapp.NewWeChatShopRefundService(platformstore.NewUnitOfWork(pool), repository, provider, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	material := readyAcceptanceShopMaterial(now, orderNo, transactionNo, 660)
	if err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, upsertErr := orderstore.NewWeChatShopMaterialRepository().UpsertWeChatShopOrderMaterial(tx, material)
		return upsertErr
	}); err != nil {
		t.Fatal(err)
	}
	command := orderport.WeChatShopRefundCommand{
		OrderReference: orderNo, TransactionIDConfirmation: transactionNo,
		ProductID: "product-1", SKUID: "sku-1", Count: 1, AmountMinor: 660,
		ReasonCode: "10000000", Reason: "unknown response", Checked: true,
		Actor: 703, IdempotencyKey: prefix + "/refund-key",
	}
	refund, err := service.RequestRefund(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	job := orderport.WeChatShopExecutionJob{RefundID: refund.ID, RiverJobID: now.UnixNano(), RiverAttempt: 1, ArgsDigest: sha256.Sum256([]byte(prefix + "/job")), ScheduledAt: now}
	refund, err = service.ExecuteRefund(ctx, job)
	if err != nil || refund.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 1 {
		t.Fatalf("refund=%+v calls=%d error=%v", refund, provider.requestCalls, err)
	}
	var outcome string
	var persisted []byte
	if err = pool.QueryRow(ctx, `SELECT outcome, evidence_digest FROM order_wechat_shop_refund_attempts WHERE refund_id=$1`, refund.ID).Scan(&outcome, &persisted); err != nil {
		t.Fatal(err)
	}
	if outcome != "outcome_unknown" || string(persisted) != string(evidence[:]) {
		t.Fatalf("outcome=%s evidence=%x", outcome, persisted)
	}
	if _, err = service.ExecuteRefund(ctx, job); err != nil || provider.requestCalls != 1 {
		t.Fatalf("replay calls=%d error=%v", provider.requestCalls, err)
	}
}

func insertCommerceRefundShopOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, merchant, transaction string, amount int64, now time.Time) int64 {
	t.Helper()
	var orderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO order_list_projections
	  (provider,provider_label,merchant_order_no,platform_transaction_no,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
	  VALUES ('wechat_shop','微信小店',$1,$2,'快照客户','13800000000','external_userid',$3,$4,'快照商品',$5,'CNY','paid','已支付',$6,$7,$7)
	  RETURNING id`, merchant, transaction, merchant+"-identity", merchant+"-SKU", amount, "/api/admin/orders/"+merchant, now).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	return orderID
}

func readyAcceptanceShopMaterial(now time.Time, orderNo, transactionNo string, amount int64) orderport.WeChatShopOrderMaterial {
	return orderport.WeChatShopOrderMaterial{
		ProviderOrderID: orderNo, StatusCode: 20, DealRecorded: true, AmountMinor: amount, Currency: "CNY",
		TransactionDigest: sha256.Sum256([]byte("wechat-shop/transaction/v1\x00" + transactionNo)), EvidenceDigest: sha256.Sum256([]byte("acceptance/material\x00" + orderNo)),
		Source: orderport.WeChatShopMaterialProvider, Readiness: orderport.WeChatShopMaterialReady, ProviderVerified: true,
		CreatedAt: now.Add(-time.Hour), PaidAt: now.Add(-30 * time.Minute), UpdatedAt: now, SyncedAt: now,
		Lines: []orderport.WeChatShopOrderLine{{Position: 1, ProductID: "product-1", SKUID: "sku-1", SKUCount: 1, RealPriceMinor: amount, RemainingSKUCount: 1, AfterSaleEvidenceExact: true, Readiness: orderport.WeChatShopLineReady}},
	}
}

type acceptanceWeChatShopProvider struct {
	afterSaleID   string
	occurredAt    time.Time
	request       orderport.WeChatShopRefundRequest
	requestResult orderport.WeChatShopProviderResult
	query         orderport.WeChatShopRefundQueryResult
	requestCalls  int
	queryCalls    int
}

func (*acceptanceWeChatShopProvider) Enabled() bool { return true }

func (provider *acceptanceWeChatShopProvider) RequestRefund(_ context.Context, request orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	provider.requestCalls++
	provider.request = request
	if provider.requestResult.Completion != "" {
		return provider.requestResult, nil
	}
	return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: sha256.Sum256([]byte("fake-shop-provider-acceptance")), AfterSaleID: provider.afterSaleID}, nil
}

func (provider *acceptanceWeChatShopProvider) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	provider.queryCalls++
	return provider.query, nil
}
