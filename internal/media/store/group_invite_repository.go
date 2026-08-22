package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type GroupInviteRepository struct{}

var _ mediaapp.GroupInviteStore = (*GroupInviteRepository)(nil)
var _ mediaport.ImageMetadataReader = (*GroupInviteRepository)(nil)
var _ mediaport.ChannelGroupInviteReferenceReader = (*GroupInviteRepository)(nil)

func NewGroupInviteRepository() *GroupInviteRepository { return &GroupInviteRepository{} }

func (repository *GroupInviteRepository) ListGroupInvites(ctx context.Context, input mediaport.GroupInviteListQuery) ([]mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return nil, groupInviteUnavailable(err)
	}
	rows, err := query.ListMediaGroupInvites(ctx, mediadb.ListMediaGroupInvitesParams{EnabledOnly: input.EnabledOnly, Search: input.Search, RowLimit: input.Limit, RowOffset: input.Offset})
	if err != nil {
		return nil, groupInviteUnavailable(err)
	}
	items := make([]mediaport.GroupInvite, len(rows))
	for index, row := range rows {
		items[index], err = mapGroupInvite(row.ID, row.Name, row.Title, row.Description, row.JoinUrl, row.CoverImageID, row.Enabled,
			row.CreatedBy, row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt, row.ArchivedAt)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (repository *GroupInviteRepository) CountGroupInvites(ctx context.Context, input mediaport.GroupInviteListQuery) (int64, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return 0, groupInviteUnavailable(err)
	}
	count, err := query.CountMediaGroupInvites(ctx, mediadb.CountMediaGroupInvitesParams{EnabledOnly: input.EnabledOnly, Search: input.Search})
	if err != nil || count < 0 {
		return 0, groupInviteUnavailable(err)
	}
	return count, nil
}

func (repository *GroupInviteRepository) GetGroupInvite(ctx context.Context, id int64) (mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	row, err := query.GetMediaGroupInvite(ctx, id)
	if err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	return mapGroupInvite(row.ID, row.Name, row.Title, row.Description, row.JoinUrl, row.CoverImageID, row.Enabled,
		row.CreatedBy, row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt, row.ArchivedAt)
}

func (repository *GroupInviteRepository) LockGroupInvite(ctx context.Context, id int64) (mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	if _, err = query.LockMediaGroupInvite(ctx, id); err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	return repository.GetGroupInvite(ctx, id)
}

func (repository *GroupInviteRepository) ChannelGroupInviteEligible(ctx context.Context, id int64) (bool, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if repository == nil || id < 1 || err != nil {
		return false, groupInviteUnavailable(err)
	}
	var lockedID int64
	err = tx.QueryRow(ctx, `SELECT id FROM media_group_invites WHERE id = $1 AND enabled = TRUE AND archived_at IS NULL FOR KEY SHARE`, id).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && lockedID == id, groupInviteUnavailable(err)
}

func (repository *GroupInviteRepository) CreateGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	id, err := query.CreateMediaGroupInvite(ctx, groupInviteCreateParams(item))
	if err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	return repository.GetGroupInvite(ctx, id)
}

func (repository *GroupInviteRepository) UpdateGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	params := mediadb.UpdateMediaGroupInviteParams{ID: item.ID, Name: item.Name, Title: item.Title, Description: item.Description,
		JoinUrl: item.JoinURL, CoverImageID: nullableID(item.CoverImageID), Enabled: item.Enabled, UpdatedBy: item.UpdatedBy,
		Version: item.Version, UpdatedAt: stamp(item.UpdatedAt)}
	if err = query.UpdateMediaGroupInvite(ctx, params); err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	return repository.GetGroupInvite(ctx, item.ID)
}

func (repository *GroupInviteRepository) ArchiveGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || item.ArchivedAt == nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	if err = query.ArchiveMediaGroupInvite(ctx, mediadb.ArchiveMediaGroupInviteParams{UpdatedBy: item.UpdatedBy, Version: item.Version,
		UpdatedAt: stamp(item.UpdatedAt), ArchivedAt: stamp(*item.ArchivedAt), ID: item.ID}); err != nil {
		return mediaport.GroupInvite{}, groupInviteUnavailable(err)
	}
	return item, nil
}

