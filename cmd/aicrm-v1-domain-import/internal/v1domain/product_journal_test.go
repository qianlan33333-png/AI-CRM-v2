package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
)

func productScopeFixture() Scope {
	return Scope{ImportVersion: "v1-product-static", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: productTableID, TargetDomain: "product", TargetTable: "products"}
}

func productReceiptFixture() product.HistoricalStaticProductReceipt {
	return product.HistoricalStaticProductReceipt{SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte("source-key"))),
		SourceID: 9007199254740993, PayloadDigest: sha256.Sum256([]byte("authenticated-source-payload")),
		OriginalStatus: "active", OriginalEnabled: false, TargetProductID: 703, TargetProductCode: "legacy-code",
		TargetProductName: "Legacy product", PriceMinor: 990, Currency: "CNY", CreatedBy: 17}
}

func TestProductJournalMetadataRoundTripDoesNotLoseSourceID(t *testing.T) {
	want := productReceiptFixture()
	terminal, err := productTerminalFromReceipt(want)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.TargetID != "703" || terminal.Metadata["source_id"] != "9007199254740993" || terminal.Metadata["price_minor"] != "990" ||
		terminal.Metadata["created_by"] != "17" || terminal.Disposition != "import" || terminal.Reason != "" {
		t.Fatalf("terminal=%+v", terminal)
	}
	// JSONB may reorder keys and add whitespace; the Journal supplies a decoded
	// object. Storing source IDs as strings also survives standard json.Unmarshal.
	encoded, err := json.MarshalIndent(terminal.Metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	terminal.Metadata = nil
	if err = json.Unmarshal(encoded, &terminal.Metadata); err != nil {
		t.Fatal(err)
	}
	got, err := productReceiptFromTerminal(want.SourceIdentifier, terminal)
	if err != nil || got != want {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if terminal.TargetDigest != productTargetDigest("703", want.PayloadDigest) {
		t.Fatal("wrong target digest")
	}
}

func TestProductJournalRejectsInvalidOrNonimportReceipts(t *testing.T) {
	for name, mutate := range map[string]func(*TerminalReceipt){
		"quarantine":      func(r *TerminalReceipt) { r.Disposition = "quarantine" },
		"reason":          func(r *TerminalReceipt) { r.Reason = "not-empty" },
		"source-key":      func(r *TerminalReceipt) { r.SourceKeyDigest[0]++ },
		"payload":         func(r *TerminalReceipt) { r.PayloadDigest[0]++ },
		"target":          func(r *TerminalReceipt) { r.TargetID = "704" },
		"target-digest":   func(r *TerminalReceipt) { r.TargetDigest[0]++ },
		"source-number":   func(r *TerminalReceipt) { r.Metadata["source_id"] = float64(11) },
		"missing-status":  func(r *TerminalReceipt) { delete(r.Metadata, "original_status") },
		"missing-enabled": func(r *TerminalReceipt) { delete(r.Metadata, "original_enabled") },
		"enabled-string":  func(r *TerminalReceipt) { r.Metadata["original_enabled"] = "false" },
		"empty-code":      func(r *TerminalReceipt) { r.Metadata["target_product_code"] = "" },
		"empty-name":      func(r *TerminalReceipt) { r.Metadata["target_product_name"] = "" },
		"price-float":     func(r *TerminalReceipt) { r.Metadata["price_minor"] = float64(990) },
		"price-negative":  func(r *TerminalReceipt) { r.Metadata["price_minor"] = "-1" },
		"price-leading-0": func(r *TerminalReceipt) { r.Metadata["price_minor"] = "0990" },
		"currency":        func(r *TerminalReceipt) { r.Metadata["currency"] = "USD" },
		"actor-float":     func(r *TerminalReceipt) { r.Metadata["created_by"] = float64(17) },
		"actor-zero":      func(r *TerminalReceipt) { r.Metadata["created_by"] = "0" },
		"actor-leading-0": func(r *TerminalReceipt) { r.Metadata["created_by"] = "017" },
		"unknown-field":   func(r *TerminalReceipt) { r.Metadata["provider_verified"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := productReceiptFixture()
			terminal, err := productTerminalFromReceipt(receipt)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&terminal)
			if _, err := productReceiptFromTerminal(receipt.SourceIdentifier, terminal); !errors.Is(err, ErrConflict) {
				t.Fatalf("got %v", err)
			}
		})
	}
	for _, target := range []string{"0", "-1", "+703", "0703", "18446744073709551615"} {
		receipt := productReceiptFixture()
		terminal, _ := productTerminalFromReceipt(receipt)
		terminal.TargetID = target
		terminal.TargetDigest = productTargetDigest(target, terminal.PayloadDigest)
		if _, err := productReceiptFromTerminal(receipt.SourceIdentifier, terminal); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted target %q", target)
		}
	}
}

