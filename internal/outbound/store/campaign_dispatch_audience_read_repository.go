package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
)

func (repository *CampaignDispatchRepository) AudiencePackageExists(ctx context.Context, packageID int64) (bool, error) {
	if repository == nil || repository.pool == nil || packageID < 1 {
		return false, outbound.ErrCampaignDispatchInvalid
	}
	return outbounddb.New(repository.pool).AudiencePackageExists(ctx, packageID)
}

func (repository *CampaignDispatchRepository) ListAudienceSendRecords(ctx context.Context, packageID int64, limit, offset int32) ([]outboundport.AudienceSendRecord, int64, error) {
	if repository == nil || repository.pool == nil || packageID < 1 || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, outbound.ErrCampaignDispatchInvalid
	}
	queries := outbounddb.New(repository.pool)
	total, err := queries.CountAudienceSendRecords(ctx, pgtype.Int8{Int64: packageID, Valid: true})
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListAudienceSendRecords(ctx, outbounddb.ListAudienceSendRecordsParams{PackageID: pgtype.Int8{Int64: packageID, Valid: true}, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	result := make([]outboundport.AudienceSendRecord, len(rows))
	for index, row := range rows {
		result[index], err = audienceSendRecordFromRow(row.ID, row.State, row.TechnicalAttemptCount, row.FailureClassification, row.ProviderResultReceived, row.ReceiptPresent, row.DeliveryProven, row.BusinessCallDispatched, row.RealExternalCallExecuted, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
	}
	return result, total, nil
}

func (repository *CampaignDispatchRepository) GetAudienceSendRecord(ctx context.Context, packageID, recordID int64) (outboundport.AudienceSendRecord, error) {
	if repository == nil || repository.pool == nil || packageID < 1 || recordID < 1 {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignDispatchInvalid
	}
	row, err := outbounddb.New(repository.pool).GetAudienceSendRecord(ctx, outbounddb.GetAudienceSendRecordParams{PackageID: pgtype.Int8{Int64: packageID, Valid: true}, RecordID: recordID})
	if errors.Is(err, pgx.ErrNoRows) {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignHandoffNotFound
	}
	if err != nil {
		return outboundport.AudienceSendRecord{}, err
	}
	return audienceSendRecordFromRow(row.ID, row.State, row.TechnicalAttemptCount, row.FailureClassification, row.ProviderResultReceived, row.ReceiptPresent, row.DeliveryProven, row.BusinessCallDispatched, row.RealExternalCallExecuted, row.CreatedAt, row.UpdatedAt)
}

func audienceSendRecordFromRow(id int64, state string, attempts int32, failure string, providerResult, receipt, delivery, dispatched, executed bool, createdAt, updatedAt pgtype.Timestamptz) (outboundport.AudienceSendRecord, error) {
	if id < 1 || attempts < 0 || !createdAt.Valid || !updatedAt.Valid {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignDispatchUnavailable
	}
	return outboundport.AudienceSendRecord{
		ID: id, State: outbound.CampaignDispatchState(state), TechnicalAttemptCount: attempts,
		FailureClassification: failure, ProviderResultReceived: providerResult,
		ReceiptPresent: receipt, DeliveryProven: delivery, BusinessCallDispatched: dispatched,
		RealExternalCallExecuted: executed, CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(),
	}, nil
}
