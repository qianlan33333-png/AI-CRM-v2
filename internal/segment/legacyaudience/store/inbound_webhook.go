package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	legacyaudience "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

const sha256Size = 32

type CreateInboundWebhookReceiptParams struct {
	PackageID             int64
	ExternalEventIDDigest []byte
	PayloadDigest         []byte
	MemberEventID         *int64
	CallbackStatus        string
	MessageJSON           json.RawMessage
	ActionJSON            json.RawMessage
	CreatedAt             time.Time
}

type InboundWebhookReceiptRecord struct {
	ID                    int64
	PackageID             int64
	State                 string
	ExternalEventIDDigest []byte
	PayloadDigest         []byte
	CreatedAt             time.Time
}

type CreateInboundWebhookTransportReplayParams struct {
	ClientID      string
	EventIDDigest []byte
	ReceiptID     int64
	PayloadDigest []byte
	CreatedAt     time.Time
}

type InboundWebhookTransportReplayRecord struct {
	ReceiptID     int64
	PayloadDigest []byte
}

type InboundWebhookQueries interface {
	AIAudienceInboundPackageExists(context.Context, int64) (bool, error)
	CreateAIAudienceInboundWebhookReceipt(context.Context, CreateInboundWebhookReceiptParams) (InboundWebhookReceiptRecord, error)
	GetAIAudienceInboundWebhookReceipt(context.Context, int64, []byte) (InboundWebhookReceiptRecord, error)
	CreateAIAudienceInboundWebhookTransportReplay(context.Context, CreateInboundWebhookTransportReplayParams) (InboundWebhookTransportReplayRecord, error)
	GetAIAudienceInboundWebhookTransportReplay(context.Context, string, []byte) (InboundWebhookTransportReplayRecord, error)
}

type InboundWebhookQueryFactory func(pgx.Tx) InboundWebhookQueries

type InboundWebhookRepository struct {
	queries func(context.Context) (InboundWebhookQueries, error)
}

var _ legacyaudience.InboundWebhookRepository = (*InboundWebhookRepository)(nil)

func NewInboundWebhookRepository(factory InboundWebhookQueryFactory) (*InboundWebhookRepository, error) {
	if factory == nil {
		return nil, legacyaudience.ErrUnavailable
	}
	return &InboundWebhookRepository{queries: func(ctx context.Context) (InboundWebhookQueries, error) {
		tx, err := platformstore.TxFromContext(ctx)
		if err != nil {
			return nil, err
		}
		return factory(tx), nil
	}}, nil
}

func (repository *InboundWebhookRepository) PackageExistsForInbound(ctx context.Context, packageID int64) (bool, error) {
	queries, err := repository.querySet(ctx)
	if err != nil {
		return false, err
	}
	exists, err := queries.AIAudienceInboundPackageExists(ctx, packageID)
	if err != nil {
		return false, unavailable(err)
	}
	return exists, nil
}

func (repository *InboundWebhookRepository) ReserveInboundWebhook(ctx context.Context, reservation legacyaudience.InboundWebhookReservation) (legacyaudience.InboundWebhookReceipt, bool, error) {
	queries, err := repository.querySet(ctx)
	if err != nil {
		return legacyaudience.InboundWebhookReceipt{}, false, err
	}
	record, err := queries.CreateAIAudienceInboundWebhookReceipt(ctx, CreateInboundWebhookReceiptParams{
		PackageID: reservation.PackageID, ExternalEventIDDigest: reservation.ExternalEventIDDigest[:], PayloadDigest: reservation.PayloadDigest[:],
		MemberEventID: reservation.MemberEventID, CallbackStatus: reservation.CallbackStatus,
		MessageJSON: reservation.Message, ActionJSON: reservation.Action, CreatedAt: reservation.CreatedAt,
	})
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		record, err = queries.GetAIAudienceInboundWebhookReceipt(ctx, reservation.PackageID, reservation.ExternalEventIDDigest[:])
	}
	if err != nil {
		return legacyaudience.InboundWebhookReceipt{}, false, unavailable(err)
	}
	receipt, err := inboundWebhookReceipt(record)
	if err != nil {
		return legacyaudience.InboundWebhookReceipt{}, false, err
	}
	if receipt.PayloadDigest != reservation.PayloadDigest {
		return legacyaudience.InboundWebhookReceipt{}, false, legacyaudience.ErrIdempotencyConflict
	}
	replayRecord, err := queries.CreateAIAudienceInboundWebhookTransportReplay(ctx, CreateInboundWebhookTransportReplayParams{
		ClientID: reservation.ClientID, EventIDDigest: reservation.TransportEventIDDigest[:], ReceiptID: receipt.ID,
		PayloadDigest: reservation.PayloadDigest[:], CreatedAt: reservation.CreatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		replayRecord, err = queries.GetAIAudienceInboundWebhookTransportReplay(ctx, reservation.ClientID, reservation.TransportEventIDDigest[:])
	}
	if err != nil {
		return legacyaudience.InboundWebhookReceipt{}, false, unavailable(err)
	}
	replay, err := inboundWebhookTransportReplay(replayRecord)
	if err != nil {
		return legacyaudience.InboundWebhookReceipt{}, false, err
	}
	if replay.ReceiptID != receipt.ID || replay.PayloadDigest != reservation.PayloadDigest {
		return legacyaudience.InboundWebhookReceipt{}, false, legacyaudience.ErrIdempotencyConflict
	}
	return receipt, created, nil
}

func (repository *InboundWebhookRepository) querySet(ctx context.Context) (InboundWebhookQueries, error) {
	if repository == nil || repository.queries == nil || ctx == nil {
		return nil, legacyaudience.ErrUnavailable
	}
	queries, err := repository.queries(ctx)
	if err != nil || queries == nil {
		return nil, unavailable(err)
	}
	return queries, nil
}

func inboundWebhookReceipt(record InboundWebhookReceiptRecord) (legacyaudience.InboundWebhookReceipt, error) {
	if len(record.ExternalEventIDDigest) != sha256Size || len(record.PayloadDigest) != sha256Size {
		return legacyaudience.InboundWebhookReceipt{}, legacyaudience.ErrUnavailable
	}
	receipt := legacyaudience.InboundWebhookReceipt{
		ID: record.ID, PackageID: record.PackageID, State: record.State, CreatedAt: record.CreatedAt,
	}
	copy(receipt.ExternalEventIDDigest[:], record.ExternalEventIDDigest)
	copy(receipt.PayloadDigest[:], record.PayloadDigest)
	return receipt, nil
}

func inboundWebhookTransportReplay(record InboundWebhookTransportReplayRecord) (legacyaudience.InboundWebhookTransportReplay, error) {
	if len(record.PayloadDigest) != sha256Size {
		return legacyaudience.InboundWebhookTransportReplay{}, legacyaudience.ErrUnavailable
	}
	replay := legacyaudience.InboundWebhookTransportReplay{ReceiptID: record.ReceiptID}
	copy(replay.PayloadDigest[:], record.PayloadDigest)
	return replay, nil
}

func unavailable(err error) error {
	if err == nil {
		return legacyaudience.ErrUnavailable
	}
	return errors.Join(legacyaudience.ErrUnavailable, err)
}
