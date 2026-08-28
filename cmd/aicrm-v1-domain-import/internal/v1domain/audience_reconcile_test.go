package v1domain

import (
	"context"
	"errors"
	"testing"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type audienceVersionPageStub struct {
	segmentport.AudienceHistoryReader
	versions []segmentport.HistoricalAudienceVersion
	calls    int
}

func (r *audienceVersionPageStub) ListHistoricalAudienceVersions(_ context.Context, id int64, limit, offset int32) ([]segmentport.HistoricalAudienceVersion, int64, error) {
	r.calls++
	end := min(int(offset+limit), len(r.versions))
	if int(offset) >= end {
		return nil, int64(len(r.versions)), nil
	}
	return r.versions[offset:end], int64(len(r.versions)), nil
}

func TestAudienceCurrentVersionRequiresSameBatchAndParent(t *testing.T) {
	sourceID := int64(700)
	value := segmentport.HistoricalAudiencePackage{ID: 11, CurrentVersionSourceID: &sourceID}
	reader := &audienceVersionPageStub{versions: make([]segmentport.HistoricalAudienceVersion, 101)}
	reader.versions[100] = segmentport.HistoricalAudienceVersion{ID: 42, SourceID: sourceID, PackageHistoryID: 11}
	targets := map[string]map[string]struct{}{"segment_v1_audience_versions": {"42": {}}}
	if err := verifyAudienceCurrentVersion(context.Background(), reader, value, targets); err != nil || reader.calls != 2 {
		t.Fatal("current source version not found across real pages")
	}
	if err := verifyAudienceCurrentVersion(context.Background(), reader, value, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign-batch version accepted")
	}
	reader.versions[100].PackageHistoryID = 12
	if err := verifyAudienceCurrentVersion(context.Background(), reader, value, targets); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong parent accepted")
	}
	reader.versions = nil
	if err := verifyAudienceCurrentVersion(context.Background(), reader, value, targets); !errors.Is(err, ErrConflict) {
		t.Fatal("missing current source version accepted")
	}
	value.CurrentVersionSourceID = nil
	if err := verifyAudienceCurrentVersion(context.Background(), nil, value, nil); err != nil {
		t.Fatal("nullable current version rejected")
	}
}

func TestAudienceReconcileScopeDoesNotIncludeRuntimeTables(t *testing.T) {
	for _, table := range []string{"public/segment_members", "public/ai_audience_member_event", "public/ai_audience_package_run", "public/ai_audience_hxc_member_usage_projection"} {
		if isAudienceHistorySource(table) {
			t.Fatal("runtime table included")
		}
	}
	if _, err := ReconcileAudienceHistory(context.Background(), nil, "different", "run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong version accepted")
	}
	if _, err := verifyAudienceHistoryTarget(context.Background(), nil, reconciliationRow{}, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("missing target accepted")
	}
}
