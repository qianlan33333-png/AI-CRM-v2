package media

import (
	"context"
	"crypto/subtle"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

var (
	ErrHistoricalMiniProgramConflict = errors.New("historical miniprogram definition conflict")
	ErrHistoricalMiniProgramInvalid  = errors.New("invalid historical miniprogram definition")
)

// V1MiniProgramLibraryRow is the read-only shape of V1 miniprogram_library.
// Thumbnail fields are accepted only so the adapter can record that they were
// dropped; they never become V2 material fields.
type V1MiniProgramLibraryRow struct {
	ID                                      int64
	Name, AppID, PagePath, Title            string
	ThumbnailImageURL, ThumbnailImageBase64 string
	ThumbnailMediaID                        string
	ThumbnailMediaExpiresAt                 *time.Time
	Enabled                                 bool
	CreatedAt, UpdatedAt                    time.Time
}

// HistoricalMiniProgramDefinition is a local, disabled V2 card definition.
// SourceIdentifier is an opaque migration-owned key; SourceID is the original
// V1 primary key retained by media_miniprograms.legacy_source_id.
type HistoricalMiniProgramDefinition struct {
	SourceIdentifier        string
	SourceID                int64
	PayloadDigest           [32]byte
	Item                    mediaport.MiniProgram
	ProviderMaterialDropped bool
}

// HistoricalMiniProgramReceipt is persisted by the migration-owned journal.
// It records no token, media ID, URL, thumbnail blob, or provider result.
type HistoricalMiniProgramReceipt struct {
	SourceIdentifier        string
	SourceID                int64
	PayloadDigest           [32]byte
	TargetMiniProgramID     int64
	ProviderMaterialDropped bool
	Replayed                bool
}

// HistoricalMiniProgramStore is Media-owned persistence. It must share the
// caller's transaction with HistoricalMiniProgramJournal.
type HistoricalMiniProgramStore interface {
	InsertHistoricalMiniProgram(context.Context, HistoricalMiniProgramDefinition) (int64, error)
}

// HistoricalMiniProgramJournal is migration-owned provenance and idempotency
// storage. Its implementation must use the same UnitOfWork as the Media store.
type HistoricalMiniProgramJournal interface {
	LoadHistoricalMiniProgram(context.Context, string) (HistoricalMiniProgramReceipt, bool, error)
	RecordHistoricalMiniProgram(context.Context, HistoricalMiniProgramReceipt) error
}

type HistoricalMiniProgramWriter struct {
	store   HistoricalMiniProgramStore
	journal HistoricalMiniProgramJournal
}

func NewHistoricalMiniProgramWriter(store HistoricalMiniProgramStore, journal HistoricalMiniProgramJournal) (*HistoricalMiniProgramWriter, error) {
	if nilish(store) || nilish(journal) {
		return nil, ErrHistoricalMiniProgramInvalid
	}
	return &HistoricalMiniProgramWriter{store: store, journal: journal}, nil
}

// AdaptV1MiniProgramLibrary converts only static card metadata. The V1 enabled
// flag is intentionally not carried over: all historical cards remain disabled
// until a separate, explicit V2 approval flow enables them.
func AdaptV1MiniProgramLibrary(source V1MiniProgramLibraryRow, sourceIdentifier string, payloadDigest [32]byte, actor int64) (HistoricalMiniProgramDefinition, error) {
	dropped := source.ThumbnailImageURL != "" || source.ThumbnailImageBase64 != "" || source.ThumbnailMediaID != "" || source.ThumbnailMediaExpiresAt != nil
	definition := HistoricalMiniProgramDefinition{
		SourceIdentifier: sourceIdentifier, SourceID: source.ID, PayloadDigest: payloadDigest, ProviderMaterialDropped: dropped,
		Item: mediaport.MiniProgram{Name: source.Name, AppID: source.AppID, PagePath: source.PagePath, Title: source.Title,
			Enabled: false, CreatedBy: actor, UpdatedBy: actor, Version: 1, CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC()},
	}
	if !validHistoricalMiniProgram(definition) {
		return HistoricalMiniProgramDefinition{}, ErrHistoricalMiniProgramInvalid
	}
	return definition, nil
}

// Import inserts one disabled local definition. It bypasses the normal Media
// service, its operation receipts, events, thumbnail cache, and all Providers.
func (writer *HistoricalMiniProgramWriter) Import(ctx context.Context, definition HistoricalMiniProgramDefinition) (HistoricalMiniProgramReceipt, error) {
	if writer == nil || nilish(writer.store) || nilish(writer.journal) || ctx == nil || !validHistoricalMiniProgram(definition) {
		return HistoricalMiniProgramReceipt{}, ErrHistoricalMiniProgramInvalid
	}
	existing, found, err := writer.journal.LoadHistoricalMiniProgram(ctx, definition.SourceIdentifier)
	if err != nil {
		return HistoricalMiniProgramReceipt{}, err
	}
	if found {
		if !sameHistoricalMiniProgram(existing, definition) {
			return HistoricalMiniProgramReceipt{}, ErrHistoricalMiniProgramConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	targetID, err := writer.store.InsertHistoricalMiniProgram(ctx, definition)
	if err != nil {
		return HistoricalMiniProgramReceipt{}, err
	}
	receipt := HistoricalMiniProgramReceipt{SourceIdentifier: definition.SourceIdentifier, SourceID: definition.SourceID,
		PayloadDigest: definition.PayloadDigest, TargetMiniProgramID: targetID, ProviderMaterialDropped: definition.ProviderMaterialDropped}
	if err = writer.journal.RecordHistoricalMiniProgram(ctx, receipt); err != nil {
		return HistoricalMiniProgramReceipt{}, err
	}
	return receipt, nil
}

func validHistoricalMiniProgram(value HistoricalMiniProgramDefinition) bool {
	item := value.Item
	return value.SourceID > 0 && validHistoricalSourceIdentifier(value.SourceIdentifier) && !item.Enabled && item.ID == 0 && item.Version == 1 &&
		item.ThumbnailImageURL == "" && item.ThumbnailImageBase64 == "" && item.ThumbnailImageID == nil && item.ThumbnailMediaID == "" && item.ThumbnailMediaExpiresAt == nil &&
		!item.CreatedAt.IsZero() && !item.UpdatedAt.IsZero() && !item.UpdatedAt.Before(item.CreatedAt) && domain.ValidMiniProgram(item, false)
}

func validHistoricalSourceIdentifier(value string) bool {
	return value != "" && len(value) <= 512 && value == strings.TrimSpace(value)
}

func sameHistoricalMiniProgram(receipt HistoricalMiniProgramReceipt, definition HistoricalMiniProgramDefinition) bool {
	return receipt.SourceIdentifier == definition.SourceIdentifier && receipt.SourceID == definition.SourceID && receipt.TargetMiniProgramID > 0 &&
		receipt.ProviderMaterialDropped == definition.ProviderMaterialDropped && subtle.ConstantTimeCompare(receipt.PayloadDigest[:], definition.PayloadDigest[:]) == 1
}

func nilish(value any) bool {
	if value == nil {
		return true
	}
	item := reflect.ValueOf(value)
	return (item.Kind() == reflect.Ptr || item.Kind() == reflect.Interface || item.Kind() == reflect.Map || item.Kind() == reflect.Slice || item.Kind() == reflect.Func) && item.IsNil()
}
