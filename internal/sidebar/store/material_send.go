package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebardb "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/store/generated"
)

type MaterialSendReceiptStore struct{ pool *pgxpool.Pool }

func NewMaterialSendReceiptStore(pool *pgxpool.Pool) (*MaterialSendReceiptStore, error) {
	if pool == nil {
		return nil, sidebarapp.ErrUnavailable
	}
	return &MaterialSendReceiptStore{pool: pool}, nil
}

func (store *MaterialSendReceiptStore) ReserveSidebarImageSend(ctx context.Context, actorID, customerID, imageID int64, keyDigest [32]byte) (sidebarapp.SidebarImageSendReceipt, bool, error) {
	if store == nil || store.pool == nil || ctx == nil || actorID < 1 || customerID < 1 || imageID < 1 {
		return sidebarapp.SidebarImageSendReceipt{}, false, sidebarapp.ErrUnavailable
	}
	queries := sidebardb.New(store.pool)
	row, err := queries.ReserveSidebarImageSend(ctx, sidebardb.ReserveSidebarImageSendParams{
		ActorID: actorID, CustomerID: customerID, ImageID: imageID, KeyDigest: keyDigest[:],
	})
	if err == nil {
		return mapMaterialSendReceipt(row.ID, row.ImageID, row.State, row.MediaID, row.MediaExpiresAt, row.ProviderCallDispatched), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sidebarapp.SidebarImageSendReceipt{}, false, err
	}
	existing, err := queries.GetSidebarImageSendByKey(ctx, sidebardb.GetSidebarImageSendByKeyParams{
		ActorID: actorID, CustomerID: customerID, KeyDigest: keyDigest[:],
	})
	if err != nil {
		return sidebarapp.SidebarImageSendReceipt{}, false, err
	}
	return mapMaterialSendReceipt(existing.ID, existing.ImageID, existing.State, existing.MediaID, existing.MediaExpiresAt, existing.ProviderCallDispatched), false, nil
}

func (store *MaterialSendReceiptStore) CompleteSidebarImageSend(ctx context.Context, receiptID int64, state, mediaID string, mediaExpiresAt time.Time, providerCallDispatched bool) (sidebarapp.SidebarImageSendReceipt, error) {
	if store == nil || store.pool == nil || ctx == nil || receiptID < 1 || (state != "ready" && state != "outcome_unknown" && state != "final_failed") {
		return sidebarapp.SidebarImageSendReceipt{}, sidebarapp.ErrUnavailable
	}
	row, err := sidebardb.New(store.pool).CompleteSidebarImageSend(ctx, sidebardb.CompleteSidebarImageSendParams{
		ID: receiptID, State: state, MediaID: mediaID, MediaExpiresAt: nullableTime(mediaExpiresAt),
		ProviderCallDispatched: providerCallDispatched,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sidebarapp.SidebarImageSendReceipt{}, sidebarapp.ErrUnavailable
	}
	if err != nil {
		return sidebarapp.SidebarImageSendReceipt{}, err
	}
	return mapMaterialSendReceipt(row.ID, row.ImageID, row.State, row.MediaID, row.MediaExpiresAt, row.ProviderCallDispatched), nil
}

func mapMaterialSendReceipt(id, imageID int64, state string, mediaID pgtype.Text, mediaExpiresAt pgtype.Timestamptz, dispatched bool) sidebarapp.SidebarImageSendReceipt {
	receipt := sidebarapp.SidebarImageSendReceipt{ID: id, ImageID: imageID, State: state, ProviderCallDispatched: dispatched}
	if mediaID.Valid {
		receipt.MediaID = mediaID.String
	}
	if mediaExpiresAt.Valid {
		value := mediaExpiresAt.Time.UTC()
		receipt.MediaExpiresAt = value
	}
	return receipt
}

func nullableTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
