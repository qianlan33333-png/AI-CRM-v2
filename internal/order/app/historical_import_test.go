package app

import (
	"context"
	"errors"
	"testing"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestHistoricalImportOrderCreatesAndReplays(t *testing.T) {
	service, store, journal, _ := newHistoricalImportTestService()
	input := historicalOrderInput()

	created, err := service.ImportOrder(context.Background(), input)
	if err != nil || created.TargetID != 1 || created.Replayed || len(store.orders) != 1 || journal.appendCalls != 1 {
		t.Fatalf("created/store/journal = %#v/%#v/%#v", created, store.orders, journal)
	}
	if receipt := journal.receipts[historicalReceiptKey(historicalOrderKind, input.Fact.SourceKeyDigest)]; receipt.TargetDigest != HistoricalOrderTargetDigest(input.Order) || !sameHistoricalDigest(receipt.FieldDigest, input.Fact.FieldDigest) {
		t.Fatalf("receipt = %#v", receipt)
	}

	replayed, err := service.ImportOrder(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.TargetID != created.TargetID || len(store.orders) != 1 || journal.appendCalls != 1 {
		t.Fatalf("replayed/store/journal = %#v/%#v/%#v", replayed, store.orders, journal)
	}
}

func TestHistoricalImportOrderRejectsNativeAndDrift(t *testing.T) {
	t.Run("native input", func(t *testing.T) {
		service, store, journal, uow := newHistoricalImportTestService()
		input := historicalOrderInput()
		input.Order.RecordOrigin = orderport.RecordOriginNative
		if _, err := service.ImportOrder(context.Background(), input); !errors.Is(err, orderport.ErrHistoricalInput) || uow.calls != 0 || len(store.orders) != 0 || len(journal.receipts) != 0 {
			t.Fatalf("error/calls/store/receipts = %v/%d/%#v/%#v", err, uow.calls, store.orders, journal.receipts)
		}
	})

	t.Run("fact and target drift", func(t *testing.T) {
		service, store, _, _ := newHistoricalImportTestService()
		input := historicalOrderInput()
		created, err := service.ImportOrder(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		changedFact := input
		changedFact.Fact.PayloadDigest[0]++
		if _, err = service.ImportOrder(context.Background(), changedFact); !errors.Is(err, orderport.ErrHistoricalConflict) || len(store.orders) != 1 {
			t.Fatalf("fact drift error/orders = %v/%#v", err, store.orders)
		}
		stored := store.orders[orderport.ID(created.TargetID)]
		stored.Status = "altered"
		store.orders[stored.ID] = stored
		if _, err = service.ImportOrder(context.Background(), input); !errors.Is(err, orderport.ErrHistoricalConflict) {
			t.Fatalf("target drift error = %v", err)
		}
	})
}

func TestHistoricalImportOrderRollsBackOnReceiptFailure(t *testing.T) {
	service, store, journal, _ := newHistoricalImportTestService()
	journal.appendErr = errors.New("receipt write failed")
	result, err := service.ImportOrder(context.Background(), historicalOrderInput())
	if !errors.Is(err, orderport.ErrHistoricalUnavailable) || result != (HistoricalImportResult{}) || len(store.orders) != 0 || len(journal.receipts) != 0 {
		t.Fatalf("result/error/store/receipts = %#v/%v/%#v/%#v", result, err, store.orders, journal.receipts)
	}
	journal.appendErr = nil
	result, err = service.ImportOrder(context.Background(), historicalOrderInput())
	if err != nil || result.TargetID != 1 || result.Replayed || len(store.orders) != 1 || len(journal.receipts) != 1 {
		t.Fatalf("retry result/error/store/receipts = %#v/%v/%#v/%#v", result, err, store.orders, journal.receipts)
	}
}

func TestHistoricalImportResetsResultForTransactionRetry(t *testing.T) {
	t.Run("order replay then create", func(t *testing.T) {
		service, store, journal, uow := newHistoricalImportTestService()
		input := historicalOrderInput()
		stored := input.Order
		stored.ID = 77
		store.orders[stored.ID], store.nextOrder = stored, 77
		journal.receipts[historicalReceiptKey(historicalOrderKind, input.Fact.SourceKeyDigest)] = orderport.HistoricalImportReceipt{HistoricalFact: input.Fact, TargetID: 77, TargetDigest: HistoricalOrderTargetDigest(input.Order)}
		uow.retryAfterFirst = func() {
			store.orders, store.nextOrder = map[orderport.ID]orderport.Record{}, 0
			journal.receipts = map[string]orderport.HistoricalImportReceipt{}
		}
		result, err := service.ImportOrder(context.Background(), input)
		if err != nil || result != (HistoricalImportResult{TargetID: 1}) {
			t.Fatalf("result/error = %#v/%v", result, err)
		}
	})

	t.Run("refund replay then create", func(t *testing.T) {
		service, store, journal, uow := newHistoricalImportTestService()
		order := historicalOrderInput().Order
		order.ID = 1
		store.orders[order.ID], store.nextOrder = order, 1
		input := historicalRefundInput(1)
		stored := input.Refund
		stored.ID = 77
		store.refunds[stored.ID], store.nextRefund = stored, 77
		journal.receipts[historicalReceiptKey(historicalRefundKind, input.Fact.SourceKeyDigest)] = orderport.HistoricalImportReceipt{HistoricalFact: input.Fact, TargetID: 77, TargetDigest: HistoricalRefundTargetDigest(input.Refund)}
		uow.retryAfterFirst = func() {
			store.refunds, store.nextRefund = map[int64]orderport.HistoricalRefund{}, 0
			journal.receipts = map[string]orderport.HistoricalImportReceipt{}
		}
		result, err := service.ImportRefund(context.Background(), input)
		if err != nil || result != (HistoricalImportResult{TargetID: 1}) {
			t.Fatalf("result/error = %#v/%v", result, err)
		}
	})
}

func TestHistoricalImportOrderDoesNotOverwriteStoreConflict(t *testing.T) {
	service, store, journal, _ := newHistoricalImportTestService()
	store.createOrderErr = orderport.ErrHistoricalConflict
	if _, err := service.ImportOrder(context.Background(), historicalOrderInput()); !errors.Is(err, orderport.ErrHistoricalConflict) || len(store.orders) != 0 || len(journal.receipts) != 0 {
		t.Fatalf("error/store/receipts = %v/%#v/%#v", err, store.orders, journal.receipts)
	}
}

func TestHistoricalImportRefundChecksHistoricalOrderAndReplays(t *testing.T) {
	service, store, journal, _ := newHistoricalImportTestService()
	orderResult, err := service.ImportOrder(context.Background(), historicalOrderInput())
	if err != nil {
		t.Fatal(err)
	}
	input := historicalRefundInput(orderResult.TargetID)
	created, err := service.ImportRefund(context.Background(), input)
	if err != nil || created.TargetID != 1 || created.Replayed || len(store.refunds) != 1 || store.refunds[1].Reason != " 原始退款原因 " {
		t.Fatalf("created/refunds = %#v/%#v", created, store.refunds)
	}
	replayed, err := service.ImportRefund(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.TargetID != created.TargetID || len(store.refunds) != 1 || journal.appendCalls != 2 {
		t.Fatalf("replayed/refunds/journal = %#v/%#v/%#v", replayed, store.refunds, journal)
	}

	mismatch := historicalRefundInput(orderResult.TargetID)
	mismatch.Fact.SourceKeyDigest = historicalDigest(9)
	mismatch.Refund.OrderAmountMinor++
	if _, err = service.ImportRefund(context.Background(), mismatch); !errors.Is(err, orderport.ErrHistoricalConflict) || len(store.refunds) != 1 {
		t.Fatalf("mismatch error/refunds = %v/%#v", err, store.refunds)
	}
}

func TestHistoricalImportRefundRejectsReceiptTargetDrift(t *testing.T) {
	service, store, _, _ := newHistoricalImportTestService()
	orderResult, err := service.ImportOrder(context.Background(), historicalOrderInput())
	if err != nil {
		t.Fatal(err)
	}
	input := historicalRefundInput(orderResult.TargetID)
	created, err := service.ImportRefund(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	stored := store.refunds[created.TargetID]
	stored.Reason = "modified"
	store.refunds[created.TargetID] = stored
	if _, err = service.ImportRefund(context.Background(), input); !errors.Is(err, orderport.ErrHistoricalConflict) {
		t.Fatalf("error = %v", err)
	}
	stored.Reason = input.Refund.Reason
	store.refunds[created.TargetID] = stored
	order := store.orders[input.Refund.OrderID]
	order.AmountMinor++
	store.orders[order.ID] = order
	if _, err = service.ImportRefund(context.Background(), input); !errors.Is(err, orderport.ErrHistoricalConflict) {
		t.Fatalf("related order drift error = %v", err)
	}
}

type historicalImportMemoryStore struct {
	orders          map[orderport.ID]orderport.Record
	refunds         map[int64]orderport.HistoricalRefund
	nextOrder       int64
	nextRefund      int64
	createOrderErr  error
	createRefundErr error
}

func (store *historicalImportMemoryStore) CreateHistoricalOrder(_ context.Context, input orderport.Record) (orderport.Record, error) {
	if store.createOrderErr != nil {
		return orderport.Record{}, store.createOrderErr
	}
	for _, existing := range store.orders {
		if existing.Provider == input.Provider && existing.MerchantOrderNo == input.MerchantOrderNo {
			return orderport.Record{}, orderport.ErrHistoricalConflict
		}
	}
	store.nextOrder++
	input.ID = orderport.ID(store.nextOrder)
	store.orders[input.ID] = input
	return input, nil
}

func (store *historicalImportMemoryStore) GetHistoricalOrder(_ context.Context, id orderport.ID) (orderport.Record, error) {
	value, found := store.orders[id]
	if !found {
		return orderport.Record{}, ErrNotFound
	}
	return value, nil
}

func (store *historicalImportMemoryStore) CreateHistoricalRefund(_ context.Context, input orderport.HistoricalRefund) (orderport.HistoricalRefund, error) {
	if store.createRefundErr != nil {
		return orderport.HistoricalRefund{}, store.createRefundErr
	}
	for _, existing := range store.refunds {
		if existing.SourceRefundID == input.SourceRefundID || existing.RefundNumber == input.RefundNumber {
			return orderport.HistoricalRefund{}, orderport.ErrHistoricalConflict
		}
	}
	store.nextRefund++
	input.ID = store.nextRefund
	store.refunds[input.ID] = input
	return input, nil
}

func (store *historicalImportMemoryStore) GetHistoricalRefund(_ context.Context, id int64) (orderport.HistoricalRefund, error) {
	value, found := store.refunds[id]
	if !found {
		return orderport.HistoricalRefund{}, ErrNotFound
	}
	return value, nil
}

type historicalImportMemoryJournal struct {
	receipts    map[string]orderport.HistoricalImportReceipt
	appendErr   error
	appendCalls int
}

func (journal *historicalImportMemoryJournal) FindHistoricalOrderReceipt(_ context.Context, kind string, source [32]byte) (orderport.HistoricalImportReceipt, bool, error) {
	receipt, found := journal.receipts[historicalReceiptKey(kind, source)]
	return receipt, found, nil
}

func (journal *historicalImportMemoryJournal) AppendHistoricalOrderReceipt(_ context.Context, kind string, receipt orderport.HistoricalImportReceipt) error {
	journal.appendCalls++
	if journal.appendErr != nil {
		return journal.appendErr
	}
	journal.receipts[historicalReceiptKey(kind, receipt.SourceKeyDigest)] = receipt
	return nil
}

type historicalImportMemoryUOW struct {
	store           *historicalImportMemoryStore
	journal         *historicalImportMemoryJournal
	calls           int
	retryAfterFirst func()
}

func (uow *historicalImportMemoryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	orders, refunds, receipts := copyHistoricalOrders(uow.store.orders), copyHistoricalRefunds(uow.store.refunds), copyHistoricalReceipts(uow.journal.receipts)
	nextOrder, nextRefund := uow.store.nextOrder, uow.store.nextRefund
	if err := callback(ctx); err != nil {
		uow.store.orders, uow.store.refunds, uow.journal.receipts = orders, refunds, receipts
		uow.store.nextOrder, uow.store.nextRefund = nextOrder, nextRefund
		return err
	}
	if uow.retryAfterFirst != nil {
		retry := uow.retryAfterFirst
		uow.retryAfterFirst = nil
		retry()
		return callback(ctx)
	}
	return nil
}

func newHistoricalImportTestService() (*HistoricalImportService, *historicalImportMemoryStore, *historicalImportMemoryJournal, *historicalImportMemoryUOW) {
	store := &historicalImportMemoryStore{orders: map[orderport.ID]orderport.Record{}, refunds: map[int64]orderport.HistoricalRefund{}}
	journal := &historicalImportMemoryJournal{receipts: map[string]orderport.HistoricalImportReceipt{}}
	uow := &historicalImportMemoryUOW{store: store, journal: journal}
	service, err := NewHistoricalImportService(uow, store, journal)
	if err != nil {
		panic(err)
	}
	return service, store, journal, uow
}

func historicalOrderInput() orderport.HistoricalOrderRecord {
	stamp := time.Date(2026, 8, 28, 11, 0, 0, 123_456_000, time.FixedZone("CST", 8*60*60))
	customer, product := int64(17), int64(29)
	return orderport.HistoricalOrderRecord{Fact: historicalFact(1), Order: orderport.Record{
		RecordOrigin: orderport.RecordOriginV1History, Provider: "wechat", ProviderLabel: "微信支付（V1历史）",
		MerchantOrderNo: "v1-order-1", PlatformTransactionNo: "v1-transaction-1", CustomerID: &customer,
		ProductID: &product, ProductCode: "v1-product-1", ProductNameSnapshot: "V1 商品", AmountMinor: 9901, Currency: "CNY",
		Status: "paid", StatusLabel: "V1历史/未重新核验", DetailURL: "/api/admin/wechat-pay/orders/v1-order-1", CreatedAt: stamp, UpdatedAt: stamp,
	}}
}

func historicalRefundInput(orderID int64) orderport.HistoricalRefundRecord {
	stamp := time.Date(2026, 8, 28, 11, 1, 0, 123_456_000, time.UTC)
	return orderport.HistoricalRefundRecord{Fact: historicalFact(4), Refund: orderport.HistoricalRefund{
		OrderID: orderport.ID(orderID), SourceRefundID: 99, RefundNumber: "v1-refund-99", ProviderRefundID: "provider-refund-99",
		TransactionID: "v1-transaction-1", Status: "SUCCESS", AmountMinor: 100, OrderAmountMinor: 9901, Currency: "CNY",
		Reason: " 原始退款原因 ", CreatedAt: stamp, UpdatedAt: stamp,
	}}
}

func historicalFact(first byte) orderport.HistoricalFact {
	return orderport.HistoricalFact{SourceKeyDigest: historicalDigest(first), PayloadDigest: historicalDigest(first + 1), FieldDigest: historicalDigest(first + 2)}
}

func historicalDigest(first byte) (value [32]byte)             { value[0] = first; return value }
func historicalReceiptKey(kind string, digest [32]byte) string { return kind + string(digest[:]) }

func copyHistoricalOrders(source map[orderport.ID]orderport.Record) map[orderport.ID]orderport.Record {
	result := make(map[orderport.ID]orderport.Record, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func copyHistoricalRefunds(source map[int64]orderport.HistoricalRefund) map[int64]orderport.HistoricalRefund {
	result := make(map[int64]orderport.HistoricalRefund, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func copyHistoricalReceipts(source map[string]orderport.HistoricalImportReceipt) map[string]orderport.HistoricalImportReceipt {
	result := make(map[string]orderport.HistoricalImportReceipt, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
