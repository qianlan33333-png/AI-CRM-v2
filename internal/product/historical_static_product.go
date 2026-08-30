package product

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

var (
	ErrHistoricalStaticProductConflict = errors.New("historical static product conflict")
	ErrHistoricalStaticProductInvalid  = errors.New("invalid historical static product")
)

// V1WeChatPayProductStaticRow is the typed, unit-preserving static subset of
// V1 public.wechat_pay_products. AmountTotal is the source integer amount; the
// adapter copies it to V2 PriceMinor without scaling, rounding, or exchange.
// Provider, lead, and runtime fields intentionally stay in the encrypted V1
// archive until their V2-owned mappings are explicitly approved.
type V1WeChatPayProductStaticRow struct {
	ID                          int64
	ProductCode, Name, Currency string
	AmountTotal                 int64
	Status                      string
	Enabled                     bool
	CreatedAt, UpdatedAt        time.Time
}

// HistoricalStaticProductDefinition is a migration-only, disabled V2 Product
// definition. SourceIdentifier is opaque and belongs to the migration journal;
// SourceID is retained only in the receipt/mapping, never as a V2 Product ID.
type HistoricalStaticProductDefinition struct {
	SourceIdentifier string
	SourceID         int64
	PayloadDigest    [32]byte
	OriginalStatus   string
	OriginalEnabled  bool
	Product          productport.Product
}

// HistoricalStaticProductReceipt is persisted by the migration-owned journal.
// It proves a static, disabled import only; it is neither an operation receipt
// nor evidence of a Provider, payment, entitlement, or external side effect.
type HistoricalStaticProductReceipt struct {
	SourceIdentifier  string
	SourceID          int64
	PayloadDigest     [32]byte
	OriginalStatus    string
	OriginalEnabled   bool
	TargetProductID   productport.ID
	TargetProductCode string
	TargetProductName string
	PriceMinor        int64
	Currency          string
	CreatedBy         int64
	Replayed          bool
}

// HistoricalEditableProductProjection is current V2 configuration derived
// from the sealed V1 product row. Persisting it performs no payment, Provider,
// entitlement, queue, or callback action.
type HistoricalEditableProductProjection struct {
	SourceID        int64
	PayloadDigest   [32]byte
	TargetProductID productport.ID
	AdminProjection json.RawMessage
	LocalLifecycle  productport.LocalProductLifecycle
	ProjectedAt     time.Time
}

// HistoricalStaticProductStore is Product-owned migration persistence. It
// inserts the static definition, then may attach the sealed editable local
// configuration in the same transaction. It must not reserve runtime receipts
// or emit Product events as part of an import.
type HistoricalStaticProductStore interface {
	InsertHistoricalStaticProduct(context.Context, HistoricalStaticProductDefinition) (productport.Product, error)
	ProjectHistoricalEditableProduct(context.Context, HistoricalEditableProductProjection) (bool, error)
}

// HistoricalStaticProductJournal is the narrow migration-owned provenance
// seam. Main-line integration supplies the same-UnitOfWork implementation.
type HistoricalStaticProductJournal interface {
	LoadHistoricalStaticProduct(context.Context, string) (HistoricalStaticProductReceipt, bool, error)
	RecordHistoricalStaticProduct(context.Context, HistoricalStaticProductReceipt) error
}

type HistoricalStaticProductWriter struct {
	store   HistoricalStaticProductStore
	journal HistoricalStaticProductJournal
}

func NewHistoricalStaticProductWriter(store HistoricalStaticProductStore, journal HistoricalStaticProductJournal) (*HistoricalStaticProductWriter, error) {
	if store == nil || journal == nil {
		return nil, ErrHistoricalStaticProductInvalid
	}
	return &HistoricalStaticProductWriter{store: store, journal: journal}, nil
}

