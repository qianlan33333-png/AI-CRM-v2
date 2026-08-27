package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type productArchiveFake struct {
	rows  []v1archive.ArchivedRow
	err   error
	calls int
}

func (archive *productArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	archive.calls++
	if run != "archive-run" || table != productTableID {
		return ErrInvalidScope
	}
	for _, row := range archive.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return archive.err
}

type productTxKey struct{}

type productImportFake struct {
	products                                     map[string]productport.Product
	terminals                                    map[string]TerminalReceipt
	insertCalls, recordCalls, commits, rollbacks int
	insertErr, recordErr                         error
}

func (fake *productImportFake) Within(ctx context.Context, callback func(context.Context) error) error {
	products, terminals := map[string]productport.Product{}, map[string]TerminalReceipt{}
	for key, value := range fake.products {
		products[key] = value
	}
	for key, value := range fake.terminals {
		terminals[key] = value
	}
	err := callback(context.WithValue(ctx, productTxKey{}, true))
	if err != nil {
		fake.products, fake.terminals = products, terminals
		fake.rollbacks++
		return err
	}
	fake.commits++
	return nil
}

func (fake *productImportFake) ValidateProductImportScope(run string) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (fake *productImportFake) InsertHistoricalStaticProduct(ctx context.Context, definition product.HistoricalStaticProductDefinition) (productport.Product, error) {
	if ctx.Value(productTxKey{}) != true {
		return productport.Product{}, errors.New("missing transaction")
	}
	fake.insertCalls++
	if fake.insertErr != nil {
		return productport.Product{}, fake.insertErr
	}
	if _, found := fake.products[definition.Product.ProductCode]; found {
		return productport.Product{}, product.ErrHistoricalStaticProductConflict
	}
	item := definition.Product
	item.ID = productport.ID(700 + len(fake.products))
	fake.products[item.ProductCode] = item
	return item, nil
}

func (fake *productImportFake) LoadTerminal(ctx context.Context, key string) (TerminalReceipt, bool, error) {
	if ctx.Value(productTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	receipt, found := fake.terminals[key]
	return receipt, found, nil
}

func (fake *productImportFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(productTxKey{}) != true {
		return errors.New("missing transaction")
	}
	fake.recordCalls++
	if fake.recordErr != nil {
		return fake.recordErr
	}
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := fake.terminals[key]; found && !reflect.DeepEqual(old, receipt) {
		return ErrConflict
	}
	fake.terminals[key] = receipt
	return nil
}

func (fake *productImportFake) LoadHistoricalStaticProduct(ctx context.Context, source string) (product.HistoricalStaticProductReceipt, bool, error) {
	terminal, found, err := fake.LoadTerminal(ctx, source)
	if err != nil || !found {
		return product.HistoricalStaticProductReceipt{}, false, err
	}
	receipt, err := productReceiptFromTerminal(source, terminal)
	return receipt, err == nil, err
}

func (fake *productImportFake) RecordHistoricalStaticProduct(ctx context.Context, receipt product.HistoricalStaticProductReceipt) error {
	terminal, err := productTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return fake.Record(ctx, terminal)
}

func productArchiveRow(t *testing.T, id int64, mutate func(map[string]any)) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	value := map[string]any{"id": id, "product_code": fmt.Sprintf("legacy-%d", id), "name": "Historical product", "amount_total": 990,
		"currency": "CNY", "status": "active", "enabled": true, "created_at": stamp, "updated_at": stamp,
		"lead_program_id": 999, "completion_redirect_enabled": true, "completion_redirect_url": "https://never-called.invalid/",
		"wecom_tagging_json": map[string]any{"enabled": true}, "metadata_json": map[string]any{"entitlement": "never activated"}}
	if mutate != nil {
		mutate(value)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: productTableID, SourceOrdinal: id,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("product/%d", id))), PayloadHMAC: sha256.Sum256(payload), Payload: payload}
}

