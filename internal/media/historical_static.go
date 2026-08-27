package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/media/domain"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

var (
	ErrHistoricalStaticInvalid  = errors.New("invalid historical static media")
	ErrHistoricalStaticConflict = errors.New("historical static media conflict")
)

type HistoricalStaticKind string

const (
	HistoricalImage      HistoricalStaticKind = "image_library"
	HistoricalAttachment HistoricalStaticKind = "attachment_library"
)

// HistoricalStaticOrigin is authenticated by the migration-owned archive
// adapter. PayloadDigest covers the original complete row, not its projection.
type HistoricalStaticOrigin struct {
	SourceIdentifier string
	SourceID         int64
	PayloadDigest    [32]byte
}

// These DTOs accept the complete archived data_base64, never a URL or a
// Provider media ID. V1 has no blob checksum column: after strict size/base64
// validation the adapter computes SHA-256, which the writer and store recheck.
type V1ImageLibraryRow struct {
	ID                                             int64
	Name, FileName, MimeType, DataBase64           string
	Description, Category, SourceURL, ThumbMediaID string
	Tags                                           []string
	FileSize                                       int64
	ThumbMediaExpiresAt                            *time.Time
	Enabled                                        bool
	CreatedAt, UpdatedAt                           time.Time
}

type V1AttachmentLibraryRow struct {
	ID                                                int64
	Name, FileName, MimeType, DataBase64, Description string
	Tags                                              []string
	FileSize                                          int64
	MediaID                                           string
	MediaExpiresAt                                    *time.Time
	Enabled                                           bool
	CreatedAt, UpdatedAt                              time.Time
}

type HistoricalImageDefinition struct {
	Origin                  HistoricalStaticOrigin
	Image                   mediaport.Image
	Content                 []byte
	Checksum                [32]byte
	Actor                   int64
	ProviderMaterialDropped bool
}

type HistoricalAttachmentDefinition struct {
	Origin                  HistoricalStaticOrigin
	Attachment              mediaport.Attachment
	Content                 []byte
	Checksum                [32]byte
	ProviderMaterialDropped bool
}

// HistoricalStaticReceipt contains only provenance and integrity facts.
type HistoricalStaticReceipt struct {
	Origin                  HistoricalStaticOrigin
	Kind                    HistoricalStaticKind
	TargetID                int64
	Checksum                [32]byte
	DefinitionDigest        [32]byte
	ProviderMaterialDropped bool
	Replayed                bool
}

type HistoricalStaticStore interface {
	InsertHistoricalImage(context.Context, HistoricalImageDefinition) (int64, error)
	InsertHistoricalAttachment(context.Context, HistoricalAttachmentDefinition) (int64, error)
}

// HistoricalStaticJournal must serialize a source key even before its first
// receipt exists, and persist the receipt in the same caller-owned UnitOfWork
// as HistoricalStaticStore. Any Import error requires rollback of that UnitOfWork.
type HistoricalStaticJournal interface {
	LoadHistoricalStatic(context.Context, string) (HistoricalStaticReceipt, bool, error)
	RecordHistoricalStatic(context.Context, HistoricalStaticReceipt) error
}

type HistoricalStaticWriter struct {
	store   HistoricalStaticStore
	journal HistoricalStaticJournal
}

func NewHistoricalStaticWriter(store HistoricalStaticStore, journal HistoricalStaticJournal) (*HistoricalStaticWriter, error) {
	if staticNil(store) || staticNil(journal) {
		return nil, ErrHistoricalStaticInvalid
	}
	return &HistoricalStaticWriter{store: store, journal: journal}, nil
}

