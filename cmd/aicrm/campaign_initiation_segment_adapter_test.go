package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type campaignSegmentSnapshotStub struct {
	segment  segmentport.SegmentTouchPlanSnapshot
	audience segmentport.AudiencePackageTouchPlanSnapshot
	err      error
}

func (stub *campaignSegmentSnapshotStub) ReadSegmentTouchPlanSnapshot(context.Context, segmentport.SegmentID) (segmentport.SegmentTouchPlanSnapshot, error) {
	return stub.segment, stub.err
}

func (stub *campaignSegmentSnapshotStub) ReadAudiencePackageTouchPlanSnapshot(context.Context, segmentport.SegmentID) (segmentport.AudiencePackageTouchPlanSnapshot, error) {
	return stub.audience, stub.err
}

func TestCampaignSegmentSourceAdapterMapsOwnerSnapshots(t *testing.T) {
	watermark := time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC)
	segment := segmentport.SegmentTouchPlanSnapshot{SegmentID: 7, RefreshedAt: watermark, CustomerIDs: []segmentport.CustomerID{2, 9}}
	segment.Digest = segmentport.CanonicalSegmentTouchPlanSnapshotDigest(segment.SegmentID, segment.RefreshedAt, segment.CustomerIDs)
	audience := segmentport.AudiencePackageTouchPlanSnapshot{PackageID: 11, PackageVersion: 6, RefreshedAt: watermark, CustomerIDs: []segmentport.CustomerID{2, 9}}
	audience.Digest = segmentport.CanonicalAudiencePackageTouchPlanSnapshotDigest(audience.PackageID, audience.PackageVersion, audience.RefreshedAt, audience.CustomerIDs)
	reader := &campaignSegmentSnapshotStub{segment: segment, audience: audience}
	adapter, err := newCampaignSegmentSourceAdapter(reader)
	if err != nil {
		t.Fatal(err)
	}

	segmentResult, err := adapter.ResolveCampaignTargets(context.Background(), campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceSegmentMembers, SegmentID: 7})
	if err != nil || segmentResult.Source.Segment == nil || segmentResult.Source.Segment.Digest != segment.Digest ||
		!reflect.DeepEqual(segmentResult.CustomerIDs, []int64{2, 9}) {
		t.Fatalf("segment result=%+v err=%v", segmentResult, err)
	}
	audienceResult, err := adapter.ResolveCampaignTargets(context.Background(), campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceAudiencePackageMembers, AudiencePackageID: 11})
	if err != nil || audienceResult.Source.AudiencePackage == nil || audienceResult.Source.AudiencePackage.Digest != audience.Digest ||
		audienceResult.Source.AudiencePackage.PackageVersion != 6 || !reflect.DeepEqual(audienceResult.CustomerIDs, []int64{2, 9}) {
		t.Fatalf("audience result=%+v err=%v", audienceResult, err)
	}
}

func TestCampaignSegmentSourceAdapterFailsClosed(t *testing.T) {
	adapter, err := newCampaignSegmentSourceAdapter(&campaignSegmentSnapshotStub{err: errors.New("source unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.ResolveCampaignTargets(context.Background(), campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceSegmentMembers, SegmentID: 7}); !errors.Is(err, campaignport.ErrSourceFactsUnavailable) {
		t.Fatalf("source error = %v", err)
	}
	if _, err = adapter.ResolveCampaignTargets(context.Background(), campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerSelection, CustomerIDs: []int64{1}}); !errors.Is(err, campaignport.ErrSourceFactsUnavailable) {
		t.Fatalf("unsupported source error = %v", err)
	}
}
