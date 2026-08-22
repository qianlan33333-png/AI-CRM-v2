package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type MiniProgramRepository struct{}

var _ mediaapp.MiniProgramStore = (*MiniProgramRepository)(nil)
var _ mediaport.ImageMetadataReader = (*MiniProgramRepository)(nil)
var _ mediaport.ChannelMiniProgramReferenceReader = (*MiniProgramRepository)(nil)
var _ mediaport.ThumbnailCacheResolver = (*MiniProgramRepository)(nil)

func NewMiniProgramRepository() *MiniProgramRepository { return &MiniProgramRepository{} }

func (repository *MiniProgramRepository) ListMiniPrograms(ctx context.Context, input mediaport.MiniProgramListQuery) ([]mediaport.MiniProgram, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return nil, miniProgramStoreUnavailable(err)
	}
	rows, err := query.ListMediaMiniPrograms(ctx, mediadb.ListMediaMiniProgramsParams{
		EnabledOnly: input.EnabledOnly, Search: input.Search, RowLimit: input.Limit, RowOffset: input.Offset,
	})
	if err != nil {
		return nil, miniProgramStoreUnavailable(err)
	}
	items := make([]mediaport.MiniProgram, len(rows))
	for index, row := range rows {
		items[index], err = mapMiniProgram(row.ID, row.Name, row.AppID, row.PagePath, row.Title, row.ThumbnailImageUrl,
			row.ThumbnailImageID, row.ThumbnailMediaID, row.ThumbnailMediaExpiresAt, row.Enabled, row.CreatedBy,
			row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (repository *MiniProgramRepository) CountMiniPrograms(ctx context.Context, input mediaport.MiniProgramListQuery) (int64, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return 0, miniProgramStoreUnavailable(err)
	}
	count, err := query.CountMediaMiniPrograms(ctx, mediadb.CountMediaMiniProgramsParams{EnabledOnly: input.EnabledOnly, Search: input.Search})
	if err != nil || count < 0 {
		return 0, miniProgramStoreUnavailable(err)
	}
	return count, nil
}

func (repository *MiniProgramRepository) GetMiniProgram(ctx context.Context, id int64) (mediaport.MiniProgram, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	row, err := query.GetMediaMiniProgram(ctx, id)
	if err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	return mapMiniProgram(row.ID, row.Name, row.AppID, row.PagePath, row.Title, row.ThumbnailImageUrl,
		row.ThumbnailImageID, row.ThumbnailMediaID, row.ThumbnailMediaExpiresAt, row.Enabled, row.CreatedBy,
		row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt)
}

func (repository *MiniProgramRepository) LockMiniProgram(ctx context.Context, id int64) (mediaport.MiniProgram, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	if _, err = query.LockMediaMiniProgram(ctx, id); err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	return repository.GetMiniProgram(ctx, id)
}

func (repository *MiniProgramRepository) ChannelMiniProgramEligible(ctx context.Context, id int64) (bool, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if repository == nil || id < 1 || err != nil {
		return false, miniProgramStoreUnavailable(err)
	}
	var lockedID int64
	err = tx.QueryRow(ctx, `SELECT id FROM media_miniprograms WHERE id = $1 AND enabled = TRUE FOR KEY SHARE`, id).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && lockedID == id, miniProgramStoreUnavailable(err)
}

func (repository *MiniProgramRepository) CreateMiniProgram(ctx context.Context, item mediaport.MiniProgram) (mediaport.MiniProgram, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	id, err := query.CreateMediaMiniProgram(ctx, miniProgramCreateParams(item))
	if err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	return repository.GetMiniProgram(ctx, id)
}

func (repository *MiniProgramRepository) UpdateMiniProgram(ctx context.Context, item mediaport.MiniProgram) (mediaport.MiniProgram, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	params := mediadb.UpdateMediaMiniProgramParams{
		Name: item.Name, AppID: item.AppID, PagePath: item.PagePath, Title: item.Title,
		ThumbnailImageUrl: item.ThumbnailImageURL, ThumbnailImageID: nullableMiniProgramID(item.ThumbnailImageID),
		ThumbnailMediaID: item.ThumbnailMediaID, ThumbnailMediaExpiresAt: nullableMiniProgramTime(item.ThumbnailMediaExpiresAt),
		Enabled: item.Enabled, UpdatedBy: item.UpdatedBy, Version: item.Version, UpdatedAt: stamp(item.UpdatedAt), ID: item.ID,
	}
	if err = query.UpdateMediaMiniProgram(ctx, params); err != nil {
		return mediaport.MiniProgram{}, miniProgramStoreUnavailable(err)
	}
	return repository.GetMiniProgram(ctx, item.ID)
}

func (repository *MiniProgramRepository) DeleteMiniProgram(ctx context.Context, id int64) error {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return miniProgramStoreUnavailable(err)
	}
	if err = query.DeleteMediaMiniProgram(ctx, id); err != nil {
		return miniProgramStoreUnavailable(err)
	}
	return nil
}

