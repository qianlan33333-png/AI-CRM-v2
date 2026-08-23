package order_acceptance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestP4OrderABLocalExportRefundIntentReplayAndOutcomeUnknownGate(t *testing.T) {
	pool, ctx := openOrderPool(t)
	prefix := fmt.Sprintf("p4-order-ab-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	var orderID int64
	err := pool.QueryRow(ctx, `INSERT INTO order_list_projections
      (provider,provider_label,merchant_order_no,platform_transaction_no,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
      VALUES ('wechat','微信支付',$1,$2,'快照客户','13800000000','external_userid',$3,$4,'快照商品',1990,'CNY','paid','已支付',$5,$6,$6)
      RETURNING id`, prefix+"-M", prefix+"-T", prefix+"-identity", prefix+"-SKU", "/api/admin/orders/"+prefix, now).Scan(&orderID)
	if err != nil {
		t.Fatal(err)
	}
	service := orderapp.NewBoardService(platformstore.NewUnitOfWork(pool), orderstore.NewRepository(), eventstore.NewAppender())
	exportKey := "p4-order-ab-export-" + prefix
	export, err := service.CreateExport(ctx, orderport.ExportCommand{Resource: "orders", Format: "csv", Actor: 701, IdempotencyKey: exportKey})
	if err != nil || export.Status != "completed" || export.ContentText == "" || export.DownloadURL == "" {
		t.Fatalf("export=%#v err=%v", export, err)
	}
	if replay, replayErr := service.CreateExport(ctx, orderport.ExportCommand{Resource: "orders", Format: "csv", Actor: 701, IdempotencyKey: exportKey}); replayErr != nil || replay.JobID != export.JobID {
		t.Fatalf("export replay=%#v err=%v", replay, replayErr)
	}
	for _, forbidden := range []string{prefix + "-M", prefix + "-T", prefix + "-identity", "13800000000", "merchant_order_no", "transaction_id"} {
		if strings.Contains(export.ContentText, forbidden) {
			t.Fatalf("safe export leaked %q: %q", forbidden, export.ContentText)
		}
	}
	preview, err := service.PreviewExport(ctx, orderport.ExportCommand{Resource: "orders", Format: "csv", Actor: 701})
	if err != nil || preview.Total < 1 || preview.ContentText == "" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	var exportReceiptCount, exportJobCount, exportEventCount int
	if err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM order_operation_receipts WHERE actor_scope='admin:701'),
      (SELECT count(*) FROM order_export_jobs),
      (SELECT count(*) FROM event_log WHERE event_type='order.export_created')`).Scan(&exportReceiptCount, &exportJobCount, &exportEventCount); err != nil || exportReceiptCount != 1 || exportJobCount != 1 || exportEventCount != 1 {
		t.Fatalf("preview wrote durable facts receipt/job/event=%d/%d/%d err=%v", exportReceiptCount, exportJobCount, exportEventCount, err)
	}

	refundKey := "p4-order-ab-refund-" + prefix
	command := orderport.RefundCommand{Provider: "wechat", OrderReference: prefix + "-M", RefundAmountTotal: 1990, Reason: "重复支付", TransactionIDConfirmation: prefix + "-T", Checked: true, Actor: 701, IdempotencyKey: refundKey}
	refund, err := service.RequestRefund(ctx, command)
	if err != nil || refund.OrderID != orderport.ID(orderID) || refund.Status != "pending_external_gate" || refund.ExternalEffectState != "pending_external_gate" || refund.AutoRetryAllowed {
		t.Fatalf("refund=%#v err=%v", refund, err)
	}
	if replay, replayErr := service.RequestRefund(ctx, command); replayErr != nil || replay.ID != refund.ID || replay.RefundID != refund.RefundID {
		t.Fatalf("refund replay=%#v err=%v", replay, replayErr)
	}
	command.Reason = "不同的理由"
	if _, err = service.RequestRefund(ctx, command); !errors.Is(err, orderapp.ErrBoardConflict) {
		t.Fatalf("same key different refund command error=%v", err)
	}
	var receiptCount, refundCount, effectCount, noAutoRetry, eventCount int
	err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM order_operation_receipts WHERE actor_scope='admin:701'),
      (SELECT count(*) FROM order_refunds WHERE id=$1),
      (SELECT count(*) FROM order_external_effects WHERE id=$2),
      (SELECT count(*) FROM order_external_effects WHERE id=$2 AND auto_retry_allowed=FALSE),
      (SELECT count(*) FROM event_log WHERE event_type='order.refund_requested' AND payload->>'refund_id'=$3)`, refund.ID, refund.ExternalEffectID, refund.RefundID).Scan(&receiptCount, &refundCount, &effectCount, &noAutoRetry, &eventCount)
	if err != nil || receiptCount != 2 || refundCount != 1 || effectCount != 1 || noAutoRetry != 1 || eventCount != 1 {
		t.Fatalf("receipts/refunds/effects/no-auto/event=%d/%d/%d/%d/%d err=%v", receiptCount, refundCount, effectCount, noAutoRetry, eventCount, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE order_external_effects SET state='outcome_unknown', updated_at=updated_at WHERE id=$1`, refund.ExternalEffectID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RequestExternalEffectRetry(ctx, refund.ExternalEffectID, 701, "p4-order-ab-review-"+prefix); !errors.Is(err, orderapp.ErrBoardConflict) {
		t.Fatalf("outcome_unknown must not retry: %v", err)
	}
	var reviewCalls int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE event_type='order.external_effect_retry_requested' AND payload->>'external_effect_id'=($1::bigint)::text`, refund.ExternalEffectID).Scan(&reviewCalls); err != nil || reviewCalls != 0 {
		t.Fatalf("outcome_unknown retry event count=%d err=%v", reviewCalls, err)
	}
}
