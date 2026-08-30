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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
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

type EditableProductProjectionResult struct {
	Projected   int `json:"projected"`
	Replayed    int `json:"replayed"`
	Quarantined int `json:"quarantined"`
}

// The writer and terminal journal must share the same scoped Journal and
// caller-owned UnitOfWork. This importer never invokes the live Product app.
func NewProductImporter(archive ArchiveSource, uow UnitOfWork, writer *product.HistoricalStaticProductWriter, journal ProductImportJournal, actorID int64) (*ProductImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	return &ProductImporter{archive: archive, uow: uow, writer: writer, journal: journal, actorID: actorID}, nil
}

// Pointers distinguish missing/null facts from valid zero cents or
// disabled=false. Editable local configuration is restored; Provider secrets,
// runtime receipts, and executable effects remain outside this projection.
type productJSON struct {
	ID                        *int64          `json:"id"`
	ProductCode               *string         `json:"product_code"`
	Name                      *string         `json:"name"`
	AmountTotal               *int64          `json:"amount_total"`
	Currency                  *string         `json:"currency"`
	Status                    *string         `json:"status"`
	Enabled                   *bool           `json:"enabled"`
	CTAText                   *string         `json:"cta_text"`
	RequireMobile             *bool           `json:"require_mobile"`
	LeadProgramID             json.RawMessage `json:"lead_program_id"`
	LeadChannelID             json.RawMessage `json:"lead_channel_id"`
	Metadata                  json.RawMessage `json:"metadata_json"`
	CompletionRedirectEnabled *bool           `json:"completion_redirect_enabled"`
	CompletionRedirectURL     *string         `json:"completion_redirect_url"`
	CompletionTarget          json.RawMessage `json:"completion_target_json"`
	WeComTagging              json.RawMessage `json:"wecom_tagging_json"`
	LeadQRTitle               *string         `json:"lead_qr_title"`
	LeadQRSubtitle            *string         `json:"lead_qr_subtitle"`
	CreatedAt                 *time.Time      `json:"created_at"`
	UpdatedAt                 *time.Time      `json:"updated_at"`
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
		definition, _, reason := adaptArchivedProduct(row, importer.actorID)
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
			if err != nil {
				return err
			}
			replayed = receipt.Replayed
			return nil
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

// ProjectEditable restores current local Product configuration only after the
// immutable import receipts have been reconciled. It refuses to create a
// missing historical Product and never invokes a live Provider or entitlement
// path.
func (importer *ProductImporter) ProjectEditable(ctx context.Context, archiveRunID string, at time.Time) (EditableProductProjectionResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.journal == nil || importer.actorID < 1 ||
		at.IsZero() || at.Location() != time.UTC {
		return EditableProductProjectionResult{}, ErrInvalidScope
	}
	if err := importer.journal.ValidateProductImportScope(archiveRunID); err != nil {
		return EditableProductProjectionResult{}, err
	}
	result := EditableProductProjectionResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, productTableID, func(row v1archive.ArchivedRow) error {
		if row.TableID != productTableID || row.AdapterID != v1archive.DefaultAdapterID || row.SourceOrdinal < 1 ||
			row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} {
			return ErrConflict
		}
		definition, editable, reason := adaptArchivedProduct(row, importer.actorID)
		if reason != "" {
			result.Quarantined++
			return nil
		}
		var replayed bool
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			receipt, err := importer.writer.Import(tx, definition)
			if err != nil {
				return err
			}
			if !receipt.Replayed {
				return ErrConflict
			}
			editable.TargetProductID = receipt.TargetProductID
			editable.ProjectedAt = at
			replayed, err = importer.writer.ProjectEditable(tx, editable)
			return err
		}); err != nil {
			return err
		}
		if replayed {
			result.Replayed++
		} else {
			result.Projected++
		}
		return nil
	})
	return result, err
}

