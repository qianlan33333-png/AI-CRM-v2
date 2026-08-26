package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// NewInboundWebhookQueryFactory binds the generated Segment query set to the
// transaction supplied by the platform unit of work.
func NewInboundWebhookQueryFactory() InboundWebhookQueryFactory {
	return func(tx pgx.Tx) InboundWebhookQueries {
		return inboundWebhookGeneratedQueries{queries: segmentdb.New(tx)}
	}
}

type inboundWebhookGeneratedQueries struct{ queries *segmentdb.Queries }

func (adapter inboundWebhookGeneratedQueries) AIAudienceInboundPackageExists(ctx context.Context, packageID int64) (bool, error) {
	return adapter.queries.AIAudienceInboundPackageExists(ctx, packageID)
}

func (adapter inboundWebhookGeneratedQueries) CreateAIAudienceInboundWebhookReceipt(ctx context.Context, params CreateInboundWebhookReceiptParams) (InboundWebhookReceiptRecord, error) {
	row, err := adapter.queries.CreateAIAudienceInboundWebhookReceipt(ctx, segmentdb.CreateAIAudienceInboundWebhookReceiptParams{
		PackageID: params.PackageID, ExternalEventIDDigest: params.ExternalEventIDDigest, PayloadDigest: params.PayloadDigest,
		MemberEventID: optionalInboundMemberEventID(params.MemberEventID), CallbackStatus: params.CallbackStatus,
		MessageJson: params.MessageJSON, ActionJson: params.ActionJSON, CreatedAt: pgtype.Timestamptz{Time: params.CreatedAt, Valid: true},
	})
	return inboundWebhookReceiptRecord(row.ID, row.PackageID, row.State, row.ExternalEventIDDigest, row.PayloadDigest, row.CreatedAt), err
}

func (adapter inboundWebhookGeneratedQueries) GetAIAudienceInboundWebhookReceipt(ctx context.Context, packageID int64, externalEventIDDigest []byte) (InboundWebhookReceiptRecord, error) {
	row, err := adapter.queries.GetAIAudienceInboundWebhookReceipt(ctx, segmentdb.GetAIAudienceInboundWebhookReceiptParams{
		PackageID: packageID, ExternalEventIDDigest: externalEventIDDigest,
	})
	return inboundWebhookReceiptRecord(row.ID, row.PackageID, row.State, row.ExternalEventIDDigest, row.PayloadDigest, row.CreatedAt), err
}

func (adapter inboundWebhookGeneratedQueries) CreateAIAudienceInboundWebhookTransportReplay(ctx context.Context, params CreateInboundWebhookTransportReplayParams) (InboundWebhookTransportReplayRecord, error) {
	row, err := adapter.queries.CreateAIAudienceInboundWebhookTransportReplay(ctx, segmentdb.CreateAIAudienceInboundWebhookTransportReplayParams{
		ClientID: params.ClientID, EventIDDigest: params.EventIDDigest, ReceiptID: params.ReceiptID,
		PayloadDigest: params.PayloadDigest, CreatedAt: pgtype.Timestamptz{Time: params.CreatedAt, Valid: true},
	})
	return InboundWebhookTransportReplayRecord{ReceiptID: row.ReceiptID, PayloadDigest: row.PayloadDigest}, err
}

func (adapter inboundWebhookGeneratedQueries) GetAIAudienceInboundWebhookTransportReplay(ctx context.Context, clientID string, eventIDDigest []byte) (InboundWebhookTransportReplayRecord, error) {
	row, err := adapter.queries.GetAIAudienceInboundWebhookTransportReplay(ctx, segmentdb.GetAIAudienceInboundWebhookTransportReplayParams{
		ClientID: clientID, EventIDDigest: eventIDDigest,
	})
	return InboundWebhookTransportReplayRecord{ReceiptID: row.ReceiptID, PayloadDigest: row.PayloadDigest}, err
}

func optionalInboundMemberEventID(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func inboundWebhookReceiptRecord(id, packageID int64, state string, externalEventIDDigest, payloadDigest []byte, createdAt pgtype.Timestamptz) InboundWebhookReceiptRecord {
	return InboundWebhookReceiptRecord{
		ID: id, PackageID: packageID, State: state, ExternalEventIDDigest: externalEventIDDigest,
		PayloadDigest: payloadDigest, CreatedAt: createdAt.Time,
	}
}
