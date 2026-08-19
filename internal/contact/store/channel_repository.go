package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ChannelRepository struct{}

var _ contactapp.ChannelStore = (*ChannelRepository)(nil)
var _ contactport.ImageReferenceReader = (*ChannelRepository)(nil)

func NewChannelRepository() *ChannelRepository { return &ChannelRepository{} }

func channelQueries(ctx context.Context) (*contactdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return contactdb.New(tx), nil
}

func (repository *ChannelRepository) ListChannels(ctx context.Context, limit int32, status string, includeArchived bool) ([]contactapp.Channel, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil {
		return nil, channelError(err)
	}
	rows, err := queries.ListChannels(ctx, contactdb.ListChannelsParams{FilterStatus: status, IncludeArchived: includeArchived, RowLimit: limit})
	if err != nil {
		return nil, channelError(err)
	}
	result := make([]contactapp.Channel, len(rows))
	for i, row := range rows {
		result[i] = channel(row.ID, row.ChannelCode, row.ChannelName, row.Status, row.LegacyProjection, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt)
	}
	return result, nil
}
func (repository *ChannelRepository) GetChannel(ctx context.Context, id int64) (contactapp.Channel, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	row, err := queries.GetChannel(ctx, id)
	if err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	return channel(row.ID, row.ChannelCode, row.ChannelName, row.Status, row.LegacyProjection, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt), nil
}
func (repository *ChannelRepository) ListImageReferenceChannelIDs(ctx context.Context, imageID int64) ([]int64, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil || imageID < 1 {
		return nil, channelError(err)
	}
	rows, err := queries.ListChannelImageReferencePackages(ctx)
	if err != nil {
		return nil, channelError(err)
	}
	result := make([]int64, 0, len(rows))
	var previousID int64
	for _, row := range rows {
		if row.ID < 1 || row.ID <= previousID {
			return nil, contactapp.ErrChannelUnavailable
		}
		previousID = row.ID
		ids, parseErr := channelImageReferenceIDs(json.RawMessage(row.WelcomeImageLibraryIds))
		if parseErr != nil {
			return nil, contactapp.ErrChannelUnavailable
		}
		for _, candidate := range ids {
			if candidate == imageID {
				result = append(result, row.ID)
				break
			}
		}
	}
	return result, nil
}

func channelImageReferenceIDs(raw json.RawMessage) ([]int64, error) {
	if len(raw) == 0 {
		return []int64{}, nil
	}
	if raw[0] != '[' {
		return nil, contactapp.ErrChannelUnavailable
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, contactapp.ErrChannelUnavailable
	}
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		id, err := channelCanonicalImageReferenceID(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, contactapp.ErrChannelUnavailable
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func channelCanonicalImageReferenceID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || raw[0] < '1' || raw[0] > '9' {
		return 0, contactapp.ErrChannelUnavailable
	}
	for _, character := range raw[1:] {
		if character < '0' || character > '9' {
			return 0, contactapp.ErrChannelUnavailable
		}
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, contactapp.ErrChannelUnavailable
	}
	return id, nil
}
func (repository *ChannelRepository) CreateChannel(ctx context.Context, command contactapp.CreateChannelCommand, now time.Time) (contactapp.Channel, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	row, err := queries.CreateChannel(ctx, contactdb.CreateChannelParams{ChannelCode: command.ChannelCode, ChannelName: command.ChannelName, Status: command.Status, LegacyProjection: command.LegacyProjection, Actor: command.Actor, ChangedAt: stampChannel(now)})
	if err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	return channel(row.ID, row.ChannelCode, row.ChannelName, row.Status, row.LegacyProjection, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt), nil
}
func (repository *ChannelRepository) UpdateChannel(ctx context.Context, current contactapp.Channel, actor int64, now time.Time) (contactapp.Channel, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	row, err := queries.UpdateChannel(ctx, contactdb.UpdateChannelParams{ChannelName: current.ChannelName, Status: current.Status, LegacyProjection: current.LegacyProjection, Actor: actor, ChangedAt: stampChannel(now), ChannelID: current.ID})
	if err != nil {
		return contactapp.Channel{}, channelError(err)
	}
	return channel(row.ID, row.ChannelCode, row.ChannelName, row.Status, row.LegacyProjection, row.CreatedBy, row.UpdatedBy, row.CreatedAt, row.UpdatedAt), nil
}
func (repository *ChannelRepository) ReserveChannel(ctx context.Context, reservation contactapp.ChannelReservation) (contactapp.ChannelReceipt, bool, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil {
		return contactapp.ChannelReceipt{}, false, channelError(err)
	}
	params := contactdb.ReserveChannelOperationReceiptParams{Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: stampChannel(reservation.CreatedAt)}
	row, err := queries.ReserveChannelOperationReceipt(ctx, params)
	if err == nil {
		return channelReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ChannelReceipt{}, false, channelError(err)
	}
	old, err := queries.GetChannelOperationReceipt(ctx, contactdb.GetChannelOperationReceiptParams{Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if err != nil {
		return contactapp.ChannelReceipt{}, false, channelError(err)
	}
	return channelReceipt(old.ID, old.Operation, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}
func (repository *ChannelRepository) CompleteChannel(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (contactapp.ChannelReceipt, error) {
	queries, err := channelQueries(ctx)
	if repository == nil || err != nil || id < 1 || !json.Valid(snapshot) {
		return contactapp.ChannelReceipt{}, channelError(err)
	}
	row, err := queries.CompleteChannelOperationReceipt(ctx, contactdb.CompleteChannelOperationReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stampChannel(now)})
	if err != nil {
		return contactapp.ChannelReceipt{}, channelError(err)
	}
	return channelReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}
func channel(id int64, code, name, status string, projection []byte, createdBy, updatedBy int64, createdAt, updatedAt pgtype.Timestamptz) contactapp.Channel {
	return contactapp.Channel{ID: id, ChannelCode: code, ChannelName: name, Status: status, LegacyProjection: append([]byte(nil), projection...), CreatedBy: createdBy, UpdatedBy: updatedBy, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
}
func channelReceipt(id int64, operation, actor string, key, payload []byte, state string, snapshot []byte) contactapp.ChannelReceipt {
	var r contactapp.ChannelReceipt
	r.ID = id
	r.Operation = operation
	r.ActorScope = actor
	r.State = state
	r.ResultSnapshot = append([]byte(nil), snapshot...)
	copy(r.KeyDigest[:], key)
	copy(r.PayloadDigest[:], payload)
	return r
}
func stampChannel(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func channelError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.ErrChannelNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return contactapp.ErrChannelConflict
		}
		return err
	}
	return contactapp.ErrChannelUnavailable
}
