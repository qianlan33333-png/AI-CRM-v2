package order_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/http"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/order/provider"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCommerceRefundV2WeChatShopProviderAcceptanceIsNotDeliveryProof(t *testing.T) {
	pool, ctx := openPE01Pool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	prefix := fmt.Sprintf("wechat-shop-refund-%d", now.UnixNano())
	var orderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO order_list_projections
      (provider,provider_label,merchant_order_no,platform_transaction_no,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
      VALUES ('wechat_shop','微信小店',$1,$2,'快照客户','13800000000','external_userid',$3,$4,'快照商品',880,'CNY','paid','已支付',$5,$6,$6)
      RETURNING id`, prefix+"-M", prefix+"-T", prefix+"-identity", prefix+"-SKU", "/api/admin/orders/"+prefix, now).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	repository, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	provider := &acceptanceWeChatShopProvider{}
	service, err := orderapp.NewWeChatShopRefundService(platformstore.NewUnitOfWork(pool), repository, provider, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	command := orderport.WeChatShopRefundCommand{OrderReference: prefix + "-M", TransactionIDConfirmation: prefix + "-T", AmountMinor: 880, Reason: "acceptance return", Checked: true, Actor: 702, IdempotencyKey: prefix + "/refund-key"}
	refund, err := service.RequestRefund(ctx, command)
	if err != nil || refund.OrderID != orderport.ID(orderID) || refund.State != orderport.WeChatShopRefundAccepted || provider.requestCalls != 0 {
		t.Fatalf("reservation=%+v calls=%d error=%v", refund, provider.requestCalls, err)
	}

	job := orderport.WeChatShopExecutionJob{RefundID: refund.ID, RiverJobID: now.UnixNano(), RiverAttempt: 1, ArgsDigest: sha256.Sum256([]byte(prefix + "/job")), ScheduledAt: now.Add(time.Second)}
	refund, err = service.ExecuteRefund(ctx, job)
	if err != nil || refund.State != orderport.WeChatShopRefundProviderAccepted || provider.requestCalls != 1 || refund.SettledAt.IsZero() == false {
		t.Fatalf("provider acceptance=%+v calls=%d error=%v", refund, provider.requestCalls, err)
	}
	if _, err = service.ExecuteRefund(ctx, job); err != nil || provider.requestCalls != 1 {
		t.Fatalf("provider-accepted execution replay calls=%d error=%v", provider.requestCalls, err)
	}

	refund, err = service.ApplyRefundCallback(ctx, orderport.WeChatShopRefundCallbackCommand{OutRefundNo: refund.OutRefundNo, ProviderEventDigest: sha256.Sum256([]byte(prefix + "/event")), PayloadDigest: sha256.Sum256([]byte(prefix + "/payload")), ProviderRefundDigest: sha256.Sum256([]byte(prefix + "/provider-refund")), AmountMinor: 880, Currency: "CNY", Succeeded: true, OccurredAt: now.Add(2 * time.Second)})
	if err != nil || refund.State != orderport.WeChatShopRefundSucceeded || refund.SettledAt.IsZero() {
		t.Fatalf("delivery proof=%+v error=%v", refund, err)
	}

	queryOrderID := insertCommerceRefundShopOrder(t, ctx, pool, prefix+"-Q-M", prefix+"-Q-T", 660, now)
	queryProvider := &acceptanceWeChatShopQueryProvider{at: now.Add(5 * time.Second)}
	queryService, err := orderapp.NewWeChatShopRefundService(platformstore.NewUnitOfWork(pool), repository, queryProvider, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	queryRefund, err := queryService.RequestRefund(ctx, orderport.WeChatShopRefundCommand{OrderReference: prefix + "-Q-M", TransactionIDConfirmation: prefix + "-Q-T", AmountMinor: 660, Reason: "acceptance query", Checked: true, Actor: 703, IdempotencyKey: prefix + "/query-refund-key"})
	if err != nil || queryRefund.OrderID != orderport.ID(queryOrderID) {
		t.Fatalf("query reservation=%+v error=%v", queryRefund, err)
	}
	queryRefund, err = queryService.ExecuteRefund(ctx, orderport.WeChatShopExecutionJob{RefundID: queryRefund.ID, RiverJobID: now.UnixNano() + 1, RiverAttempt: 1, ArgsDigest: sha256.Sum256([]byte(prefix + "/query-job")), ScheduledAt: now.Add(time.Second)})
	if err != nil || queryRefund.State != orderport.WeChatShopRefundOutcomeUnknown {
		t.Fatalf("query unknown=%+v error=%v", queryRefund, err)
	}
	queryRefund, err = queryService.ReconcileRefund(ctx, queryRefund.ID)
	if err != nil || queryRefund.State != orderport.WeChatShopRefundSucceeded {
		t.Fatalf("query reconcile=%+v error=%v", queryRefund, err)
	}

	var canonical, attempts, callbacks, legacyRefunds, legacyEffects int
	if err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM order_wechat_shop_refunds WHERE order_id=$1 AND state='succeeded'),
      (SELECT count(*) FROM order_wechat_shop_refund_attempts WHERE refund_id=$2 AND outcome='provider_accepted'),
      (SELECT count(*) FROM order_wechat_shop_refund_callbacks WHERE refund_id=$2 AND state='completed' AND outcome='applied'),
      (SELECT count(*) FROM order_refunds WHERE order_id=$1),
      (SELECT count(*) FROM order_external_effects WHERE order_id=$1)`, orderID, refund.ID).Scan(&canonical, &attempts, &callbacks, &legacyRefunds, &legacyEffects); err != nil {
		t.Fatal(err)
	}
	if canonical != 1 || attempts != 1 || callbacks != 1 || legacyRefunds != 0 || legacyEffects != 0 {
		t.Fatalf("canonical/attempt/callback=%d/%d/%d legacy=%d/%d", canonical, attempts, callbacks, legacyRefunds, legacyEffects)
	}
}