func (repository *MiniProgramRepository) ImageExists(ctx context.Context, id int64) (bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 {
		return false, miniProgramStoreUnavailable(err)
	}
	_, err = query.LockMediaImageReference(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, miniProgramStoreUnavailable(err)
	}
	return true, nil
}

func (repository *MiniProgramRepository) ResolveThumbnailFromCache(ctx context.Context, item mediaport.MiniProgram) (mediaport.ThumbnailCacheResolution, error) {
	if repository == nil {
		return mediaport.ThumbnailCacheResolution{}, mediaapp.ErrMiniProgramUnavailable
	}
	if item.ThumbnailImageID == nil || *item.ThumbnailImageID < 1 {
		return mediaport.ThumbnailCacheResolution{Status: mediaport.ThumbnailNotAvailable, CacheOwner: mediaport.ThumbnailCacheOwner,
			CacheReceipt: "media.thumbnail-cache:no-image", SideEffectExecuted: false, RealExternalCallExecuted: false}, nil
	}
	query, err := queries(ctx)
	if err != nil {
		return mediaport.ThumbnailCacheResolution{}, miniProgramStoreUnavailable(err)
	}
	row, err := query.GetMediaThumbnailCache(ctx, *item.ThumbnailImageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaport.ThumbnailCacheResolution{Status: mediaport.ThumbnailNotAvailable, CacheOwner: mediaport.ThumbnailCacheOwner,
			CacheReceipt: "media.thumbnail-cache:miss:" + strconv.FormatInt(*item.ThumbnailImageID, 10), SideEffectExecuted: false, RealExternalCallExecuted: false}, nil
	}
	if err != nil {
		return mediaport.ThumbnailCacheResolution{}, miniProgramStoreUnavailable(err)
	}
	resolution := mediaport.ThumbnailCacheResolution{CacheOwner: mediaport.ThumbnailCacheOwner, CacheReceipt: row.CacheReceipt,
		MediaID: row.MediaID, SideEffectExecuted: false, RealExternalCallExecuted: false}
	switch row.State {
	case string(mediaport.ThumbnailResolved):
		resolution.Status = mediaport.ThumbnailResolved
		if row.ExpiresAt.Valid {
			value := row.ExpiresAt.Time.UTC()
			resolution.ExpiresAt = &value
		}
	case string(mediaport.ThumbnailOutcomeUnknown):
		resolution.Status = mediaport.ThumbnailOutcomeUnknown
	default:
		return mediaport.ThumbnailCacheResolution{}, mediaapp.ErrMiniProgramUnavailable
	}
	return resolution, nil
}

