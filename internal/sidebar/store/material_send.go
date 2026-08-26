package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
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
	const insert = `INSERT INTO public.sidebar_image_temporary_media_receipts(actor_id, customer_id, image_id, key_digest, state)
VALUES ($1, $2, $3, $4, 'pending')
ON CONFLICT (actor_id, customer_id, key_digest) DO NOTHING
RETURNING id, image_id, state, media_id, media_expires_at, provider_call_dispatched`
	receipt, err := scanMaterialSendReceipt(store.pool.QueryRow(ctx, insert, actorID, customerID, imageID, keyDigest[:]))
	if err == nil {
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sidebarapp.SidebarImageSendReceipt{}, false, err
	}
	const read = `SELECT id, image_id, state, media_id, media_expires_at, provider_call_dispatched
FROM public.sidebar_image_temporary_media_receipts
WHERE actor_id = $1 AND customer_id = $2 AND key_digest = $3`
	receipt, err = scanMaterialSendReceipt(store.pool.QueryRow(ctx, read, actorID, customerID, keyDigest[:]))
	if err != nil {
		return sidebarapp.SidebarImageSendReceipt{}, false, err
	}
	return receipt, false, nil
}

func (store *MaterialSendReceiptStore) CompleteSidebarImageSend(ctx context.Context, receiptID int64, state, mediaID string, mediaExpiresAt time.Time, providerCallDispatched bool) (sidebarapp.SidebarImageSendReceipt, error) {
	if store == nil || store.pool == nil || ctx == nil || receiptID < 1 || (state != "ready" && state != "outcome_unknown" && state != "final_failed") {
		return sidebarapp.SidebarImageSendReceipt{}, sidebarapp.ErrUnavailable
	}
	const complete = `UPDATE public.sidebar_image_temporary_media_receipts
SET state = $2, media_id = NULLIF($3, ''), media_expires_at = $4, provider_call_dispatched = $5, updated_at = now()
WHERE id = $1 AND state = 'pending'
RETURNING id, image_id, state, media_id, media_expires_at, provider_call_dispatched`
	receipt, err := scanMaterialSendReceipt(store.pool.QueryRow(ctx, complete, receiptID, state, mediaID, nullableTime(mediaExpiresAt), providerCallDispatched))
	if errors.Is(err, pgx.ErrNoRows) {
		return sidebarapp.SidebarImageSendReceipt{}, sidebarapp.ErrUnavailable
	}
	return receipt, err
}

type materialSendRow interface{ Scan(...any) error }

func scanMaterialSendReceipt(row materialSendRow) (sidebarapp.SidebarImageSendReceipt, error) {
	var receipt sidebarapp.SidebarImageSendReceipt
	var mediaID *string
	var mediaExpiresAt *time.Time
	if err := row.Scan(&receipt.ID, &receipt.ImageID, &receipt.State, &mediaID, &mediaExpiresAt, &receipt.ProviderCallDispatched); err != nil {
		return sidebarapp.SidebarImageSendReceipt{}, err
	}
	if mediaID != nil {
		receipt.MediaID = *mediaID
	}
	if mediaExpiresAt != nil {
		receipt.MediaExpiresAt = mediaExpiresAt.UTC()
	}
	return receipt, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
