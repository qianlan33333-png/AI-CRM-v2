package app

import (
	"context"
	"errors"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

const (
	DefaultAudienceSendRecordLimit int32 = 20
	MaximumAudienceSendRecordLimit int32 = 100
)

type AudienceSendRecordPage struct {
	Items  []outboundport.AudienceSendRecord
	Total  int64
	Limit  int32
	Offset int32
}

type AudienceSendRecordService struct {
	repository outboundport.AudienceSendRecordRepository
}

func NewAudienceSendRecordService(repository outboundport.AudienceSendRecordRepository) (*AudienceSendRecordService, error) {
	if nilCampaignDispatchDependency(repository) {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	return &AudienceSendRecordService{repository: repository}, nil
}

func (service *AudienceSendRecordService) List(ctx context.Context, packageID int64, limit, offset int32) (AudienceSendRecordPage, error) {
	if ctx == nil || service == nil || service.repository == nil || packageID < 1 || limit < 1 || limit > MaximumAudienceSendRecordLimit || offset < 0 {
		return AudienceSendRecordPage{}, outbound.ErrCampaignDispatchInvalid
	}
	present, err := service.repository.AudiencePackageExists(ctx, packageID)
	if err != nil {
		return AudienceSendRecordPage{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	if !present {
		return AudienceSendRecordPage{}, outbound.ErrCampaignHandoffNotFound
	}
	items, total, err := service.repository.ListAudienceSendRecords(ctx, packageID, limit, offset)
	if err != nil || total < 0 || int64(len(items)) > int64(limit) {
		return AudienceSendRecordPage{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	for _, item := range items {
		if !validAudienceSendRecord(item) {
			return AudienceSendRecordPage{}, outbound.ErrCampaignDispatchUnavailable
		}
	}
	return AudienceSendRecordPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (service *AudienceSendRecordService) Get(ctx context.Context, packageID, recordID int64) (outboundport.AudienceSendRecord, error) {
	if ctx == nil || service == nil || service.repository == nil || packageID < 1 || recordID < 1 {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignDispatchInvalid
	}
	present, err := service.repository.AudiencePackageExists(ctx, packageID)
	if err != nil {
		return outboundport.AudienceSendRecord{}, errors.Join(outbound.ErrCampaignDispatchUnavailable, err)
	}
	if !present {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignHandoffNotFound
	}
	item, err := service.repository.GetAudienceSendRecord(ctx, packageID, recordID)
	if err != nil {
		return outboundport.AudienceSendRecord{}, err
	}
	if !validAudienceSendRecord(item) || item.ID != recordID {
		return outboundport.AudienceSendRecord{}, outbound.ErrCampaignDispatchUnavailable
	}
	return item, nil
}

func validAudienceSendRecord(item outboundport.AudienceSendRecord) bool {
	return item.ID > 0 && item.TechnicalAttemptCount >= 0 && !item.CreatedAt.IsZero() && !item.UpdatedAt.IsZero() &&
		(item.State == outbound.CampaignDispatchBlocked || item.State == outbound.CampaignDispatchAccepted || item.State == outbound.CampaignDispatchQueued || item.State == outbound.CampaignDispatchAttempted || item.State == outbound.CampaignDispatchExecuted || item.State == outbound.CampaignDispatchOutcomeUnknown || item.State == outbound.CampaignDispatchReconciled || item.State == outbound.CampaignDispatchRetryableFailed || item.State == outbound.CampaignDispatchFinalFailed) &&
		(!item.DeliveryProven || item.ProviderResultReceived && item.ReceiptPresent && item.BusinessCallDispatched && item.RealExternalCallExecuted)
}
