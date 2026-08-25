package order_acceptance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
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

type acceptanceWeChatShopProvider struct{ requestCalls int }

func (*acceptanceWeChatShopProvider) Enabled() bool { return true }
func (provider *acceptanceWeChatShopProvider) RequestRefund(context.Context, orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	provider.requestCalls++
	return orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: sha256.Sum256([]byte("fake-shop-provider-acceptance"))}, nil
}
func (*acceptanceWeChatShopProvider) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	return orderport.WeChatShopRefundQueryResult{}, orderport.ErrProviderOutcomeUnknown
}
