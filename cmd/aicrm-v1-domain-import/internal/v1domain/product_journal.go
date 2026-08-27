package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var _ product.HistoricalStaticProductJournal = (*Journal)(nil)
var _ ProductImportJournal = (*Journal)(nil)

func (journal *Journal) ValidateProductImportScope(archiveRunID string) error {
	if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ArchiveRunID != archiveRunID ||
		journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != productTableID || journal.scope.TargetDomain != "product" || journal.scope.TargetTable != "products" {
		return ErrInvalidScope
	}
	return nil
}

func (journal *Journal) LoadHistoricalStaticProduct(ctx context.Context, sourceIdentifier string) (product.HistoricalStaticProductReceipt, bool, error) {
	if journal == nil || ctx == nil || journal.ValidateProductImportScope(journal.scope.ArchiveRunID) != nil {
		return product.HistoricalStaticProductReceipt{}, false, ErrInvalidScope
	}
	terminal, found, err := journal.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return product.HistoricalStaticProductReceipt{}, false, err
	}
	receipt, err := productReceiptFromTerminal(sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *Journal) RecordHistoricalStaticProduct(ctx context.Context, receipt product.HistoricalStaticProductReceipt) error {
	if journal == nil || ctx == nil || journal.ValidateProductImportScope(journal.scope.ArchiveRunID) != nil {
		return ErrInvalidScope
	}
	terminal, err := productTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.Record(ctx, terminal)
}

func productTerminalFromReceipt(receipt product.HistoricalStaticProductReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == [sha256.Size]byte{} || receipt.PayloadDigest == [sha256.Size]byte{} || receipt.Replayed || receipt.SourceID < 1 ||
		receipt.TargetProductID < 1 || !productReceiptText(receipt.OriginalStatus, 128) || !productReceiptText(receipt.TargetProductCode, 200) ||
		!productReceiptText(receipt.TargetProductName, 200) || receipt.PriceMinor < 0 || receipt.Currency != "CNY" || receipt.CreatedBy < 1 {
		return TerminalReceipt{}, ErrInvalidScope
	}
	targetID := strconv.FormatInt(int64(receipt.TargetProductID), 10)
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: targetID,
		TargetDigest: productTargetDigest(targetID, receipt.PayloadDigest), Metadata: map[string]any{
			"source_id": strconv.FormatInt(receipt.SourceID, 10), "original_status": receipt.OriginalStatus,
			"original_enabled": receipt.OriginalEnabled, "target_product_code": receipt.TargetProductCode,
			"target_product_name": receipt.TargetProductName, "price_minor": strconv.FormatInt(receipt.PriceMinor, 10),
			"currency": receipt.Currency, "created_by": strconv.FormatInt(receipt.CreatedBy, 10),
		}}, nil
}

func productReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (product.HistoricalStaticProductReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || sourceKey == [sha256.Size]byte{} || terminal.SourceKeyDigest != sourceKey || terminal.PayloadDigest == [sha256.Size]byte{} ||
		terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetDigest != productTargetDigest(terminal.TargetID, terminal.PayloadDigest) || len(terminal.Metadata) != 8 {
		return product.HistoricalStaticProductReceipt{}, ErrConflict
	}
	sourceText, sourceOK := terminal.Metadata["source_id"].(string)
	status, statusOK := terminal.Metadata["original_status"].(string)
	enabled, enabledOK := terminal.Metadata["original_enabled"].(bool)
	code, codeOK := terminal.Metadata["target_product_code"].(string)
	name, nameOK := terminal.Metadata["target_product_name"].(string)
	priceText, priceOK := terminal.Metadata["price_minor"].(string)
	currency, currencyOK := terminal.Metadata["currency"].(string)
	actorText, actorOK := terminal.Metadata["created_by"].(string)
	sourceID, sourceErr := strconv.ParseInt(sourceText, 10, 64)
	targetID, targetErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	priceMinor, priceErr := strconv.ParseInt(priceText, 10, 64)
	createdBy, actorErr := strconv.ParseInt(actorText, 10, 64)
	if !sourceOK || !statusOK || !enabledOK || !codeOK || !nameOK || !priceOK || !currencyOK || !actorOK || sourceErr != nil || targetErr != nil || priceErr != nil || actorErr != nil ||
		sourceID < 1 || targetID < 1 || priceMinor < 0 || createdBy < 1 || strconv.FormatInt(sourceID, 10) != sourceText || strconv.FormatInt(targetID, 10) != terminal.TargetID ||
		strconv.FormatInt(priceMinor, 10) != priceText || strconv.FormatInt(createdBy, 10) != actorText || !productReceiptText(status, 128) || !productReceiptText(code, 200) ||
		!productReceiptText(name, 200) || currency != "CNY" {
		return product.HistoricalStaticProductReceipt{}, ErrConflict
	}
	return product.HistoricalStaticProductReceipt{SourceIdentifier: sourceIdentifier, SourceID: sourceID, PayloadDigest: terminal.PayloadDigest,
		OriginalStatus: status, OriginalEnabled: enabled, TargetProductID: productport.ID(targetID), TargetProductCode: code,
		TargetProductName: name, PriceMinor: priceMinor, Currency: currency, CreatedBy: createdBy}, nil
}

func productTargetDigest(targetID string, payload [sha256.Size]byte) [sha256.Size]byte {
	return sha256.Sum256([]byte("product\x00products\x00" + targetID + "\x00" + hex.EncodeToString(payload[:])))
}

func productReceiptText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00')
}