func productImporterFixture(t *testing.T, rows ...v1archive.ArchivedRow) (*ProductImporter, *productImportFake, *productArchiveFake) {
	t.Helper()
	fake := &productImportFake{products: map[string]productport.Product{}, terminals: map[string]TerminalReceipt{}}
	archive := &productArchiveFake{rows: rows}
	writer, err := product.NewHistoricalStaticProductWriter(fake, fake)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewProductImporter(archive, fake, writer, fake, 7)
	if err != nil {
		t.Fatal(err)
	}
	return importer, fake, archive
}

func TestProductImporterUsesMinorUnitsAndOneTerminalPerRow(t *testing.T) {
	rows := []v1archive.ArchivedRow{
		productArchiveRow(t, 11, nil),
		productArchiveRow(t, 12, func(row map[string]any) { row["amount_total"] = 0; row["enabled"] = false }),
		productArchiveRow(t, 13, func(row map[string]any) { row["currency"] = "USD" }),
		productArchiveRow(t, 14, func(row map[string]any) { row["amount_total"] = 9.9 }),
	}
	importer, fake, _ := productImporterFixture(t, rows...)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (StaticImportResult{Imported: 2, Quarantined: 2}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(fake.products) != 2 || len(fake.terminals) != 4 || fake.commits != 4 {
		t.Fatalf("wrong terminal counts: %+v", fake)
	}
	for _, item := range fake.products {
		if item.ID < 700 || item.StockQuantity != 0 || item.LocalLifecycle != productport.LocalProductDisabled || item.Currency != "CNY" || item.CreatedBy != 7 || item.Version != 1 || item.Description != "" || len(item.Images) != 0 {
			t.Fatalf("unsafe static product: %+v", item)
		}
		var projection map[string]any
		if err := json.Unmarshal(item.LegacyAdminProjection, &projection); err != nil {
			t.Fatal(err)
		}
		if projection["enabled"] != false || projection["completion_redirect_enabled"] != false || projection["completion_redirect_url"] != "" || projection["lead_program_id"] != nil {
			t.Fatal("runtime behavior retained")
		}
	}
	if fake.products["legacy-11"].PriceMinor != 990 || fake.products["legacy-12"].PriceMinor != 0 {
		t.Fatal("source cents were scaled/defaulted")
	}
	for _, row := range rows {
		terminal := fake.terminals[SourceIdentifier(row.SourceKeyHMAC)]
		if terminal.PayloadDigest != row.PayloadHMAC || terminal.SourceKeyDigest != row.SourceKeyHMAC || sha256.Sum256(row.Payload) != row.PayloadHMAC {
			t.Fatal("source provenance changed")
		}
		if terminal.Disposition == "import" && (terminal.Metadata["target_product_name"] != "Historical product" || terminal.Metadata["currency"] != "CNY" ||
			terminal.Metadata["created_by"] != "7") {
			t.Fatalf("incomplete static product receipt: %+v", terminal.Metadata)
		}
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (StaticImportResult{Imported: 2, Quarantined: 2, Replayed: 4}) || fake.insertCalls != 2 || fake.recordCalls != 4 {
		t.Fatalf("replay result=%+v err=%v", result, err)
	}
}

func TestProductImporterQuarantinesMalformedStaticFacts(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"missing-amount":     func(row map[string]any) { delete(row, "amount_total") },
		"null-amount":        func(row map[string]any) { row["amount_total"] = nil },
		"negative":           func(row map[string]any) { row["amount_total"] = -1 },
		"not-source-integer": func(row map[string]any) { row["amount_total"] = int64(2147483648) },
		"string-amount":      func(row map[string]any) { row["amount_total"] = "990" },
		"missing-enabled":    func(row map[string]any) { delete(row, "enabled") },
		"missing-currency":   func(row map[string]any) { delete(row, "currency") },
		"missing-time":       func(row map[string]any) { delete(row, "updated_at") },
		"invalid-time":       func(row map[string]any) { row["updated_at"] = "not a time" },
		"nul-name":           func(row map[string]any) { row["name"] = "a\x00b" },
	} {
		t.Run(name, func(t *testing.T) {
			importer, fake, _ := productImporterFixture(t, productArchiveRow(t, 11, mutate))
			result, err := importer.Import(context.Background(), "archive-run")
			if err != nil || result.Quarantined != 1 || result.Imported != 0 || fake.insertCalls != 0 || len(fake.terminals) != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
	for _, payload := range []string{"{", "null", "[]"} {
		row := productArchiveRow(t, 11, nil)
		row.Payload = []byte(payload)
		row.PayloadHMAC = sha256.Sum256(row.Payload)
		importer, fake, _ := productImporterFixture(t, row)
		if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result.Quarantined != 1 || fake.insertCalls != 0 {
			t.Fatalf("payload shape %q result=%+v err=%v", payload, result, err)
		}
	}
	row := productArchiveRow(t, 11, nil)
	row.RedactedFields = []string{"name"}
	importer, fake, _ := productImporterFixture(t, row)
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	if fake.terminals[SourceIdentifier(row.SourceKeyHMAC)].Reason != "redacted_product_definition" {
		t.Fatal("redacted static fact was imported")
	}
}

func TestProductImporterStopsOnProvenanceDriftAndRollsBackFailure(t *testing.T) {
	for _, stage := range []string{"insert", "receipt"} {
		t.Run(stage, func(t *testing.T) {
			importer, fake, _ := productImporterFixture(t, productArchiveRow(t, 11, nil))
			failure := errors.New("injected storage failure")
			if stage == "insert" {
				fake.insertErr = failure
			} else {
				fake.recordErr = failure
			}
			result, err := importer.Import(context.Background(), "archive-run")
			if !errors.Is(err, failure) || result != (StaticImportResult{}) || len(fake.products) != 0 || len(fake.terminals) != 0 || fake.rollbacks != 1 {
				t.Fatalf("result=%+v err=%v fake=%+v", result, err, fake)
			}
		})
	}
	for name, mutate := range map[string]func(*v1archive.ArchivedRow){
		"table":          func(row *v1archive.ArchivedRow) { row.TableID = "public/wechat_pay_orders" },
		"adapter":        func(row *v1archive.ArchivedRow) { row.AdapterID = "other" },
		"key":            func(row *v1archive.ArchivedRow) { row.SourceKeyHMAC = [32]byte{} },
		"payload-digest": func(row *v1archive.ArchivedRow) { row.PayloadHMAC = [32]byte{} },
	} {
		t.Run(name, func(t *testing.T) {
			row := productArchiveRow(t, 11, nil)
			mutate(&row)
			importer, fake, _ := productImporterFixture(t, row)
			if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || fake.insertCalls != 0 || fake.recordCalls != 0 {
				t.Fatalf("got %v", err)
			}
		})
	}
	for _, quarantined := range []bool{false, true} {
		row := productArchiveRow(t, 11, nil)
		if quarantined {
			row.Payload = []byte("null")
		}
		importer, fake, archive := productImporterFixture(t, row)
		if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
			t.Fatal(err)
		}
		archive.rows[0].PayloadHMAC[0]++
		if _, err := importer.Import(context.Background(), "archive-run"); err == nil {
			t.Fatal("changed source digest replayed")
		}
		if len(fake.terminals) != 1 {
			t.Fatal("drift created another terminal")
		}
	}
}

func TestProductImporterRejectsWrongRunBeforeReadingArchive(t *testing.T) {
	importer, _, archive := productImporterFixture(t)
	if _, err := importer.Import(context.Background(), "other-run"); !errors.Is(err, ErrInvalidScope) || archive.calls != 0 {
		t.Fatal("wrong run read archive")
	}
	if _, err := importer.Import(nil, "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal(err)
	}
	archive.err = errors.New("archive authentication failed")
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, archive.err) {
		t.Fatal(err)
	}
}
