package order_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eerfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store/acceptancefixture"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	productfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/acceptancefixture"
)

func TestPE01FakeWeChatPayOutcomeUnknownReconcilesAndCompensates(t *testing.T) {
	pool, ctx := openPE01Pool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	prefix := fmt.Sprintf("pe01-%d", now.UnixNano())
	customerID, err := contactfixture.CreateCustomerRecord(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	productID, err := productfixture.CreatePE01Product(ctx, pool, prefix, now)
	if err != nil {
		t.Fatal(err)
	}

	financial, err := orderstore.NewFinancialRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	benefits, err := productapp.NewPaidSettlementService(productstore.NewPaidSettlementRepository(), eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := orderapp.NewSettlementService(platformstore.NewUnitOfWork(pool), financial, productstore.NewCatalogRepository(), benefits, eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pe01FakeRuntime{pool: pool}
	provider := &pe01FakeProvider{now: now.Add(time.Second)}
	execution, err := orderapp.NewEffectExecutionService(platformstore.NewUnitOfWork(pool), financial, runtime, provider, settlement)
	if err != nil {
		t.Fatal(err)
	}

	identity := sha256.Sum256([]byte(prefix + "/payer"))
	checkoutCommand := orderport.CheckoutCommand{CustomerID: customerID, ProductID: productID, ProductKind: orderport.ProductKindOrdinary, PaymentIdentityDigest: identity, ActorScope: "payment-session:" + hex.EncodeToString(identity[:]), IdempotencyKey: prefix + "/checkout-key"}
	checkout, err := settlement.Checkout(ctx, checkoutCommand)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := settlement.Checkout(ctx, checkoutCommand)
	if err != nil || replayed.OrderID != checkout.OrderID || replayed.PaymentCommandID != checkout.PaymentCommandID {
		t.Fatalf("checkout replay=%+v error=%v", replayed, err)
	}
	job := orderapp.EffectJob{RecordID: checkout.PaymentCommandID, RiverJobID: 9001, RiverGeneration: 1, RiverQueue: "critical", RiverArgsDigest: sha256.Sum256([]byte(prefix + "/payment-job")), ScheduledAt: now}
	if err = execution.ExecutePayment(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err = execution.ReconcilePayment(ctx, checkout.PaymentCommandID); err != nil {
		t.Fatal(err)
	}

	refund, err := settlement.RequestRefundV2(ctx, orderport.RefundCommandV2{OrderID: checkout.OrderID, AmountMinor: checkout.AmountMinor, Reason: "acceptance full refund", Actor: 1, IdempotencyKey: prefix + "/refund-key"})
	if err != nil {
		t.Fatal(err)
	}
	refundJob := orderapp.EffectJob{RecordID: refund.ID, RiverJobID: 9002, RiverGeneration: 1, RiverQueue: "critical", RiverArgsDigest: sha256.Sum256([]byte(prefix + "/refund-job")), ScheduledAt: now.Add(2 * time.Second)}
	if err = execution.ExecuteRefund(ctx, refundJob); err != nil {
		t.Fatal(err)
	}
	if err = execution.ReconcileRefund(ctx, refund.ID); err != nil {
		t.Fatal(err)
	}

	var orderState, entitlementState string
	var checkoutReceipts, callbackReceipts, orderEvents, productEvents, effects, mergeSideEffects, pendingSideEffects int
	err = pool.QueryRow(ctx, `SELECT
      (SELECT status FROM order_list_projections WHERE id=$1),
	      (SELECT state FROM product_local_entitlements WHERE order_id=$1 AND source='paid_order'),
	      (SELECT count(*) FROM order_operation_receipts WHERE operation IN ('pe01.checkout','pe01.refund') AND state='completed' AND result_snapshot->>'OrderID'=$1::text),
      (SELECT count(*) FROM order_provider_callback_receipts WHERE order_id=$1 AND state='completed'),
      (SELECT count(*) FROM event_log WHERE customer_id=$2 AND event_type LIKE 'order.%'),
	      (SELECT count(*) FROM event_log WHERE customer_id=$2 AND event_type IN ('product.entitlement_granted','product.entitlement_revoked')),
	      (SELECT count(*) FROM external_effects e WHERE e.id=(SELECT external_effect_id FROM order_payment_commands WHERE order_id=$1) OR e.id IN (SELECT external_effect_id FROM order_financial_refunds WHERE order_id=$1)),
      (SELECT count(*) FROM customer_merges),
      (SELECT count(*) FROM pending_events)`, checkout.OrderID, customerID).Scan(&orderState, &entitlementState, &checkoutReceipts, &callbackReceipts, &orderEvents, &productEvents, &effects, &mergeSideEffects, &pendingSideEffects)
	if err != nil {
		t.Fatal(err)
	}
	if orderState != "refunded" || entitlementState != "revoked" || checkoutReceipts != 2 || callbackReceipts != 2 || orderEvents != 4 || productEvents != 2 || effects != 2 || mergeSideEffects != 0 || pendingSideEffects != 0 {
		t.Fatalf("state=%s entitlement=%s receipts=%d/%d events=%d/%d effects=%d side_effects=%d/%d", orderState, entitlementState, checkoutReceipts, callbackReceipts, orderEvents, productEvents, effects, mergeSideEffects, pendingSideEffects)
	}
}

type pe01FakeProvider struct{ now time.Time }

func (provider *pe01FakeProvider) CreatePrepay(context.Context, orderport.PrepayRequest) (orderport.ProviderResult, error) {
	return orderport.ProviderResult{Completion: orderport.ProviderOutcomeUnknown, ReceiptDigest: sha256.Sum256([]byte("fake-prepay-unknown"))}, nil
}
func (provider *pe01FakeProvider) RequestRefund(context.Context, orderport.RefundRequest) (orderport.ProviderResult, error) {
	return orderport.ProviderResult{Completion: orderport.ProviderOutcomeUnknown, ReceiptDigest: sha256.Sum256([]byte("fake-refund-unknown"))}, nil
}
func (provider *pe01FakeProvider) QueryPayment(context.Context, string) (orderport.PaymentQueryResult, error) {
	return orderport.PaymentQueryResult{Confirmed: true, EvidenceDigest: sha256.Sum256([]byte("fake-payment-query")), ProviderTransactionDigest: sha256.Sum256([]byte("fake-transaction")), AmountMinor: 1990, Currency: "CNY", OccurredAt: provider.now}, nil
}
func (provider *pe01FakeProvider) QueryRefund(context.Context, string) (orderport.RefundQueryResult, error) {
	return orderport.RefundQueryResult{Confirmed: true, EvidenceDigest: sha256.Sum256([]byte("fake-refund-query")), ProviderRefundDigest: sha256.Sum256([]byte("fake-refund")), AmountMinor: 1990, Currency: "CNY", OccurredAt: provider.now.Add(time.Second)}, nil
}

type pe01FakeRuntime struct {
	pool *pgxpool.Pool
	seq  atomic.Int64
}

func (runtime *pe01FakeRuntime) Execute(ctx context.Context, command orderport.ExternalEffectCommand, execute orderport.ProviderExecution) (orderport.ExternalEffectResult, error) {
	result, err := execute(ctx)
	if err != nil {
		return orderport.ExternalEffectResult{}, err
	}
	state := orderport.EffectExecuted
	if result.Completion == orderport.ProviderOutcomeUnknown {
		state = orderport.EffectOutcomeUnknown
	}
	fingerprint := sha256.Sum256([]byte(fmt.Sprintf("pe01-fake-effect-%x-%d", command.SourceRefDigest, runtime.seq.Add(1))))
	id, err := eerfixture.CreatePE01Effect(ctx, runtime.pool, string(command.Kind), digestText(command.SourceRefDigest), digestText(command.TargetRefDigest), digestText(command.PayloadDigest), digestText(command.PolicyVersionDigest), digestText(fingerprint), string(state))
	return orderport.ExternalEffectResult{EffectID: fmt.Sprintf("eer_%d", id), State: state, ReceiptDigest: result.ReceiptDigest}, err
}

func (*pe01FakeRuntime) Reconcile(_ context.Context, effectID string, evidence [32]byte) (orderport.ExternalEffectResult, error) {
	return orderport.ExternalEffectResult{EffectID: effectID, State: orderport.EffectReconciled, ReceiptDigest: evidence}, nil
}

func digestText(value [32]byte) string { return "sha256:" + hex.EncodeToString(value[:]) }

func openPE01Pool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *i03DatabaseURL == "" {
		t.Fatal("database-url is required")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*i03DatabaseURL, acceptancefixtures.PE01DatabaseName); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *i03DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil || version/10000 != 16 {
		t.Fatalf("PostgreSQL version=%d error=%v", version, err)
	}
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}