func (repository *GroupInviteRepository) ImageExists(ctx context.Context, id int64) (bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 {
		return false, groupInviteUnavailable(err)
	}
	_, err = query.LockMediaImageReference(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, groupInviteUnavailable(err)
	}
	return true, nil
}

func (repository *GroupInviteRepository) ReserveGroupInvite(ctx context.Context, input mediaapp.GroupInviteReservation) (mediaapp.GroupInviteReceipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaapp.GroupInviteReceipt{}, false, groupInviteUnavailable(err)
	}
	params := mediadb.ReserveMediaGroupInviteReceiptParams{Operation: input.Operation, ActorScope: input.ActorScope, BusinessKey: input.BusinessKey,
		KeyDigest: input.KeyDigest[:], PayloadDigest: input.PayloadDigest[:], CreatedAt: stamp(input.CreatedAt)}
	row, err := query.ReserveMediaGroupInviteReceipt(ctx, params)
	if err == nil {
		return mapGroupInviteReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.GroupInviteReceipt{}, false, groupInviteUnavailable(err)
	}
	old, err := query.GetMediaGroupInviteReceipt(ctx, mediadb.GetMediaGroupInviteReceiptParams{Operation: input.Operation, ActorScope: input.ActorScope,
		BusinessKey: input.BusinessKey, KeyDigest: input.KeyDigest[:]})
	if err != nil {
		return mediaapp.GroupInviteReceipt{}, false, groupInviteUnavailable(err)
	}
	return mapGroupInviteReceipt(old.ID, old.Operation, old.ActorScope, old.BusinessKey, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (repository *GroupInviteRepository) CompleteGroupInvite(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.GroupInviteReceipt, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return mediaapp.GroupInviteReceipt{}, groupInviteUnavailable(err)
	}
	row, err := query.CompleteMediaGroupInviteReceipt(ctx, mediadb.CompleteMediaGroupInviteReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if err != nil {
		return mediaapp.GroupInviteReceipt{}, groupInviteUnavailable(err)
	}
	return mapGroupInviteReceipt(row.ID, row.Operation, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func groupInviteCreateParams(item mediaport.GroupInvite) mediadb.CreateMediaGroupInviteParams {
	return mediadb.CreateMediaGroupInviteParams{Name: item.Name, Title: item.Title, Description: item.Description, JoinUrl: item.JoinURL,
		CoverImageID: nullableID(item.CoverImageID), Enabled: item.Enabled, CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy,
		Version: item.Version, CreatedAt: stamp(item.CreatedAt), UpdatedAt: stamp(item.UpdatedAt)}
}

func mapGroupInvite(id int64, name, title, description, joinURL string, cover pgtype.Int8, enabled bool, createdBy, updatedBy, version int64,
	createdAt, updatedAt, archivedAt pgtype.Timestamptz) (mediaport.GroupInvite, error) {
	if !createdAt.Valid || !updatedAt.Valid {
		return mediaport.GroupInvite{}, mediaapp.ErrGroupInviteUnavailable
	}
	item := mediaport.GroupInvite{ID: id, Name: name, Title: title, Description: description, JoinURL: joinURL, Enabled: enabled,
		CreatedBy: createdBy, UpdatedBy: updatedBy, Version: version, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
	if cover.Valid {
		item.CoverImageID = cover.Int64
	}
	if archivedAt.Valid {
		value := archivedAt.Time
		item.ArchivedAt = &value
	}
	return item, nil
}

func mapGroupInviteReceipt(id int64, operation, actor, business string, key, payload []byte, state string, snapshot []byte) mediaapp.GroupInviteReceipt {
	result := mediaapp.GroupInviteReceipt{ID: id, Operation: operation, ActorScope: actor, BusinessKey: business, State: state,
		ResultSnapshot: append(json.RawMessage{}, snapshot...)}
	copy(result.KeyDigest[:], key)
	copy(result.PayloadDigest[:], payload)
	return result
}

func nullableID(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }

func groupInviteUnavailable(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ErrGroupInviteNotFound
	}
	if err != nil {
		return err
	}
	return mediaapp.ErrGroupInviteUnavailable
}