func AdaptV1ImageLibrary(source V1ImageLibraryRow, origin HistoricalStaticOrigin, actor int64) (HistoricalImageDefinition, error) {
	size := source.FileSize
	// V1 ImportImageFromBase64Command stored the encoded character count.
	// Only that exact convention is normalized; original metadata stays archived.
	if size > 0 && size == int64(len(source.DataBase64)) && size <= int64(base64.StdEncoding.EncodedLen(domain.MaxImageBytes)) {
		decoded, err := base64.StdEncoding.Strict().DecodeString(source.DataBase64)
		if err != nil {
			return HistoricalImageDefinition{}, ErrHistoricalStaticInvalid
		}
		size = int64(len(decoded))
	}
	content, err := decodeHistoricalStatic(source.DataBase64, size, domain.MaxImageBytes)
	if err != nil || source.ID != origin.SourceID {
		return HistoricalImageDefinition{}, ErrHistoricalStaticInvalid
	}
	inspection, err := domain.Inspect(source.FileName, source.MimeType, content)
	tags, tagsErr := historicalStaticTags(source.Tags)
	if err != nil || tagsErr != nil {
		return HistoricalImageDefinition{}, ErrHistoricalStaticInvalid
	}
	definition := HistoricalImageDefinition{Origin: origin, Actor: actor, Content: content, Checksum: sha256.Sum256(content),
		ProviderMaterialDropped: source.ThumbMediaID != "" || source.ThumbMediaExpiresAt != nil,
		Image: mediaport.Image{Name: strings.TrimSpace(source.Name), FileName: source.FileName, MimeType: inspection.MediaType,
			FileSize: int32(len(content)), Width: inspection.Width, Height: inspection.Height, Description: strings.TrimSpace(source.Description),
			Tags: strings.Join(tags, ","), Category: strings.TrimSpace(source.Category), Enabled: false,
			CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC()}}
	return definition, definition.Validate()
}

func AdaptV1AttachmentLibrary(source V1AttachmentLibraryRow, origin HistoricalStaticOrigin, actor int64) (HistoricalAttachmentDefinition, error) {
	content, err := decodeHistoricalStatic(source.DataBase64, source.FileSize, domain.MaxAttachmentBytes)
	tags, tagsErr := historicalStaticTags(source.Tags)
	if err != nil || tagsErr != nil || source.ID != origin.SourceID {
		return HistoricalAttachmentDefinition{}, ErrHistoricalStaticInvalid
	}
	inspection, err := domain.InspectAttachment(source.FileName, source.MimeType, content)
	if err != nil {
		return HistoricalAttachmentDefinition{}, ErrHistoricalStaticInvalid
	}
	definition := HistoricalAttachmentDefinition{Origin: origin, Content: content, Checksum: sha256.Sum256(content),
		ProviderMaterialDropped: source.MediaID != "" || source.MediaExpiresAt != nil,
		Attachment: mediaport.Attachment{Name: strings.TrimSpace(source.Name), FileName: source.FileName, MimeType: inspection.MediaType,
			FileSize: int64(len(content)), Description: strings.TrimSpace(source.Description), Tags: tags, Enabled: false, Version: 1,
			CreatedBy: actor, UpdatedBy: actor, CreatedAt: source.CreatedAt.UTC(), UpdatedAt: source.UpdatedAt.UTC()}}
	return definition, definition.Validate()
}

func (definition HistoricalImageDefinition) Validate() error {
	item := definition.Image
	inspection, err := domain.Inspect(item.FileName, item.MimeType, definition.Content)
	if err != nil || !validHistoricalStaticOrigin(definition.Origin) || definition.Actor < 1 || item.ID != 0 || item.Enabled ||
		item.MimeType != inspection.MediaType || item.Width != inspection.Width || item.Height != inspection.Height || int64(item.FileSize) != int64(len(definition.Content)) ||
		definition.Checksum != sha256.Sum256(definition.Content) || !historicalStaticTimes(item.CreatedAt, item.UpdatedAt) ||
		!historicalStaticText(item.Name, 200) || !historicalStaticText(item.Description, 10_000) || !historicalStaticText(item.Tags, 10_000) || !historicalStaticText(item.Category, 200) {
		return ErrHistoricalStaticInvalid
	}
	return nil
}

func (definition HistoricalAttachmentDefinition) Validate() error {
	item := definition.Attachment
	inspection, err := domain.InspectAttachment(item.FileName, item.MimeType, definition.Content)
	_, tagsErr := historicalStaticTags(item.Tags)
	if err != nil || tagsErr != nil || !bytes.HasSuffix(bytes.TrimSpace(definition.Content), []byte("%%EOF")) || !validHistoricalStaticOrigin(definition.Origin) ||
		item.ID != 0 || item.Enabled || item.Version != 1 || item.CreatedBy < 1 || item.UpdatedBy != item.CreatedBy || item.MimeType != inspection.MediaType ||
		item.FileSize != int64(len(definition.Content)) || definition.Checksum != sha256.Sum256(definition.Content) || !historicalStaticTimes(item.CreatedAt, item.UpdatedAt) ||
		item.Name == "" || item.Name != strings.TrimSpace(item.Name) || item.Description != strings.TrimSpace(item.Description) ||
		!historicalStaticText(item.Name, 200) || !historicalStaticText(item.Description, 10_000) {
		return ErrHistoricalStaticInvalid
	}
	return nil
}

