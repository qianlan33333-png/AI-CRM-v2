package main

import (
	"context"
	"errors"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// campaignSegmentSourceAdapter keeps the cross-domain translation in the
// composition root. Campaign receives only its own source fact and canonical
// OneIDs; Segment remains the sole owner of source locks and SQL reads.
type campaignSegmentSourceAdapter struct {
	reader segmentport.TouchPlanSnapshotReader
}

var _ campaignport.TargetSourceResolver = (*campaignSegmentSourceAdapter)(nil)

func newCampaignSegmentSourceAdapter(reader segmentport.TouchPlanSnapshotReader) (*campaignSegmentSourceAdapter, error) {
	if reader == nil {
		return nil, errors.New("campaign Segment snapshot reader is required")
	}
	return &campaignSegmentSourceAdapter{reader: reader}, nil
}

func (adapter *campaignSegmentSourceAdapter) ResolveCampaignTargets(ctx context.Context, request campaign.InitiationSourceRequest) (campaignport.SourceResolution, error) {
	if adapter == nil || adapter.reader == nil || ctx == nil || ctx.Err() != nil {
		return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
	}
	switch request.Kind {
	case campaign.InitiationSourceSegmentMembers:
		snapshot, err := adapter.reader.ReadSegmentTouchPlanSnapshot(ctx, segmentport.SegmentID(request.SegmentID))
		if err != nil || !snapshot.Valid() || int64(snapshot.SegmentID) != request.SegmentID {
			return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
		}
		source, valid := campaign.NewSegmentMemberSourceRefFromSnapshot(int64(snapshot.SegmentID), snapshot.RefreshedAt, snapshot.Digest)
		if !valid {
			return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
		}
		return campaignport.SourceResolution{Source: source, CustomerIDs: campaignTargetIDs(snapshot.CustomerIDs)}, nil
	case campaign.InitiationSourceAudiencePackageMembers:
		snapshot, err := adapter.reader.ReadAudiencePackageTouchPlanSnapshot(ctx, segmentport.SegmentID(request.AudiencePackageID))
		if err != nil || !snapshot.Valid() || int64(snapshot.PackageID) != request.AudiencePackageID {
			return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
		}
		source, valid := campaign.NewAudiencePackageMemberSourceRefFromSnapshot(int64(snapshot.PackageID), snapshot.PackageVersion, snapshot.RefreshedAt, snapshot.Digest)
		if !valid {
			return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
		}
		return campaignport.SourceResolution{Source: source, CustomerIDs: campaignTargetIDs(snapshot.CustomerIDs)}, nil
	default:
		return campaignport.SourceResolution{}, campaignport.ErrSourceFactsUnavailable
	}
}

func campaignTargetIDs(customerIDs []segmentport.CustomerID) []int64 {
	result := make([]int64, len(customerIDs))
	for index, customerID := range customerIDs {
		result[index] = int64(customerID)
	}
	return result
}
