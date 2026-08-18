package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
)

var _ mediaapp.ImageDeleteStore = (*UploadRepository)(nil)

func (repository *UploadRepository) LockImageForDelete(ctx context.Context, imageID int64) (bool, error) {
	query, err := queries(ctx)
	if repository == nil || imageID < 1 {
		return false, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return false, err
	}
	_, err = query.LockMediaImageForDelete(ctx, imageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (repository *UploadRepository) ListImageDeleteMediaReferences(ctx context.Context, imageID int64) (mediaapp.ImageDeleteReferences, error) {
	query, err := queries(ctx)
	if repository == nil || imageID < 1 {
		return mediaapp.ImageDeleteReferences{}, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return mediaapp.ImageDeleteReferences{}, err
	}
	miniprograms, err := query.ListMediaImageDeleteMiniprogramReferences(ctx, imageID)
	if err != nil {
		return mediaapp.ImageDeleteReferences{}, err
	}
	groupInvites, err := query.ListMediaImageDeleteGroupInviteReferences(ctx, imageID)
	if err != nil {
		return mediaapp.ImageDeleteReferences{}, err
	}
	preflights, err := query.ListMediaImageDeleteImportPreflightReferences(ctx, imageID)
	if err != nil {
		return mediaapp.ImageDeleteReferences{}, err
	}
	return mediaapp.ImageDeleteReferences{
		Miniprograms: append([]int64{}, miniprograms...), CampaignSteps: []int64{}, GroupInvites: append([]int64{}, groupInvites...),
		AutomationAgents: []int64{}, Channels: []int64{}, ImportPreflights: append([]int64{}, preflights...),
	}, nil
}

func (repository *UploadRepository) GetImageDeleteReceipt(ctx context.Context, reservation mediaapp.ImageDeleteReservation) (mediaapp.ImageDeleteReceipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil {
		return mediaapp.ImageDeleteReceipt{}, false, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, false, err
	}
	row, err := query.GetMediaImageDeleteReceipt(ctx, mediadb.GetMediaImageDeleteReceiptParams{ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageDeleteReceipt{}, false, nil
	}
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, false, err
	}
	return imageDeleteReceipt(row.ID, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
}

func (repository *UploadRepository) ReserveImageDelete(ctx context.Context, reservation mediaapp.ImageDeleteReservation) (mediaapp.ImageDeleteReceipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil {
		return mediaapp.ImageDeleteReceipt{}, false, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, false, err
	}
	row, err := query.ReserveMediaImageDeleteReceipt(ctx, mediadb.ReserveMediaImageDeleteReceiptParams{
		ActorScope: reservation.ActorScope, BusinessKey: strconv.FormatInt(reservation.BusinessKey, 10), KeyDigest: reservation.KeyDigest[:],
		PayloadDigest: reservation.PayloadDigest[:], CreatedAt: stamp(reservation.CreatedAt),
	})
	if err == nil {
		return imageDeleteReceipt(row.ID, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.ImageDeleteReceipt{}, false, err
	}
	old, err := query.GetMediaImageDeleteReceipt(ctx, mediadb.GetMediaImageDeleteReceiptParams{ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, false, err
	}
	return imageDeleteReceipt(old.ID, old.ActorScope, old.BusinessKey, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (repository *UploadRepository) DeleteImage(ctx context.Context, imageID int64) (int64, error) {
	query, err := queries(ctx)
	if repository == nil || imageID < 1 {
		return 0, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return 0, err
	}
	return query.DeleteMediaImage(ctx, imageID)
}

func (repository *UploadRepository) CompleteImageDelete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.ImageDeleteReceipt, error) {
	query, err := queries(ctx)
	if repository == nil || id < 1 || !json.Valid(snapshot) {
		return mediaapp.ImageDeleteReceipt{}, mediaapp.ErrImageDeleteUnavailable
	}
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, err
	}
	row, err := query.CompleteMediaImageDeleteReceipt(ctx, mediadb.CompleteMediaImageDeleteReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if err != nil {
		return mediaapp.ImageDeleteReceipt{}, err
	}
	return imageDeleteReceipt(row.ID, row.ActorScope, row.BusinessKey, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func imageDeleteReceipt(id int64, actorScope, businessKey string, keyDigest, payloadDigest []byte, state string, snapshot []byte) mediaapp.ImageDeleteReceipt {
	value := mediaapp.ImageDeleteReceipt{ID: id, ActorScope: actorScope, State: state, ResultSnapshot: append(json.RawMessage{}, snapshot...)}
	parsed, err := strconv.ParseInt(businessKey, 10, 64)
	if err == nil {
		value.BusinessKey = parsed
	}
	copy(value.KeyDigest[:], keyDigest)
	copy(value.PayloadDigest[:], payloadDigest)
	return value
}