func (writer *HistoricalStaticWriter) ImportImage(ctx context.Context, definition HistoricalImageDefinition) (HistoricalStaticReceipt, error) {
	if err := definition.Validate(); err != nil {
		return HistoricalStaticReceipt{}, err
	}
	return writer.importStatic(ctx, definition.Origin, HistoricalImage, definition.Checksum, definition.ProviderMaterialDropped,
		struct {
			Image mediaport.Image
			Actor int64
		}{definition.Image, definition.Actor},
		func() (int64, error) { return writer.store.InsertHistoricalImage(ctx, definition) })
}

func (writer *HistoricalStaticWriter) ImportAttachment(ctx context.Context, definition HistoricalAttachmentDefinition) (HistoricalStaticReceipt, error) {
	if err := definition.Validate(); err != nil {
		return HistoricalStaticReceipt{}, err
	}
	return writer.importStatic(ctx, definition.Origin, HistoricalAttachment, definition.Checksum, definition.ProviderMaterialDropped, definition.Attachment,
		func() (int64, error) { return writer.store.InsertHistoricalAttachment(ctx, definition) })
}

func (writer *HistoricalStaticWriter) importStatic(ctx context.Context, origin HistoricalStaticOrigin, kind HistoricalStaticKind, checksum [32]byte, dropped bool, metadata any, insert func() (int64, error)) (HistoricalStaticReceipt, error) {
	if !writer.ready(ctx) {
		return HistoricalStaticReceipt{}, ErrHistoricalStaticInvalid
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return HistoricalStaticReceipt{}, ErrHistoricalStaticInvalid
	}
	receipt := HistoricalStaticReceipt{Origin: origin, Kind: kind, Checksum: checksum, DefinitionDigest: sha256.Sum256(encoded), ProviderMaterialDropped: dropped}
	existing, found, err := writer.journal.LoadHistoricalStatic(ctx, origin.SourceIdentifier)
	if err != nil {
		return HistoricalStaticReceipt{}, err
	}
	if found {
		receipt.TargetID, receipt.Replayed = existing.TargetID, existing.Replayed
		if receipt.TargetID < 1 || receipt != existing {
			return HistoricalStaticReceipt{}, ErrHistoricalStaticConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	receipt.TargetID, err = insert()
	if err != nil {
		return HistoricalStaticReceipt{}, err
	}
	if receipt.TargetID < 1 {
		return HistoricalStaticReceipt{}, ErrHistoricalStaticInvalid
	}
	if err = writer.journal.RecordHistoricalStatic(ctx, receipt); err != nil {
		return HistoricalStaticReceipt{}, err
	}
	return receipt, nil
}

func (writer *HistoricalStaticWriter) ready(ctx context.Context) bool {
	return writer != nil && ctx != nil && !staticNil(writer.store) && !staticNil(writer.journal)
}

func decodeHistoricalStatic(encoded string, size int64, maximum int) ([]byte, error) {
	if size < 1 || size > int64(maximum) || len(encoded) != base64.StdEncoding.EncodedLen(int(size)) {
		return nil, ErrHistoricalStaticInvalid
	}
	content, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || int64(len(content)) != size || base64.StdEncoding.EncodeToString(content) != encoded {
		return nil, ErrHistoricalStaticInvalid
	}
	return content, nil
}

func historicalStaticTags(values []string) ([]string, error) {
	if len(values) > 50 {
		return nil, ErrHistoricalStaticInvalid
	}
	tags := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || !historicalStaticText(value, 64) || strings.Contains(value, ",") || seen[value] {
			return nil, ErrHistoricalStaticInvalid
		}
		seen[value] = true
		tags = append(tags, value)
	}
	return tags, nil
}

func validHistoricalStaticOrigin(origin HistoricalStaticOrigin) bool {
	return origin.SourceID > 0 && origin.PayloadDigest != [32]byte{} && origin.SourceIdentifier != "" &&
		len(origin.SourceIdentifier) <= 512 && origin.SourceIdentifier == strings.TrimSpace(origin.SourceIdentifier) && historicalStaticText(origin.SourceIdentifier, 512)
}

func historicalStaticTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}

func historicalStaticText(value string, maximum int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum && !strings.ContainsRune(value, '\x00')
}

func staticNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Ptr || reflected.Kind() == reflect.Interface) && reflected.IsNil()
}
