package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radardb "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store/generated"
)

func (repository *PostgresRepository) GetEnabledByCode(ctx context.Context, code string) (radarport.Link, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return radarport.Link{}, err
	}
	row, err := queries.GetEnabledRadarLinkByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Link{}, radarport.ErrNotFound
	}
	if err != nil {
		return radarport.Link{}, err
	}
	return trackingLink(row)
}

func (repository *PostgresRepository) InsertEvent(ctx context.Context, record radarport.InsertEventRecord) (radarport.Event, bool, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return radarport.Event{}, false, err
	}
	row, err := queries.InsertRadarLinkEvent(ctx, radardb.InsertRadarLinkEventParams{
		ReceiptID: record.ReceiptID, LinkID: int64(record.LinkID), Stage: string(record.Stage),
		PageNo: nullableInt4(record.Page), Source: string(record.Source), KeyDigest: record.KeyDigest,
		PayloadDigest: record.PayloadDigest[:], CreatedAt: pgtype.Timestamptz{Time: record.CreatedAt, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) && len(record.KeyDigest) == 32 {
		return radarport.Event{}, false, nil
	}
	if err != nil {
		return radarport.Event{}, false, err
	}
	event, err := trackingEvent(row.ID, row.ReceiptID, row.LinkID, row.Stage, row.PageNo, row.Source, row.CreatedAt)
	return event, true, err
}

func (repository *PostgresRepository) GetEventByKey(ctx context.Context, linkID radarport.LinkID, key []byte) (radarport.Event, [32]byte, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return radarport.Event{}, [32]byte{}, err
	}
	row, err := queries.GetRadarLinkEventByKey(ctx, radardb.GetRadarLinkEventByKeyParams{LinkID: int64(linkID), KeyDigest: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return radarport.Event{}, [32]byte{}, radarport.ErrUnavailable
	}
	if err != nil {
		return radarport.Event{}, [32]byte{}, err
	}
	if len(row.PayloadDigest) != 32 {
		return radarport.Event{}, [32]byte{}, radarport.ErrUnavailable
	}
	event, err := trackingEvent(row.ID, row.ReceiptID, row.LinkID, row.Stage, row.PageNo, row.Source, row.CreatedAt)
	var digest [32]byte
	copy(digest[:], row.PayloadDigest)
	return event, digest, err
}

func (repository *PostgresRepository) ListEvents(ctx context.Context, input radarport.EventListInput) ([]radarport.Event, int64, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return nil, 0, err
	}
	params := radardb.ListRadarLinkEventsParams{LinkID: int64(input.LinkID), Stage: nullableTextStage(input.Stage), StartAt: nullableTime(input.Start), EndAt: nullableTime(input.End), RowLimit: input.Limit, RowOffset: input.Offset}
	total, err := queries.CountRadarLinkEvents(ctx, radardb.CountRadarLinkEventsParams{LinkID: params.LinkID, Stage: params.Stage, StartAt: params.StartAt, EndAt: params.EndAt})
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListRadarLinkEvents(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	items := make([]radarport.Event, 0, len(rows))
	for _, row := range rows {
		event, convertErr := trackingEvent(row.ID, row.ReceiptID, row.LinkID, row.Stage, row.PageNo, row.Source, row.CreatedAt)
		if convertErr != nil {
			return nil, 0, convertErr
		}
		items = append(items, event)
	}
	return items, total, nil
}

func (repository *PostgresRepository) EventStats(ctx context.Context, linkID radarport.LinkID) (radarport.EventStatsRecord, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return radarport.EventStatsRecord{}, err
	}
	row, err := queries.GetRadarLinkEventStats(ctx, int64(linkID))
	if err != nil {
		return radarport.EventStatsRecord{}, err
	}
	return radarport.EventStatsRecord{
		TotalEvents: row.TotalEvents, TotalLandings: row.TotalLandings, Redirects: row.Redirects,
		ViewerOpens: row.ViewerOpens, ImageLoaded: row.ImageLoaded, PDFOpened: row.PdfOpened,
		TodayLandings: row.TodayLandings, LastClickedAt: nonzeroUnix(row.LastClickedEpoch),
		LastEventAt: nonzeroUnix(row.LastEventEpoch), LastViewedAt: nonzeroUnix(row.LastViewedEpoch),
	}, nil
}

func (repository *PostgresRepository) ListEnabledForSidebar(ctx context.Context, limit, offset int32) ([]radarport.SidebarLink, int64, error) {
	queries, err := repository.trackingQueries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountEnabledRadarLinksForSidebar(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListEnabledRadarLinksForSidebar(ctx, radardb.ListEnabledRadarLinksForSidebarParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	items := make([]radarport.SidebarLink, 0, len(rows))
	for _, row := range rows {
		if !row.UpdatedAt.Valid {
			return nil, 0, radarport.ErrUnavailable
		}
		items = append(items, radarport.SidebarLink{LinkID: radarport.LinkID(row.ID), Title: row.Title, TargetType: "link", TypeLabel: "链接", URL: row.PublicCode, UpdatedAt: row.UpdatedAt.Time})
	}
	return items, total, nil
}

func (repository *PostgresRepository) trackingQueries(ctx context.Context) (*radardb.Queries, error) {
	if repository == nil || repository.tx == nil {
		return nil, radarport.ErrUnavailable
	}
	tx, err := repository.tx(ctx)
	if err != nil {
		return nil, err
	}
	return radardb.New(tx), nil
}

func trackingLink(row radardb.RadarLink) (radarport.Link, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return radarport.Link{}, radarport.ErrUnavailable
	}
	return radarport.Link{LinkID: radarport.LinkID(row.ID), PublicCode: row.PublicCode, Name: row.Name, Title: row.Title, DestinationURL: row.DestinationUrl, CoverImageID: int8Pointer(row.CoverImageID), AttachmentID: int8Pointer(row.AttachmentID), Status: radarport.Status(row.Status), Version: row.Version, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func trackingEvent(id int64, receipt string, linkID int64, stage string, page pgtype.Int4, source string, created pgtype.Timestamptz) (radarport.Event, error) {
	if id < 1 || receipt == "" || linkID < 1 || !created.Valid {
		return radarport.Event{}, radarport.ErrUnavailable
	}
	var pagePointer *int32
	if page.Valid {
		value := page.Int32
		pagePointer = &value
	}
	return radarport.Event{EventID: id, ReceiptID: receipt, LinkID: radarport.LinkID(linkID), Stage: radarport.EventStage(stage), Page: pagePointer, Source: radarport.EventSource(source), CreatedAt: created.Time}, nil
}

func nullableInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func nullableTextStage(value *radarport.EventStage) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*value), Valid: true}
}

func nullableTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func nonzeroUnix(value int64) *time.Time {
	if value == 0 {
		return nil
	}
	result := time.Unix(value, 0).UTC()
	return &result
}

func int8Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

var _ radarport.TrackingRepository = (*PostgresRepository)(nil)
