package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var errInvalidMediaStaticSource = errors.New("invalid archived static media definition")

type MediaStaticImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  *media.HistoricalStaticWriter
	journal *Journal
	kind    media.HistoricalStaticKind
	actorID int64
}

func NewMediaStaticImporter(archive ArchiveSource, uow UnitOfWork, writer *media.HistoricalStaticWriter, journal *Journal, kind media.HistoricalStaticKind, actorID int64) (*MediaStaticImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || actorID < 1 || !journal.validMediaStaticScope(kind) {
		return nil, ErrInvalidScope
	}
	return &MediaStaticImporter{archive: archive, uow: uow, writer: writer, journal: journal, kind: kind, actorID: actorID}, nil
}

type mediaStaticJSON struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	FileName            string     `json:"file_name"`
	MimeType            string     `json:"mime_type"`
	FileSize            int64      `json:"file_size"`
	DataBase64          string     `json:"data_base64"`
	Description         string     `json:"description"`
	Tags                []string   `json:"tags"`
	Category            string     `json:"category"`
	SourceURL           string     `json:"source_url"`
	ThumbMediaID        string     `json:"thumb_media_id"`
	ThumbMediaExpiresAt *time.Time `json:"thumb_media_id_expires_at"`
	MediaID             string     `json:"media_id"`
	MediaExpiresAt      *time.Time `json:"media_id_expires_at"`
	Enabled             bool       `json:"enabled"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (importer *MediaStaticImporter) Import(ctx context.Context, archiveRunID string) (StaticImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.actorID < 1 ||
		!importer.journal.validMediaStaticScope(importer.kind) || archiveRunID != importer.journal.scope.ArchiveRunID {
		return StaticImportResult{}, ErrInvalidScope
	}
	result := StaticImportResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, importer.journal.scope.TableID, func(row v1archive.ArchivedRow) error {
		if row.TableID != importer.journal.scope.TableID || row.AdapterID != importer.journal.scope.AdapterID {
			return ErrInvalidScope
		}
		if row.SourceOrdinal < 1 || row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} {
			return ErrConflict
		}
		var receipt media.HistoricalStaticReceipt
		invalid := false
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			// UnitOfWork may retry. Only the final attempt contributes counts.
			receipt, invalid = media.HistoricalStaticReceipt{}, false
			var source mediaStaticJSON
			var err error
			if json.Unmarshal(row.Payload, &source) != nil || redactedMediaStaticDefinition(row.RedactedFields) {
				err = errInvalidMediaStaticSource
			} else {
				receipt, err = importer.importSource(tx, source, row)
			}
			// Only decoding/adapter failures are quarantine decisions. Any writer
			// error must roll back, because target writes may already have happened.
			if errors.Is(err, errInvalidMediaStaticSource) {
				invalid = true
				_, found, loadErr := importer.journal.LoadTerminal(tx, SourceIdentifier(row.SourceKeyHMAC))
				if loadErr != nil {
					return loadErr
				}
				recordErr := importer.journal.Record(tx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
					Disposition: "quarantine", Reason: "invalid_static_media_definition"})
				receipt.Replayed = found
				return recordErr
			}
			return err
		}); err != nil {
			return err
		}
		if invalid {
			result.Quarantined++
		} else {
			result.Imported++
		}
		if receipt.Replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

func (importer *MediaStaticImporter) importSource(ctx context.Context, source mediaStaticJSON, row v1archive.ArchivedRow) (media.HistoricalStaticReceipt, error) {
	origin := media.HistoricalStaticOrigin{SourceIdentifier: SourceIdentifier(row.SourceKeyHMAC), SourceID: source.ID, PayloadDigest: row.PayloadHMAC}
	if importer.kind == media.HistoricalImage {
		definition, err := media.AdaptV1ImageLibrary(media.V1ImageLibraryRow{
			ID: source.ID, Name: source.Name, FileName: source.FileName, MimeType: source.MimeType, FileSize: source.FileSize,
			DataBase64: source.DataBase64, Description: source.Description, Tags: source.Tags, Category: source.Category,
			SourceURL: source.SourceURL, ThumbMediaID: source.ThumbMediaID, ThumbMediaExpiresAt: source.ThumbMediaExpiresAt,
			Enabled: source.Enabled, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		}, origin, importer.actorID)
		if err != nil {
			return media.HistoricalStaticReceipt{}, errInvalidMediaStaticSource
		}
		return importer.writer.ImportImage(ctx, definition)
	}
	definition, err := media.AdaptV1AttachmentLibrary(media.V1AttachmentLibraryRow{
		ID: source.ID, Name: source.Name, FileName: source.FileName, MimeType: source.MimeType, FileSize: source.FileSize,
		DataBase64: source.DataBase64, Description: source.Description, Tags: source.Tags,
		MediaID: source.MediaID, MediaExpiresAt: source.MediaExpiresAt, Enabled: source.Enabled,
		CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}, origin, importer.actorID)
	if err != nil {
		return media.HistoricalStaticReceipt{}, errInvalidMediaStaticSource
	}
	return importer.writer.ImportAttachment(ctx, definition)
}

func redactedMediaStaticDefinition(fields []string) bool {
	for _, field := range fields {
		switch field {
		case "id", "name", "file_name", "mime_type", "file_size", "data_base64", "description", "tags", "category", "created_at", "updated_at":
			return true
		}
	}
	return false
}
