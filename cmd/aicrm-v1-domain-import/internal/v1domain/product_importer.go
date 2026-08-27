package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
)

const productTableID = "public/wechat_pay_products"

type ProductImportJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
	ValidateProductImportScope(string) error
}

type ProductImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  *product.HistoricalStaticProductWriter
	journal ProductImportJournal
	actorID int64
}

// The writer and terminal journal must share the same scoped Journal and
// caller-owned UnitOfWork. This importer never invokes the live Product app.
func NewProductImporter(archive ArchiveSource, uow UnitOfWork, writer *product.HistoricalStaticProductWriter, journal ProductImportJournal, actorID int64) (*ProductImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	return &ProductImporter{archive: archive, uow: uow, writer: writer, journal: journal, actorID: actorID}, nil
}

// Pointers distinguish missing/null static facts from valid zero cents or
// disabled=false. Runtime/Provider fields remain only in the immutable archive.
type productJSON struct {
	ID          *int64     `json:"id"`
	ProductCode *string    `json:"product_code"`
	Name        *string    `json:"name"`
	AmountTotal *int64     `json:"amount_total"`
	Currency    *string    `json:"currency"`
	Status      *string    `json:"status"`
	Enabled     *bool      `json:"enabled"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func (importer *ProductImporter) Import(ctx context.Context, archiveRunID string) (StaticImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil || importer.actorID < 1 {
		return StaticImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateProductImportScope(archiveRunID); err != nil {
		return StaticImportResult{}, err
	}
	result := StaticImportResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, productTableID, func(row v1archive.ArchivedRow) error {
		if row.TableID != productTableID || row.AdapterID != v1archive.DefaultAdapterID || row.SourceOrdinal < 1 ||
			row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} {
			return ErrConflict
		}
		definition, reason := adaptArchivedProduct(row, importer.actorID)
		replayed := false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			// UnitOfWork may retry its callback; count only the committed outcome.
			replayed = false
			if reason != "" {
				existing, found, err := importer.journal.LoadTerminal(tx, SourceIdentifier(row.SourceKeyHMAC))
				if err != nil {
					return err
				}
				if found {
					if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" ||
						existing.Reason != reason || existing.TargetID != "" || existing.TargetDigest != [sha256.Size]byte{} || len(existing.Metadata) != 0 {
						return ErrConflict
					}
					replayed = true
					return nil
				}
				return importer.journal.Record(tx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
					Disposition: "quarantine", Reason: reason})
			}
			receipt, err := importer.writer.Import(tx, definition)
			replayed = receipt.Replayed
			return err
		}); err != nil {
			return err
		}
		if reason == "" {
			result.Imported++
		} else {
			result.Quarantined++
		}
		if replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

func adaptArchivedProduct(row v1archive.ArchivedRow, actorID int64) (product.HistoricalStaticProductDefinition, string) {
	for _, field := range row.RedactedFields {
		switch field {
		case "id", "product_code", "name", "amount_total", "currency", "status", "enabled", "created_at", "updated_at":
			return product.HistoricalStaticProductDefinition{}, "redacted_product_definition"
		}
	}
	var source productJSON
	if json.Unmarshal(row.Payload, &source) != nil {
		return product.HistoricalStaticProductDefinition{}, "invalid_product_json"
	}
	if source.ID == nil || source.ProductCode == nil || source.Name == nil || source.AmountTotal == nil || source.Currency == nil ||
		source.Status == nil || source.Enabled == nil || source.CreatedAt == nil || source.UpdatedAt == nil {
		return product.HistoricalStaticProductDefinition{}, "invalid_product_definition"
	}
	if *source.Currency != "CNY" {
		return product.HistoricalStaticProductDefinition{}, "unsupported_product_currency"
	}
	// The source schema is integer cents, not decimal yuan or an alternate unit.
	if *source.AmountTotal < 0 || *source.AmountTotal > math.MaxInt32 || strings.ContainsRune(*source.ProductCode+*source.Name+*source.Status, '\x00') {
		return product.HistoricalStaticProductDefinition{}, "invalid_product_definition"
	}
	definition, err := product.AdaptV1WeChatPayProductStatic(SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC,
		product.V1WeChatPayProductStaticRow{ID: *source.ID, ProductCode: *source.ProductCode, Name: *source.Name,
			AmountTotal: *source.AmountTotal, Currency: *source.Currency, Status: *source.Status, Enabled: *source.Enabled,
			CreatedAt: *source.CreatedAt, UpdatedAt: *source.UpdatedAt}, actorID)
	if err != nil {
		return product.HistoricalStaticProductDefinition{}, "invalid_product_definition"
	}
	return definition, ""
}
