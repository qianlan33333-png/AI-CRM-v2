package product

import (
	"context"
	"errors"
	"testing"
	"time"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestHistoricalStaticProductWriterImportsDisabledDefinitionAndReplays(t *testing.T) {
	definition := historicalStaticProductFixture(t)
	store := &historicalStaticProductMemoryStore{}
	journal := &historicalStaticProductMemoryJournal{receipts: map[string]HistoricalStaticProductReceipt{}}
	writer, err := NewHistoricalStaticProductWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Import(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Replayed || receipt.OriginalStatus != "published" || !receipt.OriginalEnabled || receipt.TargetProductID != 41 || receipt.TargetProductCode != "hxc-annual" ||
		receipt.TargetProductName != "HXC 年度服务" || receipt.PriceMinor != 19900 || receipt.Currency != "CNY" || receipt.CreatedBy != 9 {
		t.Fatalf("receipt = %#v", receipt)
	}
	if len(store.definitions) != 1 || store.definitions[0].Product.PriceMinor != 19900 || store.definitions[0].Product.Currency != "CNY" || store.definitions[0].Product.LocalLifecycle != productport.LocalProductDisabled {
		t.Fatalf("stored definitions = %#v", store.definitions)
	}
	if store.runtimeReceipts != 0 || store.events != 0 || store.providers != 0 {
		t.Fatalf("runtime side effects = %#v", store)
	}
	replayed, err := writer.Import(context.Background(), definition)
	if err != nil || !replayed.Replayed || len(store.definitions) != 1 || journal.records != 1 {
		t.Fatalf("replay/error/definitions/records = %#v/%v/%d/%d", replayed, err, len(store.definitions), journal.records)
	}
}

func TestHistoricalStaticProductWriterRejectsDigestDriftAndExistingProduct(t *testing.T) {
	t.Run("digest drift", func(t *testing.T) {
		definition := historicalStaticProductFixture(t)
		journal := &historicalStaticProductMemoryJournal{receipts: map[string]HistoricalStaticProductReceipt{
			definition.SourceIdentifier: historicalStaticProductReceipt(definition),
		}}
		store := &historicalStaticProductMemoryStore{}
		writer, _ := NewHistoricalStaticProductWriter(store, journal)
		definition.PayloadDigest[0]++
		if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalStaticProductConflict) || len(store.definitions) != 0 || journal.records != 0 {
			t.Fatalf("error/definitions/records = %v/%d/%d", err, len(store.definitions), journal.records)
		}
	})

	t.Run("existing user product", func(t *testing.T) {
		definition := historicalStaticProductFixture(t)
		store := &historicalStaticProductMemoryStore{insertErr: ErrHistoricalStaticProductConflict}
		journal := &historicalStaticProductMemoryJournal{receipts: map[string]HistoricalStaticProductReceipt{}}
		writer, _ := NewHistoricalStaticProductWriter(store, journal)
		if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalStaticProductConflict) || journal.records != 0 {
			t.Fatalf("error/records = %v/%d", err, journal.records)
		}
	})
}

func TestAdaptV1WeChatPayProductStaticPreservesIntegerAmountAndRejectsUnknownConversions(t *testing.T) {
	row := historicalStaticProductSourceFixture()
	definition, err := AdaptV1WeChatPayProductStatic("wechat_pay_products/opaque-key", [32]byte{7}, row, 9)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Product.PriceMinor != row.AmountTotal || definition.Product.Currency != row.Currency || definition.Product.CreatedAt != row.CreatedAt.UTC() || definition.Product.UpdatedAt != row.UpdatedAt.UTC() {
		t.Fatalf("definition changed source value = %#v", definition.Product)
	}
	if definition.Product.LocalLifecycle != productport.LocalProductDisabled || definition.Product.StockQuantity != 0 || definition.Product.CreatedBy != 9 || definition.OriginalStatus != row.Status || definition.OriginalEnabled != row.Enabled {
		t.Fatalf("definition = %#v", definition)
	}
	for _, mutate := range []func(*V1WeChatPayProductStaticRow){
		func(value *V1WeChatPayProductStaticRow) { value.Currency = "cny" },
		func(value *V1WeChatPayProductStaticRow) { value.AmountTotal = -1 },
		func(value *V1WeChatPayProductStaticRow) { value.ProductCode = " code" },
		func(value *V1WeChatPayProductStaticRow) { value.UpdatedAt = value.CreatedAt.Add(-time.Nanosecond) },
	} {
		invalid := row
		mutate(&invalid)
		if _, err := AdaptV1WeChatPayProductStatic("wechat_pay_products/opaque-key", [32]byte{7}, invalid, 9); !errors.Is(err, ErrHistoricalStaticProductInvalid) {
			t.Fatalf("row = %#v, error = %v", invalid, err)
		}
	}
}

