package campaign

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
)

var ErrHistoricalDefinitionConflict = errors.New("historical campaign definition conflict")

// HistoricalDefinition is a migration-only input. SourceIdentifier is opaque
// to Campaign; the migration owner persists it with PayloadDigest in its own
// receipt/mapping journal.
type HistoricalDefinition struct {
	SourceIdentifier       string
	PayloadDigest          [32]byte
	OriginalApprovalStatus string
	OriginalRuntimeStatus  string
	Campaign               Campaign
	Steps                  []Step
}

// HistoricalDefinitionReceipt retains source state for the migration receipt
// and mapping while the stored Campaign is deliberately disabled.
type HistoricalDefinitionReceipt struct {
	SourceIdentifier       string
	PayloadDigest          [32]byte
	OriginalApprovalStatus string
	OriginalRuntimeStatus  string
	TargetCampaignCode     string
	Replayed               bool
}

// HistoricalDefinitionStore is Campaign-owned persistence. Implementations
// must use the caller's transaction context for header and steps.
type HistoricalDefinitionStore interface {
	InsertHistoricalDefinition(context.Context, HistoricalDefinition) error
}

// HistoricalDefinitionJournal is migration-owned provenance. Its operations
// must share the caller's transaction with HistoricalDefinitionStore.
type HistoricalDefinitionJournal interface {
	LoadHistoricalDefinition(context.Context, string) (HistoricalDefinitionReceipt, bool, error)
	RecordHistoricalDefinition(context.Context, HistoricalDefinitionReceipt) error
}

type HistoricalDefinitionWriter struct {
	store   HistoricalDefinitionStore
	journal HistoricalDefinitionJournal
}

func NewHistoricalDefinitionWriter(store HistoricalDefinitionStore, journal HistoricalDefinitionJournal) (*HistoricalDefinitionWriter, error) {
	if nilish(store) || nilish(journal) {
		return nil, ErrUnavailable
	}
	return &HistoricalDefinitionWriter{store: store, journal: journal}, nil
}

// Import inserts a new disabled header followed by its steps. It neither
// invokes the Campaign service nor writes plans, commands, events, or receipts
// owned by the Campaign runtime.
func (writer *HistoricalDefinitionWriter) Import(ctx context.Context, definition HistoricalDefinition) (HistoricalDefinitionReceipt, error) {
	if writer == nil || nilish(writer.store) || nilish(writer.journal) || ctx == nil || !validHistoricalDefinition(definition) {
		return HistoricalDefinitionReceipt{}, ErrUnavailable
	}
	existing, found, err := writer.journal.LoadHistoricalDefinition(ctx, definition.SourceIdentifier)
	if err != nil {
		return HistoricalDefinitionReceipt{}, err
	}
	if found {
		if !sameHistoricalDefinition(existing, definition) {
			return HistoricalDefinitionReceipt{}, ErrHistoricalDefinitionConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	stored := definition
	stored.Campaign.ApprovalStatus = ApprovalRejected
	stored.Campaign.RuntimeStatus = RuntimePaused
	stored.Campaign.Version = 1
	if err = writer.store.InsertHistoricalDefinition(ctx, stored); err != nil {
		return HistoricalDefinitionReceipt{}, err
	}
	receipt := HistoricalDefinitionReceipt{
		SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest,
		OriginalApprovalStatus: definition.OriginalApprovalStatus, OriginalRuntimeStatus: definition.OriginalRuntimeStatus,
		TargetCampaignCode: definition.Campaign.Code,
	}
	if err = writer.journal.RecordHistoricalDefinition(ctx, receipt); err != nil {
		return HistoricalDefinitionReceipt{}, err
	}
	return receipt, nil
}

func validHistoricalDefinition(value HistoricalDefinition) bool {
	return validSourceStatus(value.SourceIdentifier) && validSourceStatus(value.OriginalApprovalStatus) && validSourceStatus(value.OriginalRuntimeStatus) && value.Campaign.Version == 1 && validCampaign(value.Campaign) && validSteps(value.Steps)
}

func validSourceStatus(value string) bool {
	return value != "" && len(value) <= 128 && value == strings.TrimSpace(value)
}

func sameHistoricalDefinition(receipt HistoricalDefinitionReceipt, value HistoricalDefinition) bool {
	return receipt.SourceIdentifier == value.SourceIdentifier && subtle.ConstantTimeCompare(receipt.PayloadDigest[:], value.PayloadDigest[:]) == 1 && receipt.OriginalApprovalStatus == value.OriginalApprovalStatus && receipt.OriginalRuntimeStatus == value.OriginalRuntimeStatus && receipt.TargetCampaignCode == value.Campaign.Code
}
