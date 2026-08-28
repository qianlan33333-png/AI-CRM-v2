package v1domain

import (
	"context"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type AudienceHistoryWriter interface {
	WriteGroup(context.Context, string, [32]byte, segmentport.HistoricalAudienceGroup) (segmentport.AudienceHistoryReceipt, error)
	WritePackage(context.Context, string, [32]byte, segmentport.HistoricalAudiencePackage) (segmentport.AudienceHistoryReceipt, error)
	WriteVersion(context.Context, string, [32]byte, segmentport.HistoricalAudienceVersion) (segmentport.AudienceHistoryReceipt, error)
	WriteSender(context.Context, string, [32]byte, segmentport.HistoricalAudienceSender) (segmentport.AudienceHistoryReceipt, error)
	WriteRule(context.Context, string, [32]byte, segmentport.HistoricalAudienceRule) (segmentport.AudienceHistoryReceipt, error)
	WriteRuleVersion(context.Context, string, [32]byte, segmentport.HistoricalAudienceRuleVersion) (segmentport.AudienceHistoryReceipt, error)
	WriteDefinition(context.Context, string, [32]byte, segmentport.HistoricalAudienceDefinition) (segmentport.AudienceHistoryReceipt, error)
	WriteMember(context.Context, string, [32]byte, segmentport.HistoricalAudienceMember) (segmentport.AudienceHistoryReceipt, error)
}

// Only historical DM01 lineage may provide actual V2 IDs. No Provider calls,
// numeric source-ID reuse, automatic member refresh, or access restoration.
type AudienceHistoryResolver interface {
	ResolveAudienceHistoryCustomer(context.Context, string) (*int64, error)
	ResolveAudienceHistoryStaff(context.Context, string) (*int64, error)
}

type AudienceHistoryImportResult struct {
	Imported    int
	Quarantined int
	Replayed    int
}