func adaptArchivedProduct(row v1archive.ArchivedRow, actorID int64) (product.HistoricalStaticProductDefinition, product.HistoricalEditableProductProjection, string) {
	for _, field := range row.RedactedFields {
		switch field {
		case "id", "product_code", "name", "amount_total", "currency", "status", "enabled", "cta_text", "require_mobile", "lead_program_id", "lead_channel_id", "metadata_json", "completion_redirect_enabled", "completion_redirect_url", "completion_target_json", "wecom_tagging_json", "lead_qr_title", "lead_qr_subtitle", "created_at", "updated_at":
			return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "redacted_product_definition"
		}
	}
	var source productJSON
	if json.Unmarshal(row.Payload, &source) != nil {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "invalid_product_json"
	}
	if source.ID == nil || source.ProductCode == nil || source.Name == nil || source.AmountTotal == nil || source.Currency == nil ||
		source.Status == nil || source.Enabled == nil || source.CTAText == nil || source.RequireMobile == nil || len(source.LeadProgramID) == 0 || len(source.LeadChannelID) == 0 ||
		len(source.Metadata) == 0 || source.CompletionRedirectEnabled == nil || source.CompletionRedirectURL == nil || len(source.CompletionTarget) == 0 || len(source.WeComTagging) == 0 ||
		source.LeadQRTitle == nil || source.LeadQRSubtitle == nil || source.CreatedAt == nil || source.UpdatedAt == nil {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "invalid_product_definition"
	}
	if *source.Currency != "CNY" {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "unsupported_product_currency"
	}
	// The source schema is integer cents, not decimal yuan or an alternate unit.
	if *source.AmountTotal < 0 || *source.AmountTotal > math.MaxInt32 || strings.ContainsRune(*source.ProductCode+*source.Name+*source.Status, '\x00') {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "invalid_product_definition"
	}
	definition, err := product.AdaptV1WeChatPayProductStatic(SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC,
		product.V1WeChatPayProductStaticRow{ID: *source.ID, ProductCode: *source.ProductCode, Name: *source.Name,
			AmountTotal: *source.AmountTotal, Currency: *source.Currency, Status: *source.Status, Enabled: *source.Enabled,
			CreatedAt: *source.CreatedAt, UpdatedAt: *source.UpdatedAt}, actorID)
	if err != nil {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "invalid_product_definition"
	}
	projection, lifecycle, ok := editableProductProjection(source)
	if !ok {
		return product.HistoricalStaticProductDefinition{}, product.HistoricalEditableProductProjection{}, "invalid_product_editable_projection"
	}
	return definition, product.HistoricalEditableProductProjection{SourceID: *source.ID, PayloadDigest: row.PayloadHMAC, AdminProjection: projection, LocalLifecycle: lifecycle}, ""
}

func editableProductProjection(source productJSON) (json.RawMessage, productport.LocalProductLifecycle, bool) {
	leadProgramID, ok := nullablePositiveInt64(source.LeadProgramID)
	if !ok {
		return nil, "", false
	}
	leadChannelID, ok := nullablePositiveInt64(source.LeadChannelID)
	if !ok || !json.Valid(source.Metadata) || !json.Valid(source.CompletionTarget) || !json.Valid(source.WeComTagging) {
		return nil, "", false
	}
	var metadata, completionTarget, weComTagging any
	if json.Unmarshal(source.Metadata, &metadata) != nil || json.Unmarshal(source.CompletionTarget, &completionTarget) != nil || json.Unmarshal(source.WeComTagging, &weComTagging) != nil {
		return nil, "", false
	}
	projection, err := json.Marshal(map[string]any{
		"schema_version": 1, "status": *source.Status, "enabled": *source.Enabled,
		"buy_button_text": *source.CTAText, "require_mobile": *source.RequireMobile,
		"lead_program_id": leadProgramID, "lead_channel_id": leadChannelID,
		"lead_qr_title": *source.LeadQRTitle, "lead_qr_subtitle": *source.LeadQRSubtitle,
		"completion_redirect_enabled": *source.CompletionRedirectEnabled,
		"completion_redirect_url":     *source.CompletionRedirectURL, "completion_target": completionTarget,
		"wecom_tagging": weComTagging, "slices": []string{},
	})
	if err != nil {
		return nil, "", false
	}
	lifecycle := productport.LocalProductDisabled
	if *source.Enabled && (*source.Status == "active" || *source.Status == "enabled") {
		lifecycle = productport.LocalProductEnabled
	}
	return projection, lifecycle, true
}

func nullablePositiveInt64(raw json.RawMessage) (*int64, bool) {
	if string(raw) == "null" {
		return nil, true
	}
	var value int64
	if json.Unmarshal(raw, &value) != nil || value < 1 {
		return nil, false
	}
	return &value, true
}