func TestProductJournalRejectsUnsafeRecordsBeforePersistence(t *testing.T) {
	for name, mutate := range map[string]func(*product.HistoricalStaticProductReceipt){
		"replayed":       func(r *product.HistoricalStaticProductReceipt) { r.Replayed = true },
		"source-id":      func(r *product.HistoricalStaticProductReceipt) { r.SourceID = 0 },
		"target-id":      func(r *product.HistoricalStaticProductReceipt) { r.TargetProductID = 0 },
		"source-key":     func(r *product.HistoricalStaticProductReceipt) { r.SourceIdentifier = "not-a-digest" },
		"empty-payload":  func(r *product.HistoricalStaticProductReceipt) { r.PayloadDigest = [32]byte{} },
		"empty-code":     func(r *product.HistoricalStaticProductReceipt) { r.TargetProductCode = "" },
		"nul-code":       func(r *product.HistoricalStaticProductReceipt) { r.TargetProductCode = "a\x00b" },
		"empty-name":     func(r *product.HistoricalStaticProductReceipt) { r.TargetProductName = "" },
		"negative-price": func(r *product.HistoricalStaticProductReceipt) { r.PriceMinor = -1 },
		"currency":       func(r *product.HistoricalStaticProductReceipt) { r.Currency = "USD" },
		"zero-actor":     func(r *product.HistoricalStaticProductReceipt) { r.CreatedBy = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := productReceiptFixture()
			mutate(&receipt)
			journal, err := NewJournal(productScopeFixture())
			if err != nil {
				t.Fatal(err)
			}
			if err = journal.RecordHistoricalStaticProduct(context.Background(), receipt); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestProductJournalPinsSourceAndTargetScope(t *testing.T) {
	want := productScopeFixture()
	journal, err := NewJournal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.ValidateProductImportScope(want.ArchiveRunID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(journal.scope, want) {
		t.Fatal("validation changed scope")
	}
	for name, mutate := range map[string]func(*Scope){
		"run":           func(s *Scope) { s.ArchiveRunID = "different-run" },
		"adapter":       func(s *Scope) { s.AdapterID = "different-adapter" },
		"source-table":  func(s *Scope) { s.TableID = "public/wechat_pay_orders" },
		"target-domain": func(s *Scope) { s.TargetDomain = "order" },
		"target-table":  func(s *Scope) { s.TargetTable = "orders" },
	} {
		t.Run(name, func(t *testing.T) {
			scope := want
			mutate(&scope)
			journal, err := NewJournal(scope)
			if err != nil {
				t.Fatal(err)
			}
			if err = journal.ValidateProductImportScope(want.ArchiveRunID); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("got %v", err)
			}
		})
	}
	var missing *Journal
	if err = missing.ValidateProductImportScope(want.ArchiveRunID); !errors.Is(err, ErrInvalidScope) {
		t.Fatal(err)
	}
	if _, _, err = missing.LoadHistoricalStaticProduct(context.Background(), "source"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal(err)
	}
}

func TestProductJournalUsesLockedTerminalForLoadAndSealedReplay(t *testing.T) {
	receipt := productReceiptFixture()
	terminal, err := productTerminalFromReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, exists := range []bool{false, true} {
		tx := &journalTestTx{rows: []journalTestRow{func(values ...any) error {
			if !exists {
				return pgx.ErrNoRows
			}
			metadata, err := json.MarshalIndent(terminal.Metadata, "", "  ")
			if err != nil {
				return err
			}
			domain, table, target := "product", "products", terminal.TargetID
			*values[0].(*[]byte) = terminal.PayloadDigest[:]
			*values[1].(*string) = "import"
			*values[2].(*string) = ""
			*values[3].(**string) = &domain
			*values[4].(**string) = &table
			*values[5].(**string) = &target
			*values[6].(*[]byte) = terminal.TargetDigest[:]
			*values[7].(*[]byte) = metadata
			*values[8].(*bool) = true
			return nil
		}}}
		journal := &Journal{scope: productScopeFixture(), tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
		got, found, err := journal.LoadHistoricalStaticProduct(context.Background(), receipt.SourceIdentifier)
		if err != nil || found != exists || exists && got != receipt || len(tx.execs) != 1 || !strings.Contains(tx.execs[0], "pg_advisory_xact_lock") {
			t.Fatalf("load exists=%v got=%+v found=%v err=%v", exists, got, found, err)
		}
	}
	metadata, _ := json.Marshal(terminal.Metadata)
	tx := &journalTestTx{rows: []journalTestRow{
		func(values ...any) error {
			domain, table, target := "product", "products", terminal.TargetID
			*values[0].(*[]byte) = terminal.PayloadDigest[:]
			*values[1].(*string) = "import"
			*values[3].(**string) = &domain
			*values[4].(**string) = &table
			*values[5].(**string) = &target
			*values[6].(*[]byte) = terminal.TargetDigest[:]
			*values[7].(*[]byte) = metadata
			*values[8].(*bool) = true
			return nil
		},
		func(values ...any) error {
			target := terminal.TargetID
			*values[0].(*[]byte) = terminal.PayloadDigest[:]
			*values[1].(*string) = "import"
			*values[3].(**string) = &target
			*values[4].(*[]byte) = terminal.TargetDigest[:]
			*values[5].(*bool) = true
			return nil
		},
	}}
	journal := &Journal{scope: productScopeFixture(), tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	if err := journal.RecordHistoricalStaticProduct(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if len(tx.execs) != 1 || !strings.Contains(tx.execs[0], "pg_advisory_xact_lock") {
		t.Fatal("sealed replay attempted another write")
	}
}