// AdaptV1WeChatPayProductStatic copies only values with an explicit static V2
// meaning. The resulting product is disabled regardless of the V1 runtime
// state, so importing it cannot make an old product purchasable or activate a
// Provider flow. V1 status/enabled are retained in the migration receipt.
func AdaptV1WeChatPayProductStatic(sourceIdentifier string, payloadDigest [32]byte, source V1WeChatPayProductStaticRow, migrationActor int64) (HistoricalStaticProductDefinition, error) {
	if !validV1WeChatPayProductStatic(source) || !validSourceIdentifier(sourceIdentifier) || migrationActor < 1 {
		return HistoricalStaticProductDefinition{}, ErrHistoricalStaticProductInvalid
	}
	projection, err := disabledHistoricalProductProjection()
	if err != nil {
		return HistoricalStaticProductDefinition{}, ErrHistoricalStaticProductInvalid
	}
	return HistoricalStaticProductDefinition{
		SourceIdentifier: sourceIdentifier,
		SourceID:         source.ID,
		PayloadDigest:    payloadDigest,
		OriginalStatus:   source.Status,
		OriginalEnabled:  source.Enabled,
		Product: productport.Product{
			ProductCode:           source.ProductCode,
			Name:                  source.Name,
			Description:           "",
			PriceMinor:            source.AmountTotal,
			Currency:              source.Currency,
			StockQuantity:         0,
			CreatedBy:             migrationActor,
			CreatedAt:             source.CreatedAt.UTC(),
			UpdatedAt:             source.UpdatedAt.UTC(),
			Version:               1,
			LocalLifecycle:        productport.LocalProductDisabled,
			LegacyAdminProjection: projection,
		},
	}, nil
}