func TestHistoricalStaticProductWriterRejectsUnexpectedStoredState(t *testing.T) {
	definition := historicalStaticProductFixture(t)
	store := &historicalStaticProductMemoryStore{storedLifecycle: productport.LocalProductEnabled}
	journal := &historicalStaticProductMemoryJournal{receipts: map[string]HistoricalStaticProductReceipt{}}
	writer, _ := NewHistoricalStaticProductWriter(store, journal)
	if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalStaticProductConflict) || journal.records != 0 {
		t.Fatalf("error/records = %v/%d", err, journal.records)
	}
}

type historicalStaticProductMemoryStore struct {
	definitions                        []HistoricalStaticProductDefinition
	insertErr                          error
	storedLifecycle                    productport.LocalProductLifecycle
	runtimeReceipts, events, providers int
}

func (store *historicalStaticProductMemoryStore) InsertHistoricalStaticProduct(_ context.Context, definition HistoricalStaticProductDefinition) (productport.Product, error) {
	if store.insertErr != nil {
		return productport.Product{}, store.insertErr
	}
	store.definitions = append(store.definitions, definition)
	stored := definition.Product
	stored.ID = 41
	if store.storedLifecycle != "" {
		stored.LocalLifecycle = store.storedLifecycle
	}
	return stored, nil
}

func (store *historicalStaticProductMemoryStore) ProjectHistoricalEditableProduct(_ context.Context, _ HistoricalEditableProductProjection) (bool, error) {
	return false, nil
}

type historicalStaticProductMemoryJournal struct {
	receipts map[string]HistoricalStaticProductReceipt
	records  int
}

func (journal *historicalStaticProductMemoryJournal) LoadHistoricalStaticProduct(_ context.Context, sourceIdentifier string) (HistoricalStaticProductReceipt, bool, error) {
	receipt, found := journal.receipts[sourceIdentifier]
	return receipt, found, nil
}

func (journal *historicalStaticProductMemoryJournal) RecordHistoricalStaticProduct(_ context.Context, receipt HistoricalStaticProductReceipt) error {
	journal.receipts[receipt.SourceIdentifier] = receipt
	journal.records++
	return nil
}

func historicalStaticProductFixture(t *testing.T) HistoricalStaticProductDefinition {
	t.Helper()
	definition, err := AdaptV1WeChatPayProductStatic("wechat_pay_products/opaque-key", [32]byte{7}, historicalStaticProductSourceFixture(), 9)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func historicalStaticProductReceipt(definition HistoricalStaticProductDefinition) HistoricalStaticProductReceipt {
	return HistoricalStaticProductReceipt{
		SourceIdentifier:  definition.SourceIdentifier,
		SourceID:          definition.SourceID,
		PayloadDigest:     definition.PayloadDigest,
		OriginalStatus:    definition.OriginalStatus,
		OriginalEnabled:   definition.OriginalEnabled,
		TargetProductID:   41,
		TargetProductCode: definition.Product.ProductCode,
		TargetProductName: definition.Product.Name,
		PriceMinor:        definition.Product.PriceMinor,
		Currency:          definition.Product.Currency,
		CreatedBy:         definition.Product.CreatedBy,
	}
}

func historicalStaticProductSourceFixture() V1WeChatPayProductStaticRow {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	return V1WeChatPayProductStaticRow{
		ID: 29, ProductCode: "hxc-annual", Name: "HXC 年度服务", AmountTotal: 19900, Currency: "CNY",
		Status: "published", Enabled: true, CreatedAt: stamp, UpdatedAt: stamp.Add(time.Hour),
	}
}
