package port

import (
	"context"
	"time"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
)

// AudienceSendRecord is a deliberately closed projection. Provider message
// IDs, sender user IDs, external user IDs and raw provider responses remain
// private worker facts.
type AudienceSendRecord struct {
	ID                       int64
	State                    outbound.CampaignDispatchState
	TechnicalAttemptCount    int32
	FailureClassification    string
	ProviderResultReceived   bool
	ReceiptPresent           bool
	DeliveryProven           bool
	BusinessCallDispatched   bool
	RealExternalCallExecuted bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type AudienceSendRecordRepository interface {
	AudiencePackageExists(context.Context, int64) (bool, error)
	ListAudienceSendRecords(context.Context, int64, int32, int32) ([]AudienceSendRecord, int64, error)
	GetAudienceSendRecord(context.Context, int64, int64) (AudienceSendRecord, error)
}