func (repository *MiniProgramRepository) ReserveMiniProgram(ctx context.Context, input mediaapp.MiniProgramReservation) (mediaapp.MiniProgramReceipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaapp.MiniProgramReceipt{}, false, miniProgramStoreUnavailable(err)
	}
	params := mediadb.ReserveMediaMiniProgramReceiptParams{Operation: input.Operation, ActorScope: input.ActorScope,
		BusinessKey: input.BusinessKey, KeyDigest: input.KeyDigest[:], PayloadDigest: input.PayloadDigest[:], CreatedAt: stamp(input.CreatedAt)}
	row, err := query.ReserveMediaMiniProgramReceipt(ctx, params)
	if err == nil {
		return mapMiniProgramReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.MiniProgramReceipt{}, false, miniProgramStoreUnavailable(err)
	}
	old, err := query.GetMediaMiniProgramReceipt(ctx, mediadb.GetMediaMiniProgramReceiptParams{Operation: input.Operation,
		ActorScope: input.ActorScope, BusinessKey: input.BusinessKey, KeyDigest: input.KeyDigest[:]})
	if err != nil {
		return mediaapp.MiniProgramReceipt{}, false, miniProgramStoreUnavailable(err)
	}
	return mapMiniProgramReceipt(old.ID, old.Operation, old.ActorScope, old.BusinessKey, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (repository *MiniProgramRepository) CompleteMiniProgram(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.MiniProgramReceipt, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return mediaapp.MiniProgramReceipt{}, miniProgramStoreUnavailable(err)
	}
	row, err := query.CompleteMediaMiniProgramReceipt(ctx, mediadb.CompleteMediaMiniProgramReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if err != nil {
		return mediaapp.MiniProgramReceipt{}, miniProgramStoreUnavailable(err)
	}
	return mapMiniProgramReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func miniProgramCreateParams(item mediaport.MiniProgram) mediadb.CreateMediaMiniProgramParams {
	return mediadb.CreateMediaMiniProgramParams{Name: item.Name, AppID: item.AppID, PagePath: item.PagePath, Title: item.Title,
		ThumbnailImageUrl: item.ThumbnailImageURL, ThumbnailImageID: nullableMiniProgramID(item.ThumbnailImageID),
		ThumbnailMediaID: item.ThumbnailMediaID, ThumbnailMediaExpiresAt: nullableMiniProgramTime(item.ThumbnailMediaExpiresAt),
		Enabled: item.Enabled, CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy, Version: item.Version,
		CreatedAt: stamp(item.CreatedAt), UpdatedAt: stamp(item.UpdatedAt)}
}

func mapMiniProgram(id int64, name, appID, pagePath, title, thumbnailURL string, thumbnailID pgtype.Int8,
	thumbnailMediaID string, thumbnailExpires pgtype.Timestamptz, enabled bool, createdBy, updatedBy, version int64,
	createdAt, updatedAt pgtype.Timestamptz) (mediaport.MiniProgram, error) {
	if !createdAt.Valid || !updatedAt.Valid {
		return mediaport.MiniProgram{}, mediaapp.ErrMiniProgramUnavailable
	}
	item := mediaport.MiniProgram{ID: id, Name: name, AppID: appID, PagePath: pagePath, Title: title,
		ThumbnailImageURL: thumbnailURL, ThumbnailMediaID: thumbnailMediaID, Enabled: enabled, CreatedBy: createdBy,
		UpdatedBy: updatedBy, Version: version, CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC()}
	if thumbnailID.Valid {
		value := thumbnailID.Int64
		item.ThumbnailImageID = &value
	}
	if thumbnailExpires.Valid {
		value := thumbnailExpires.Time.UTC()
		item.ThumbnailMediaExpiresAt = &value
	}
	return item, nil
}

func mapMiniProgramReceipt(id int64, operation, actor, business string, key, payload []byte, state string, snapshot []byte) mediaapp.MiniProgramReceipt {
	result := mediaapp.MiniProgramReceipt{ID: id, Operation: operation, ActorScope: actor, BusinessKey: business, State: state,
		ResultSnapshot: append(json.RawMessage{}, snapshot...)}
	copy(result.KeyDigest[:], key)
	copy(result.PayloadDigest[:], payload)
	return result
}

func nullableMiniProgramID(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

func nullableMiniProgramTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return stamp(*value)
}

func miniProgramStoreUnavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ErrMiniProgramNotFound
	}
	if err != nil {
		return err
	}
	return mediaapp.ErrMiniProgramUnavailable
}
