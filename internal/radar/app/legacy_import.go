package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

// LegacyRadarLinkRow is the import-safe V1 radar_links subset. It deliberately
// excludes enabled, click, media, campaign, staff, and runtime fields.
type LegacyRadarLinkRow struct {
	SourceID         int64
	Code             string
	Title            string
	OriginalURL      string
	MigrationActorID int64 // explicit migration actor, never the V1 created_by value
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type LegacyRadarDraftImport struct {
	Response radarport.LinkResponse
	Replayed bool
}

// ImportLegacyDraft imports one reviewed static V1 definition as a draft. It
// never calls a Provider, records tracking, appends an event, or changes a
// draft into a publicly available link.
func (service *Service) ImportLegacyDraft(ctx context.Context, row LegacyRadarLinkRow) (LegacyRadarDraftImport, error) {
	if !legacyImportReady(service) || ctx == nil || ctx.Err() != nil {
		return LegacyRadarDraftImport{}, radarport.ErrUnavailable
	}
	record, err := AdaptLegacyRadarDraft(row)
	if err != nil {
		return LegacyRadarDraftImport{}, err
	}
	writer, ok := service.repository.(radarport.HistoricalDraftImporter)
	if !ok || writer == nil {
		return LegacyRadarDraftImport{}, radarport.ErrUnavailable
	}
	var result LegacyRadarDraftImport
	err = service.uow.Within(ctx, func(tx context.Context) error {
		link, replayed, writeErr := writer.ImportHistoricalDraft(tx, record)
		if writeErr != nil {
			return writeErr
		}
		if link.Status != radarport.StatusDraft || !link.CreatedAt.Equal(record.CreatedAt) || !link.UpdatedAt.Equal(record.UpdatedAt) {
			return radarport.ErrUnavailable
		}
		response, responseErr := linkResult(link)
		if responseErr != nil {
			return responseErr
		}
		result = LegacyRadarDraftImport{Response: response, Replayed: replayed}
		return nil
	})
	if err != nil {
		return LegacyRadarDraftImport{}, classify(err)
	}
	return result, nil
}

// AdaptLegacyRadarDraft validates the static V1 row and derives a local code
// from its source ID. It intentionally does not reuse the legacy public code.
func AdaptLegacyRadarDraft(row LegacyRadarLinkRow) (radarport.HistoricalDraftRecord, error) {
	if row.SourceID < 1 || row.MigrationActorID < 1 || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) || strings.TrimSpace(row.Code) != row.Code || strings.TrimSpace(row.Title) != row.Title || strings.TrimSpace(row.OriginalURL) != row.OriginalURL {
		return radarport.HistoricalDraftRecord{}, radarport.Invalid("legacy_radar_link", "invalid")
	}
	if err := validateName(row.Code); err != nil {
		return radarport.HistoricalDraftRecord{}, err
	}
	if err := validateTitle(row.Title); err != nil {
		return radarport.HistoricalDraftRecord{}, err
	}
	if err := ValidateDestinationURL(row.OriginalURL); err != nil {
		return radarport.HistoricalDraftRecord{}, err
	}
	return radarport.HistoricalDraftRecord{
		SourceID:       row.SourceID,
		PublicCode:     legacyRadarPublicCode(row.SourceID),
		Name:           row.Code,
		Title:          row.Title,
		DestinationURL: row.OriginalURL,
		ActorID:        row.MigrationActorID,
		CreatedAt:      row.CreatedAt.UTC(),
		UpdatedAt:      row.UpdatedAt.UTC(),
	}, nil
}

func legacyRadarPublicCode(sourceID int64) string {
	digest := sha256.Sum256([]byte("radar/legacy-draft/v1/" + strconv.FormatInt(sourceID, 10)))
	return "rd_" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func legacyImportReady(service *Service) bool {
	return service != nil && !nilDependency(service.uow) && !nilDependency(service.repository)
}