func TestCommerceRefundV2DisabledProductionHTTPPathReplaysConflictsAndNeverQueues(t *testing.T) {
	pool, ctx := openPE01Pool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	prefix := fmt.Sprintf("wechat-shop-disabled-http-%d", now.UnixNano())
	orderID := insertCommerceRefundShopOrder(t, ctx, pool, prefix+"-M", prefix+"-T", 880, now)
	concurrentOrderID := insertCommerceRefundShopOrder(t, ctx, pool, prefix+"-C-M", prefix+"-C-T", 1000, now)
	repository, err := orderstore.NewCommerceRefundRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := orderapp.NewWeChatShopRefundService(platformstore.NewUnitOfWork(pool), repository, orderprovider.DisabledWeChatShopRefund{}, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := orderhttp.NewCommerceRefundHandler(disabledAcceptancePay{}, service, orderprovider.DisabledWeChatShopCallbackVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	var riverBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.river_job`).Scan(&riverBefore); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"provider":"wechat_shop","order_no":%q,"refund_amount_total":880,"reason":"disabled production acceptance","transaction_id_confirmation":%q,"checked":true}`, prefix+"-M", prefix+"-T")
	key := prefix + "-idempotency-key"
	first := commerceRefundShopRequest(t, handler, actorForCommerceRefund(t, 8601), key, body)
	second := commerceRefundShopRequest(t, handler, actorForCommerceRefund(t, 8601), key, body)
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted || first.Body.String() != second.Body.String() {
		t.Fatalf("first=%d second=%d replay_equal=%t first_body=%s second_body=%s", first.Code, second.Code, first.Body.String() == second.Body.String(), first.Body.String(), second.Body.String())
	}
	changed := commerceRefundShopRequest(t, handler, actorForCommerceRefund(t, 8601), key, strings.Replace(body, `"refund_amount_total":880`, `"refund_amount_total":879`, 1))
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed payload status=%d body=%s", changed.Code, changed.Body.String())
	}
	var accepted map[string]any
	if err = json.Unmarshal(first.Body.Bytes(), &accepted); err != nil || accepted["state"] != string(orderport.WeChatShopRefundAccepted) || accepted["provider_accepted"] != false || accepted["delivery_proven"] != false {
		t.Fatalf("accepted response=%v error=%v", accepted, err)
	}
	refundID := int64(accepted["id"].(float64))

	callback := httptest.NewRequest(http.MethodPost, orderhttp.WeChatShopCallbackPath, strings.NewReader(`{}`))
	callback.Header.Set("Content-Type", "application/json")
	callback.Header.Set("Wechatshop-Timestamp", "1756100000")
	callback.Header.Set("Wechatshop-Nonce", "nonce")
	callback.Header.Set("Wechatshop-Serial", "serial")
	callback.Header.Set("Wechatshop-Signature", "signature")
	callbackResponse := httptest.NewRecorder()
	handler.WeChatShopCallback(callbackResponse, callback)
	if callbackResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled callback status=%d body=%s", callbackResponse.Code, callbackResponse.Body.String())
	}
	reconcile := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/wechat-shop/refunds/%d/reconcile", refundID), strings.NewReader(`{}`))
	reconcile.Header.Set("Content-Type", "application/json")
	reconcile = reconcile.WithContext(actorForCommerceRefund(t, 8601))
	reconcileResponse := httptest.NewRecorder()
	handler.ReconcileWeChatShopRefund(reconcileResponse, reconcile, fmt.Sprint(refundID))
	if reconcileResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled reconcile status=%d body=%s", reconcileResponse.Code, reconcileResponse.Body.String())
	}

	concurrentBody := func() string {
		return fmt.Sprintf(`{"provider":"wechat_shop","order_no":%q,"refund_amount_total":700,"reason":"concurrent cap","transaction_id_confirmation":%q,"checked":true}`, prefix+"-C-M", prefix+"-C-T")
	}
	responses := make([]*httptest.ResponseRecorder, 2)
	keys := []string{prefix + "-concurrent-key-a", prefix + "-concurrent-key-b"}
	contexts := []context.Context{actorForCommerceRefund(t, 8602), actorForCommerceRefund(t, 8603)}
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			responses[index] = commerceRefundShopRequest(t, handler, contexts[index], keys[index], concurrentBody())
		}(index)
	}
	wait.Wait()
	acceptedCount, conflictCount := 0, 0
	for _, response := range responses {
		switch response.Code {
		case http.StatusAccepted:
			acceptedCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("concurrent status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if acceptedCount != 1 || conflictCount != 1 {
		t.Fatalf("concurrent accepted=%d conflict=%d", acceptedCount, conflictCount)
	}
	var refundCount, reservedAmount, riverAfter int
	if err = pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(amount_minor),0) FROM public.order_wechat_shop_refunds WHERE order_id=$1`, concurrentOrderID).Scan(&refundCount, &reservedAmount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.river_job`).Scan(&riverAfter); err != nil {
		t.Fatal(err)
	}
	if refundCount != 1 || reservedAmount != 700 || riverAfter != riverBefore {
		t.Fatalf("order=%d refunds=%d reserved=%d river=%d->%d", orderID, refundCount, reservedAmount, riverBefore, riverAfter)
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

func actorForCommerceRefund(t *testing.T, actor int64) context.Context {
	t.Helper()
	ctx := authport.WithAuthenticatedSession(context.Background(), authport.Principal{AdminUserID: actor, Role: authport.RoleAdmin}, authport.SessionRef("refund86-acceptance"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOrderWrite, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func commerceRefundShopRequest(t *testing.T, handler *orderhttp.CommerceRefundHandler, ctx context.Context, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/refunds", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	handler.WeChatShopCompatibility(response, request)
	return response
}

type disabledAcceptancePay struct{}

func (disabledAcceptancePay) RequestWeChatPayRefundV2(context.Context, orderport.WeChatPayRefundCompatibilityCommand) (orderport.RefundV2, error) {
	return orderport.RefundV2{}, orderport.ErrCommerceRefundUnavailable
}

type acceptanceWeChatShopProvider struct{ requestCalls int }

func (*acceptanceWeChatShopProvider) Enabled() bool { return true }
func (provider *acceptanceWeChatShopProvider) RequestRefund(context.Context, orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	provider.requestCalls++
	return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: sha256.Sum256([]byte("fake-shop-provider-acceptance"))}, nil
}
func (*acceptanceWeChatShopProvider) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	return orderport.WeChatShopRefundQueryResult{}, orderport.ErrProviderOutcomeUnknown
}

type acceptanceWeChatShopQueryProvider struct{ at time.Time }

func (*acceptanceWeChatShopQueryProvider) Enabled() bool { return true }
func (*acceptanceWeChatShopQueryProvider) RequestRefund(context.Context, orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown}, orderport.ErrProviderOutcomeUnknown
}
func (provider *acceptanceWeChatShopQueryProvider) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	return orderport.WeChatShopRefundQueryResult{Confirmed: true, EvidenceDigest: sha256.Sum256([]byte("fake-shop-query-evidence")), ProviderRefundDigest: sha256.Sum256([]byte("fake-shop-query-refund")), AmountMinor: 660, Currency: "CNY", OccurredAt: provider.at}, nil
}
