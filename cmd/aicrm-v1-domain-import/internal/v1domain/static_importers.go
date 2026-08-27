package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

type StaticImportResult struct {
	Imported    int
	Quarantined int
	Replayed    int
}

type MiniProgramImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	writer  *media.HistoricalMiniProgramWriter
	journal *Journal
	actorID int64
}

func NewMiniProgramImporter(archive ArchiveSource, uow UnitOfWork, writer *media.HistoricalMiniProgramWriter, journal *Journal, actorID int64) (*MiniProgramImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	return &MiniProgramImporter{archive: archive, uow: uow, writer: writer, journal: journal, actorID: actorID}, nil
}

type miniProgramJSON struct {
	ID                      int64      `json:"id"`
	Name                    string     `json:"name"`
	AppID                   string     `json:"appid"`
	PagePath                string     `json:"pagepath"`
	Title                   string     `json:"title"`
	ThumbnailImageURL       string     `json:"thumb_image_url"`
	ThumbnailImageBase64    string     `json:"thumb_image_base64"`
	ThumbnailMediaID        string     `json:"thumb_media_id"`
	ThumbnailMediaExpiresAt *time.Time `json:"thumb_media_id_expires_at"`
	Enabled                 bool       `json:"enabled"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func (importer *MiniProgramImporter) Import(ctx context.Context, archiveRunID string) (StaticImportResult, error) {
	if importer == nil || archiveRunID == "" {
		return StaticImportResult{}, ErrInvalidScope
	}
	result := StaticImportResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, "public/miniprogram_library", func(row v1archive.ArchivedRow) error {
		var source miniProgramJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return fmt.Errorf("decode archived miniprogram row %d: %w", row.SourceOrdinal, err)
		}
		definition, err := media.AdaptV1MiniProgramLibrary(media.V1MiniProgramLibraryRow{
			ID: source.ID, Name: source.Name, AppID: source.AppID, PagePath: source.PagePath, Title: source.Title,
			ThumbnailImageURL: source.ThumbnailImageURL, ThumbnailImageBase64: source.ThumbnailImageBase64,
			ThumbnailMediaID: source.ThumbnailMediaID, ThumbnailMediaExpiresAt: source.ThumbnailMediaExpiresAt,
			Enabled: source.Enabled, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		}, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, importer.actorID)
		if errors.Is(err, media.ErrHistoricalMiniProgramInvalid) {
			if recordErr := importer.uow.Within(ctx, func(tx context.Context) error {
				return importer.journal.Record(tx, TerminalReceipt{
					SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
					Disposition: "quarantine", Reason: "invalid_miniprogram_definition",
				})
			}); recordErr != nil {
				return recordErr
			}
			result.Quarantined++
			return nil
		}
		if err != nil {
			return err
		}
		replayed := false
		if err = importer.uow.Within(ctx, func(tx context.Context) error {
			replayed = false
			receipt, writeErr := importer.writer.Import(tx, definition)
			replayed = receipt.Replayed
			return writeErr
		}); err != nil {
			return err
		}
		result.Imported++
		if replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}

type RadarDraftImporter interface {
	ImportLegacyDraft(context.Context, radarapp.LegacyRadarLinkRow) (radarapp.LegacyRadarDraftImport, error)
}

type RadarImporter struct {
	archive ArchiveSource
	uow     UnitOfWork
	service RadarDraftImporter
	journal *Journal
	actorID int64
}

func NewRadarImporter(archive ArchiveSource, uow UnitOfWork, service RadarDraftImporter, journal *Journal, actorID int64) (*RadarImporter, error) {
	if archive == nil || uow == nil || service == nil || journal == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	return &RadarImporter{archive: archive, uow: uow, service: service, journal: journal, actorID: actorID}, nil
}

type radarJSON struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Title       string    `json:"title"`
	OriginalURL string    `json:"original_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (importer *RadarImporter) Import(ctx context.Context, archiveRunID string) (StaticImportResult, error) {
	if importer == nil || archiveRunID == "" {
		return StaticImportResult{}, ErrInvalidScope
	}
	result := StaticImportResult{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, "public/radar_links", func(row v1archive.ArchivedRow) error {
		var source radarJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return fmt.Errorf("decode archived radar row %d: %w", row.SourceOrdinal, err)
		}
		imported, err := importer.service.ImportLegacyDraft(ctx, radarapp.LegacyRadarLinkRow{
			SourceID: source.ID, Code: source.Code, Title: source.Title, OriginalURL: source.OriginalURL,
			MigrationActorID: importer.actorID, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		})
		if errors.Is(err, radarport.ErrInvalidArgument) {
			if recordErr := importer.uow.Within(ctx, func(tx context.Context) error {
				return importer.journal.Record(tx, TerminalReceipt{
					SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
					Disposition: "quarantine", Reason: "invalid_radar_definition",
				})
			}); recordErr != nil {
				return recordErr
			}
			result.Quarantined++
			return nil
		}
		if err != nil {
			return err
		}
		targetJSON, err := json.Marshal(imported.Response.Link)
		if err != nil {
			return err
		}
		targetDigest := sha256.Sum256(targetJSON)
		if err = importer.uow.Within(ctx, func(tx context.Context) error {
			return importer.journal.Record(tx, TerminalReceipt{
				SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC,
				Disposition: "import", TargetID: strconv.FormatInt(int64(imported.Response.Link.LinkID), 10),
				TargetDigest: targetDigest,
			})
		}); err != nil {
			return err
		}
		result.Imported++
		if imported.Replayed {
			result.Replayed++
		}
		return nil
	})
	return result, err
}