// Import writes one V2 static definition and its provenance in the caller's
// transaction. It intentionally does not call the Product application service.
func (writer *HistoricalStaticProductWriter) Import(ctx context.Context, definition HistoricalStaticProductDefinition) (HistoricalStaticProductReceipt, error) {
	if writer == nil || writer.store == nil || writer.journal == nil || ctx == nil || !validHistoricalStaticProductDefinition(definition) {
		return HistoricalStaticProductReceipt{}, ErrHistoricalStaticProductInvalid
	}
	existing, found, err := writer.journal.LoadHistoricalStaticProduct(ctx, definition.SourceIdentifier)
	if err != nil {
		return HistoricalStaticProductReceipt{}, err
	}
	if found {
		if !sameHistoricalStaticProduct(existing, definition) {
			return HistoricalStaticProductReceipt{}, ErrHistoricalStaticProductConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	stored, err := writer.store.InsertHistoricalStaticProduct(ctx, definition)
	if err != nil {
		return HistoricalStaticProductReceipt{}, err
	}
	if !sameStoredHistoricalStaticProduct(stored, definition.Product) {
		return HistoricalStaticProductReceipt{}, ErrHistoricalStaticProductConflict
	}
	receipt := HistoricalStaticProductReceipt{
		SourceIdentifier:  definition.SourceIdentifier,
		SourceID:          definition.SourceID,
		PayloadDigest:     definition.PayloadDigest,
		OriginalStatus:    definition.OriginalStatus,
		OriginalEnabled:   definition.OriginalEnabled,
		TargetProductID:   stored.ID,
		TargetProductCode: stored.ProductCode,
		TargetProductName: stored.Name,
		PriceMinor:        stored.PriceMinor,
		Currency:          stored.Currency,
		CreatedBy:         stored.CreatedBy,
	}
	if err = writer.journal.RecordHistoricalStaticProduct(ctx, receipt); err != nil {
		return HistoricalStaticProductReceipt{}, err
	}
	return receipt, nil
}

func (writer *HistoricalStaticProductWriter) ProjectEditable(ctx context.Context, projection HistoricalEditableProductProjection) (bool, error) {
	if writer == nil || writer.store == nil || ctx == nil || projection.SourceID < 1 || projection.TargetProductID < 1 ||
		projection.PayloadDigest == [32]byte{} || len(projection.AdminProjection) == 0 || projection.ProjectedAt.IsZero() || projection.ProjectedAt.Location() != time.UTC ||
		(projection.LocalLifecycle != productport.LocalProductEnabled && projection.LocalLifecycle != productport.LocalProductDisabled) {
		return false, ErrHistoricalStaticProductInvalid
	}
	return writer.store.ProjectHistoricalEditableProduct(ctx, projection)
}

func validV1WeChatPayProductStatic(source V1WeChatPayProductStaticRow) bool {
	return source.ID > 0 && validProductCode(source.ProductCode) && validProductName(source.Name) && source.AmountTotal >= 0 && validCurrency(source.Currency) && validSourceStatus(source.Status) && !source.CreatedAt.IsZero() && !source.UpdatedAt.IsZero() && !source.UpdatedAt.Before(source.CreatedAt)
}

func validHistoricalStaticProductDefinition(definition HistoricalStaticProductDefinition) bool {
	projection, err := disabledHistoricalProductProjection()
	return err == nil && validSourceIdentifier(definition.SourceIdentifier) && definition.SourceID > 0 && validSourceStatus(definition.OriginalStatus) &&
		definition.Product.ID == 0 && validProductCode(definition.Product.ProductCode) && validProductName(definition.Product.Name) &&
		definition.Product.Description == "" && definition.Product.PriceMinor >= 0 && validCurrency(definition.Product.Currency) &&
		definition.Product.StockQuantity == 0 && len(definition.Product.Images) == 0 && definition.Product.CreatedBy > 0 &&
		!definition.Product.CreatedAt.IsZero() && !definition.Product.UpdatedAt.IsZero() && !definition.Product.UpdatedAt.Before(definition.Product.CreatedAt) &&
		definition.Product.Version == 1 && definition.Product.LocalLifecycle == productport.LocalProductDisabled && sameHistoricalProductJSON(definition.Product.LegacyAdminProjection, projection)
}

func sameHistoricalStaticProduct(receipt HistoricalStaticProductReceipt, definition HistoricalStaticProductDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.SourceID == definition.SourceID &&
		subtle.ConstantTimeCompare(receipt.PayloadDigest[:], definition.PayloadDigest[:]) == 1 &&
		receipt.OriginalStatus == definition.OriginalStatus && receipt.OriginalEnabled == definition.OriginalEnabled &&
		receipt.TargetProductID > 0 && receipt.TargetProductCode == definition.Product.ProductCode &&
		receipt.TargetProductName == definition.Product.Name && receipt.PriceMinor == definition.Product.PriceMinor &&
		receipt.Currency == definition.Product.Currency && receipt.CreatedBy == definition.Product.CreatedBy
}

func sameStoredHistoricalStaticProduct(stored, expected productport.Product) bool {
	return stored.ID > 0 && stored.ProductCode == expected.ProductCode && stored.Name == expected.Name && stored.Description == expected.Description &&
		stored.PriceMinor == expected.PriceMinor && stored.Currency == expected.Currency && stored.StockQuantity == 0 && len(stored.Images) == 0 &&
		stored.CreatedBy == expected.CreatedBy && stored.CreatedAt.Equal(expected.CreatedAt) && stored.UpdatedAt.Equal(expected.UpdatedAt) &&
		stored.Version == 1 && stored.LocalLifecycle == productport.LocalProductDisabled && sameHistoricalProductJSON(stored.LegacyAdminProjection, expected.LegacyAdminProjection)
}

func sameHistoricalProductJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func disabledHistoricalProductProjection() (json.RawMessage, error) {
	return json.Marshal(struct {
		SchemaVersion             int             `json:"schema_version"`
		Status                    string          `json:"status"`
		Enabled                   bool            `json:"enabled"`
		BuyButtonText             string          `json:"buy_button_text"`
		RequireMobile             bool            `json:"require_mobile"`
		LeadProgramID             *int64          `json:"lead_program_id"`
		LeadChannelID             *int64          `json:"lead_channel_id"`
		LeadQRTitle               string          `json:"lead_qr_title"`
		LeadQRSubtitle            string          `json:"lead_qr_subtitle"`
		CompletionRedirectEnabled bool            `json:"completion_redirect_enabled"`
		CompletionRedirectURL     string          `json:"completion_redirect_url"`
		CompletionTarget          json.RawMessage `json:"completion_target"`
		WeComTagging              json.RawMessage `json:"wecom_tagging"`
		Slices                    []string        `json:"slices"`
	}{
		SchemaVersion: 1, Status: "disabled", Enabled: false,
		CompletionTarget: json.RawMessage("null"), WeComTagging: json.RawMessage("{}"), Slices: []string{},
	})
}

func validSourceIdentifier(value string) bool {
	return value != "" && len(value) <= 512 && value == strings.TrimSpace(value)
}

func validSourceStatus(value string) bool {
	return value != "" && len(value) <= 128 && value == strings.TrimSpace(value)
}

func validProductCode(value string) bool {
	return value != "" && len(value) <= 200 && value == strings.TrimSpace(value)
}

func validProductName(value string) bool {
	return value != "" && len(value) <= 200 && value == strings.TrimSpace(value)
}

func validCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}
