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

type UploadRepository struct{}

var _ mediaapp.Store = (*UploadRepository)(nil)

func NewUploadRepository() *UploadRepository { return &UploadRepository{} }

func queries(ctx context.Context) (*mediadb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return mediadb.New(tx), nil
}

func (repository *UploadRepository) Reserve(ctx context.Context, reservation mediaapp.Reservation) (mediaapp.Receipt, bool, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaapp.Receipt{}, false, err
	}
	row, err := query.ReserveMediaImageUploadReceipt(ctx, mediadb.ReserveMediaImageUploadReceiptParams{
		ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: stamp(reservation.CreatedAt),
	})
	if err == nil {
		return receipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return mediaapp.Receipt{}, false, err
	}
	old, err := query.GetMediaImageUploadReceipt(ctx, mediadb.GetMediaImageUploadReceiptParams{ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return mediaapp.Receipt{}, false, err
	}
	return receipt(old.ID, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}

func (repository *UploadRepository) Create(ctx context.Context, input mediaapp.CreateInput) (mediaport.Image, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil {
		return mediaport.Image{}, err
	}
	command := input.Command
	row, err := query.InsertMediaImage(ctx, mediadb.InsertMediaImageParams{
		Name: command.Name, FileName: command.FileName, MimeType: input.MediaType, FileSize: int32(len(command.Content)),
		Width: input.Width, Height: input.Height, Checksum: input.Checksum[:], Description: command.Description,
		Tags: command.Tags, Category: command.Category, CreatedBy: command.Actor, CreatedAt: stamp(input.Now),
	})
	if err != nil {
		return mediaport.Image{}, err
	}
	if err = query.InsertMediaImageBlob(ctx, mediadb.InsertMediaImageBlobParams{ImageID: row.ID, Content: command.Content, Checksum: input.Checksum[:], CreatedAt: stamp(input.Now)}); err != nil {
		return mediaport.Image{}, err
	}
	return mediaport.Image{ID: row.ID, Name: row.Name, FileName: row.FileName, FileSize: row.FileSize, MimeType: row.MimeType,
		Width: row.Width, Height: row.Height, Description: row.Description, Tags: row.Tags, Category: row.Category,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func (repository *UploadRepository) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.Receipt, error) {
	query, err := queries(ctx)
	if repository == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return mediaapp.Receipt{}, err
	}
	row, err := query.CompleteMediaImageUploadReceipt(ctx, mediadb.CompleteMediaImageUploadReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if err != nil {
		return mediaapp.Receipt{}, err
	}
	return receipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func receipt(id int64, actor string, key, payload []byte, state string, snapshot []byte) mediaapp.Receipt {
	var value mediaapp.Receipt
	value.ID, value.ActorScope, value.State = id, actor, state
	copy(value.KeyDigest[:], key)
	copy(value.PayloadDigest[:], payload)
	value.ResultSnapshot = append([]byte(nil), snapshot...)
	return value
}
func stamp(value time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: value, Valid: true} }
